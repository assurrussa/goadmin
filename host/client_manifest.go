package host

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

const (
	ClientBundleFormatVersion = 1
	ClientExtensionAPIVersion = 1
	ClientBundleManifestPath  = "dist/admin-extensions.manifest.json"
	clientCoreExtensionKey    = "goadmin.core"
)

var ErrInvalidClientBundle = errors.New("goadmin client bundle is incompatible")

// ClientExtensionRequirement is the server feature fingerprint that must be
// present in the built admin UI bundle.
type ClientExtensionRequirement struct {
	Key         string `json:"key"`
	APIVersion  int    `json:"apiVersion"`
	Fingerprint string `json:"fingerprint"`
}

// ClientBundleExtension is one build-time extension recorded in the emitted bundle manifest.
type ClientBundleExtension struct {
	Key         string   `json:"key"`
	APIVersion  int      `json:"apiVersion"`
	Fingerprint string   `json:"fingerprint"`
	Requires    []string `json:"requires,omitempty"`
}

// ClientBundleManifest is the runtime-verifiable output of the admin UI builder.
type ClientBundleManifest struct {
	FormatVersion int                     `json:"formatVersion"`
	Extensions    []ClientBundleExtension `json:"extensions"`
}

// ClientBundleValidation connects host/server requirements to a built client manifest.
type ClientBundleValidation struct {
	Requirements []ClientExtensionRequirement
	Manifest     ClientBundleManifest
}

// LoadClientBundleManifest reads and validates the standard embedded manifest path.
func LoadClientBundleManifest(publicFS fs.FS) (ClientBundleManifest, error) {
	data, err := fs.ReadFile(publicFS, ClientBundleManifestPath)
	if err != nil {
		return ClientBundleManifest{}, fmt.Errorf("read client bundle manifest: %w", err)
	}

	var manifest ClientBundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ClientBundleManifest{}, fmt.Errorf("decode client bundle manifest: %w", err)
	}
	if err := ValidateClientBundle(nil, manifest); err != nil {
		return ClientBundleManifest{}, err
	}

	return manifest, nil
}

// BuildClientBundleValidation binds a feature registry to an embedded build manifest
// and checks that the client files required by the built-in templates are present.
// New runs this check against Config.Public so assembly fails before boot
// when a required server/client extension pair has drifted.
func BuildClientBundleValidation(
	registry Registry,
	publicFS fs.FS,
) (*ClientBundleValidation, error) {
	manifest, err := LoadClientBundleManifest(publicFS)
	if err != nil {
		return nil, err
	}
	for _, path := range []string{"dist/js/app.js", "dist/css/app.css", "dist/favicon.ico"} {
		info, statErr := fs.Stat(publicFS, path)
		if statErr != nil {
			return nil, fmt.Errorf("%w: required admin asset %q: %w", ErrInvalidClientBundle, path, statErr)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("%w: required admin asset %q is empty or not a regular file", ErrInvalidClientBundle, path)
		}
	}
	requirements, err := registry.ClientExtensionRequirements()
	if err != nil {
		return nil, err
	}
	if err := ValidateClientBundle(requirements, manifest); err != nil {
		return nil, err
	}

	return &ClientBundleValidation{
		Requirements: requirements,
		Manifest:     manifest,
	}, nil
}

// ValidateClientBundle rejects missing, duplicate, incompatible, or drifted extensions.
func ValidateClientBundle(
	requirements []ClientExtensionRequirement,
	manifest ClientBundleManifest,
) error {
	if manifest.FormatVersion != ClientBundleFormatVersion {
		return fmt.Errorf(
			"%w: bundle format version %d, expected %d",
			ErrInvalidClientBundle,
			manifest.FormatVersion,
			ClientBundleFormatVersion,
		)
	}

	built := make(map[string]ClientBundleExtension, len(manifest.Extensions))
	for _, extension := range manifest.Extensions {
		if err := validateClientExtension(extension.Key, extension.APIVersion, extension.Fingerprint); err != nil {
			return err
		}
		if _, exists := built[extension.Key]; exists {
			return fmt.Errorf("%w: duplicate built extension %q", ErrInvalidClientBundle, extension.Key)
		}
		built[extension.Key] = extension
	}
	if err := validateClientBundleDependencies(built); err != nil {
		return err
	}

	seenRequirements := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		if err := validateClientExtension(
			requirement.Key,
			requirement.APIVersion,
			requirement.Fingerprint,
		); err != nil {
			return err
		}
		if _, exists := seenRequirements[requirement.Key]; exists {
			return fmt.Errorf("%w: duplicate required extension %q", ErrInvalidClientBundle, requirement.Key)
		}
		seenRequirements[requirement.Key] = struct{}{}

		extension, exists := built[requirement.Key]
		if !exists {
			return fmt.Errorf("%w: required extension %q is missing", ErrInvalidClientBundle, requirement.Key)
		}
		if extension.APIVersion != requirement.APIVersion {
			return fmt.Errorf(
				"%w: extension %q API version %d, expected %d",
				ErrInvalidClientBundle,
				requirement.Key,
				extension.APIVersion,
				requirement.APIVersion,
			)
		}
		if extension.Fingerprint != requirement.Fingerprint {
			return fmt.Errorf("%w: extension %q fingerprint mismatch", ErrInvalidClientBundle, requirement.Key)
		}
	}

	return nil
}

func validateClientBundleDependencies(built map[string]ClientBundleExtension) error {
	for _, extension := range built {
		seen := make(map[string]struct{}, len(extension.Requires))
		for _, required := range extension.Requires {
			if !validClientExtensionKey(required) {
				return fmt.Errorf(
					"%w: extension %q contains invalid dependency %q",
					ErrInvalidClientBundle,
					extension.Key,
					required,
				)
			}
			if _, exists := seen[required]; exists {
				return fmt.Errorf(
					"%w: extension %q contains duplicate dependency %q",
					ErrInvalidClientBundle,
					extension.Key,
					required,
				)
			}
			seen[required] = struct{}{}
			if required == clientCoreExtensionKey {
				continue
			}
			if _, exists := built[required]; !exists {
				return fmt.Errorf(
					"%w: extension %q requires missing extension %q",
					ErrInvalidClientBundle,
					extension.Key,
					required,
				)
			}
		}
	}

	visiting := make(map[string]bool, len(built))
	visited := make(map[string]bool, len(built))
	var visit func(string) error
	visit = func(key string) error {
		if visiting[key] {
			return fmt.Errorf("%w: client extension dependency cycle at %q", ErrInvalidClientBundle, key)
		}
		if visited[key] {
			return nil
		}
		visiting[key] = true
		for _, required := range built[key].Requires {
			if required == clientCoreExtensionKey {
				continue
			}
			if err := visit(required); err != nil {
				return err
			}
		}
		visiting[key] = false
		visited[key] = true

		return nil
	}
	for key := range built {
		if err := visit(key); err != nil {
			return err
		}
	}

	return nil
}

// ValidateClientBundleForRegistry validates all structured feature requirements.
func ValidateClientBundleForRegistry(registry Registry, manifest ClientBundleManifest) error {
	requirements, err := registry.ClientExtensionRequirements()
	if err != nil {
		return err
	}

	return ValidateClientBundle(requirements, manifest)
}

func validateClientExtension(key string, apiVersion int, fingerprint string) error {
	if !validClientExtensionKey(key) {
		return fmt.Errorf("%w: invalid extension key %q", ErrInvalidClientBundle, key)
	}
	if apiVersion != ClientExtensionAPIVersion {
		return fmt.Errorf(
			"%w: extension %q API version %d is unsupported",
			ErrInvalidClientBundle,
			key,
			apiVersion,
		)
	}
	if len(fingerprint) != len("sha256:")+64 || fingerprint[:len("sha256:")] != "sha256:" {
		return fmt.Errorf("%w: extension %q has invalid fingerprint", ErrInvalidClientBundle, key)
	}
	if _, err := hex.DecodeString(fingerprint[len("sha256:"):]); err != nil {
		return fmt.Errorf("%w: extension %q has invalid fingerprint", ErrInvalidClientBundle, key)
	}

	return nil
}

func validClientExtensionKey(key string) bool {
	if key == "" {
		return false
	}

	for i, char := range key {
		if char >= 'a' && char <= 'z' || i > 0 && char >= '0' && char <= '9' {
			continue
		}
		if i > 0 && (char == '.' || char == '-' || char == '_') {
			continue
		}

		return false
	}

	return true
}

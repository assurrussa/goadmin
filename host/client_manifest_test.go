package host_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

const (
	testClientFingerprint = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	clientExtensionKey    = "gocms"
	testClientAssetJS     = "dist/js/app.js"
	testClientAssetCSS    = "dist/css/app.css"
)

func TestValidateClientBundleAcceptsExactRequiredFingerprint(t *testing.T) {
	t.Parallel()

	manifest := adminhost.ClientBundleManifest{
		FormatVersion: adminhost.ClientBundleFormatVersion,
		Extensions: []adminhost.ClientBundleExtension{
			{Key: clientExtensionKey, APIVersion: 1, Fingerprint: testClientFingerprint},
			{Key: "host.analytics", APIVersion: 1, Fingerprint: "sha256:" + strings.Repeat("b", 64)},
		},
	}

	err := adminhost.ValidateClientBundle([]adminhost.ClientExtensionRequirement{{
		Key:         clientExtensionKey,
		APIVersion:  adminhost.ClientExtensionAPIVersion,
		Fingerprint: testClientFingerprint,
	}}, manifest)
	require.NoError(t, err)
}

func TestLoadClientBundleManifestReadsEmbeddedBuildOutput(t *testing.T) {
	t.Parallel()

	publicFS := fstest.MapFS{
		adminhost.ClientBundleManifestPath: {
			Data: []byte(
				`{"formatVersion":1,"extensions":[{"key":"gocms","apiVersion":1,"fingerprint":"` +
					testClientFingerprint + `"}]}`,
			),
		},
	}

	manifest, err := adminhost.LoadClientBundleManifest(publicFS)
	require.NoError(t, err)
	require.Equal(t, clientExtensionKey, manifest.Extensions[0].Key)
}

func TestBuildClientBundleValidationBindsRegistryRequirements(t *testing.T) {
	t.Parallel()

	publicFS := fstest.MapFS{
		adminhost.ClientBundleManifestPath: {
			Data: []byte(
				`{"formatVersion":1,"extensions":[{"key":"gocms","apiVersion":1,"fingerprint":"` +
					testClientFingerprint + `"}]}`,
			),
		},
		testClientAssetJS:  {Data: []byte("js")},
		testClientAssetCSS: {Data: []byte("css")},
		"dist/favicon.ico": {Data: []byte("ico")},
	}
	registry := adminhost.NewRegistry(storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
		Key: "content",
		ClientRequirements: []adminhost.ClientExtensionRequirement{{
			Key:         clientExtensionKey,
			APIVersion:  1,
			Fingerprint: testClientFingerprint,
		}},
	}})

	validation, err := adminhost.BuildClientBundleValidation(registry, publicFS)
	require.NoError(t, err)
	require.Len(t, validation.Requirements, 1)
	require.Equal(t, clientExtensionKey, validation.Manifest.Extensions[0].Key)
}

func TestBuildClientBundleValidationRejectsMissingOrEmptyAssets(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		assets fstest.MapFS
		want   string
	}{
		{name: "missing JavaScript", want: testClientAssetJS},
		{name: "empty JavaScript", assets: fstest.MapFS{testClientAssetJS: {Data: []byte{}}}, want: testClientAssetJS},
		{name: "missing stylesheet", assets: fstest.MapFS{testClientAssetJS: {Data: []byte("js")}}, want: testClientAssetCSS},
		{
			name: "missing favicon",
			assets: fstest.MapFS{
				testClientAssetJS:  {Data: []byte("js")},
				testClientAssetCSS: {Data: []byte("css")},
			},
			want: "dist/favicon.ico",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			publicFS := fstest.MapFS{
				adminhost.ClientBundleManifestPath: {Data: []byte(`{"formatVersion":1,"extensions":[]}`)},
			}
			for path, file := range test.assets {
				publicFS[path] = file
			}
			_, err := adminhost.BuildClientBundleValidation(adminhost.NewRegistry(), publicFS)
			require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestValidateClientBundleRejectsMissingDuplicateAndDriftedExtensions(t *testing.T) {
	t.Parallel()

	requirement := adminhost.ClientExtensionRequirement{
		Key:         clientExtensionKey,
		APIVersion:  1,
		Fingerprint: testClientFingerprint,
	}

	tests := map[string]adminhost.ClientBundleManifest{
		"missing": {
			FormatVersion: 1,
		},
		"duplicate": {
			FormatVersion: 1,
			Extensions: []adminhost.ClientBundleExtension{
				{Key: clientExtensionKey, APIVersion: 1, Fingerprint: testClientFingerprint},
				{Key: clientExtensionKey, APIVersion: 1, Fingerprint: testClientFingerprint},
			},
		},
		"fingerprint drift": {
			FormatVersion: 1,
			Extensions: []adminhost.ClientBundleExtension{{
				Key:         clientExtensionKey,
				APIVersion:  1,
				Fingerprint: "sha256:" + strings.Repeat("c", 64),
			}},
		},
	}

	for name, manifest := range tests {
		manifest := manifest
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := adminhost.ValidateClientBundle([]adminhost.ClientExtensionRequirement{requirement}, manifest)
			require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
		})
	}
}

func TestRegistryClientRequirementsRejectDuplicateOwnership(t *testing.T) {
	t.Parallel()

	requirement := adminhost.ClientExtensionRequirement{
		Key:         clientExtensionKey,
		APIVersion:  1,
		Fingerprint: testClientFingerprint,
	}
	registry := adminhost.NewRegistry(
		storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
			Key:                "content",
			ClientRequirements: []adminhost.ClientExtensionRequirement{requirement},
		}},
		storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
			Key:                "content-copy",
			ClientRequirements: []adminhost.ClientExtensionRequirement{requirement},
		}},
	)

	_, err := registry.ClientExtensionRequirements()
	require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
}

func TestValidateClientBundleRejectsInvalidDependencyGraph(t *testing.T) {
	t.Parallel()

	tests := map[string][]adminhost.ClientBundleExtension{
		"missing": {
			clientBundleExtension(clientExtensionKey, "host.missing"),
		},
		"duplicate": {
			clientBundleExtension(clientExtensionKey, "goadmin.core", "goadmin.core"),
		},
		"cycle": {
			clientBundleExtension(clientExtensionKey, "host.cycle"),
			clientBundleExtension("host.cycle", clientExtensionKey),
		},
	}

	for name, extensions := range tests {
		extensions := extensions
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := adminhost.ValidateClientBundle(nil, adminhost.ClientBundleManifest{
				FormatVersion: adminhost.ClientBundleFormatVersion,
				Extensions:    extensions,
			})
			require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
		})
	}
}

func clientBundleExtension(key string, requires ...string) adminhost.ClientBundleExtension {
	return adminhost.ClientBundleExtension{
		Key:         key,
		APIVersion:  adminhost.ClientExtensionAPIVersion,
		Fingerprint: testClientFingerprint,
		Requires:    requires,
	}
}

func TestRegistrySharesIdenticalCoreCRUDReferences(t *testing.T) {
	t.Parallel()
	requirement := adminhost.ClientExtensionRequirement{Key: "goadmin.crud", APIVersion: 1, Fingerprint: testClientFingerprint}
	feature := func(key string, r adminhost.ClientExtensionRequirement) adminhost.Feature {
		return storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
			Key: key, ClientReferences: []adminhost.ClientExtensionRequirement{r},
		}}
	}
	registry := adminhost.NewRegistry(feature("products", requirement), feature("orders", requirement))
	requirements, err := registry.ClientExtensionRequirements()
	require.NoError(t, err)
	require.Equal(t, []adminhost.ClientExtensionRequirement{requirement}, requirements)
	owner := storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
		Key: "core-owner", ClientRequirements: []adminhost.ClientExtensionRequirement{requirement},
	}}
	_, err = adminhost.NewRegistry(owner, owner).ClientExtensionRequirements()
	require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
	requirements, err = adminhost.NewRegistry(owner, feature("products", requirement)).ClientExtensionRequirements()
	require.NoError(t, err)
	require.Equal(t, []adminhost.ClientExtensionRequirement{requirement}, requirements)
	requirement.Fingerprint = "sha256:" + strings.Repeat("b", 64)
	registry = adminhost.NewRegistry(feature("products", requirements[0]), feature("orders", requirement))
	_, err = registry.ClientExtensionRequirements()
	require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
}

func TestClientReferencesShareExtensionsWithoutHidingOwners(t *testing.T) {
	t.Parallel()
	reference := adminhost.ClientExtensionRequirement{Key: "host.fields", APIVersion: 1, Fingerprint: testClientFingerprint}
	referenced := func(key string, r adminhost.ClientExtensionRequirement) adminhost.Feature {
		return storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
			Key: key, ClientReferences: []adminhost.ClientExtensionRequirement{r},
		}}
	}
	first, second := referenced("products", reference), referenced("reports", reference)
	requirements, err := adminhost.NewRegistry(first, second).ClientExtensionRequirements()
	require.NoError(t, err)
	require.Equal(t, []adminhost.ClientExtensionRequirement{reference}, requirements)
	manifest := adminhost.ClientBundleManifest{
		FormatVersion: 1,
		Extensions: []adminhost.ClientBundleExtension{{
			Key: reference.Key, APIVersion: 1, Fingerprint: reference.Fingerprint,
		}},
	}
	require.NoError(t, adminhost.ValidateClientBundle(requirements, manifest))
	require.ErrorIs(t,
		adminhost.ValidateClientBundle(requirements, adminhost.ClientBundleManifest{FormatVersion: 1}),
		adminhost.ErrInvalidClientBundle)
	owner := storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
		Key: "field-owner", ClientRequirements: []adminhost.ClientExtensionRequirement{reference},
	}}
	requirements, err = adminhost.NewRegistry(first, owner, second).ClientExtensionRequirements()
	require.NoError(t, err)
	require.Len(t, requirements, 1)
	_, err = adminhost.NewRegistry(first, owner, owner).ClientExtensionRequirements()
	require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
	conflicting := reference
	conflicting.Fingerprint = "sha256:" + strings.Repeat("b", 64)
	_, err = adminhost.NewRegistry(first, referenced("other", conflicting)).ClientExtensionRequirements()
	require.ErrorIs(t, err, adminhost.ErrInvalidClientBundle)
	descriptors := adminhost.NewRegistry(first).Descriptors()
	descriptors[0].ClientReferences[0].Fingerprint = mutatedValue
	requirements, err = adminhost.NewRegistry(first).ClientExtensionRequirements()
	require.NoError(t, err)
	require.Equal(t, reference, requirements[0])
}

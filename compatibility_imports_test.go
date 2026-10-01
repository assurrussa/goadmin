package goadmin_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func shouldSkipSourcePolicyDir(name string) bool {
	switch name {
	case ".cache", ".git", ".go-cache", "node_modules", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func TestShouldSkipSourcePolicyDir(t *testing.T) {
	t.Parallel()

	for _, name := range []string{".cache", ".git", ".go-cache", "node_modules", "tmp", "vendor"} {
		if !shouldSkipSourcePolicyDir(name) {
			t.Errorf("expected %q to be excluded from repository source policy scans", name)
		}
	}
	if shouldSkipSourcePolicyDir("host") {
		t.Error("repository source directory must remain in policy scans")
	}
}

func TestNoRuntimeImportsLegacyAuthCompatibilityPackages(t *testing.T) {
	t.Helper()

	bannedImports := []string{
		`"github.com/assurrussa/goadmin/domain/auth/core"`,
		`"github.com/assurrussa/goadmin/domain/auth/oidc"`,
		`"github.com/assurrussa/goadmin/domain/auth/sso"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/adminaccountservice"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/passwordauthservice"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/passwordchangeservice"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/passwordresetservice"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/tokenauthservice"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/externalidentityservice"`,
		`"github.com/assurrussa/goadmin/domain/auth/service/identitylinkservice"`,
	}

	excludedPrefixes := []string{
		"domain/auth/core/",
		"domain/auth/oidc/",
		"domain/auth/sso/",
		"domain/auth/service/adminaccountservice/",
		"domain/auth/service/passwordauthservice/",
		"domain/auth/service/passwordchangeservice/",
		"domain/auth/service/passwordresetservice/",
		"domain/auth/service/tokenauthservice/",
		"domain/auth/service/externalidentityservice/",
		"domain/auth/service/identitylinkservice/",
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") { //nolint:goconst // required
			return nil
		}
		if slices.ContainsFunc(excludedPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		for _, bannedImport := range bannedImports {
			if strings.Contains(string(content), bannedImport) {
				offenders = append(offenders, normalized+" -> "+bannedImport)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin sources: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("legacy auth compatibility imports remain:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestNoRuntimeImportsLegacyAuthPackages(t *testing.T) {
	t.Helper()

	const bannedImportPrefix = `"github.com/assurrussa/goadmin/domain/auth/`

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasPrefix(normalized, "domain/auth/") {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		if strings.Contains(string(content), bannedImportPrefix) {
			offenders = append(offenders, normalized)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin sources: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("runtime goadmin sources still import legacy domain/auth packages:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestNoRuntimeImportsGoauthModelOutsideCompatAndStorage(t *testing.T) {
	t.Helper()

	targetPrefixes := []string{
		"bootstrap/",
		"di/",
		"http/handlers/",
		"infrastructure/auth/",
		"infrastructure/pgsql/repositories/userrepo/",
		"infrastructure/pgsql/repositories/tokenrepo/",
	}
	excludedPrefixes := []string{
		"http/handlers/auth/mocks/",
		"http/handlers/administrations/mocks/",
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !slices.ContainsFunc(targetPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}
		if slices.ContainsFunc(excludedPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		if strings.Contains(string(content), `"github.com/assurrussa/goadmin/infrastructure/goauth/model"`) {
			offenders = append(offenders, normalized)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin sources: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("runtime goadmin sources still import goauth/model:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestProjectionReposDoNotImportCanonicalAuthRepoPackages(t *testing.T) {
	t.Helper()

	targetPrefixes := []string{
		"infrastructure/pgsql/repositories/userrepo/",
		"infrastructure/pgsql/repositories/adminrepo/",
	}
	bannedSnippets := []string{
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/subjectrepo"`,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !slices.ContainsFunc(targetPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		source := string(content)
		for _, bannedSnippet := range bannedSnippets {
			if strings.Contains(source, bannedSnippet) {
				offenders = append(offenders, normalized+" -> "+bannedSnippet)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin projection repos: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("projection repos still import canonical auth repo packages directly:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestLegacyAuthUsecaseProvidersWereRemoved(t *testing.T) {
	t.Helper()

	if _, err := os.Stat("di/providers_auth_usecases.go"); !os.IsNotExist(err) {
		t.Fatalf("legacy auth usecase providers must be removed, stat error: %v", err)
	}
}

func TestRolesAssemblyDoesNotImportGoauthRolesImplementationPackagesDirectly(t *testing.T) {
	t.Helper()

	targets := []string{
		"di/providers.go",
		"di/providers_roles.go",
		"host/install.go",
	}
	bannedPrefixes := []string{
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/cache/`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/repository/postgres"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/service/guard"`, //nolint:goconst // required
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/service/roles"`, //nolint:goconst // required
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/usecases/`,      //nolint:goconst // required
	}

	var offenders []string
	for _, target := range targets {
		content, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read %s: %v", target, err)
		}

		source := string(content)
		for _, bannedPrefix := range bannedPrefixes {
			if strings.Contains(source, bannedPrefix) {
				offenders = append(offenders, target+" -> "+bannedPrefix)
			}
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("goadmin role assembly should consume integration/roles instead of role implementation packages directly:\n%s", strings.Join(offenders, "\n")) //nolint:lll // required
	}
}

func TestRoleRuntimeDoesNotImportGoauthRoleUsecasesDirectly(t *testing.T) {
	t.Helper()

	targetPrefixes := []string{
		"bootstrap/",
		"http/handlers/auth/",
		"http/handlers/permissions/",
		"http/handlers/roles/",
		"infrastructure/roles/adminroles/",
	}
	excludedPrefixes := []string{
		"http/handlers/auth/mocks/",
		"http/handlers/permissions/mocks/",
		"http/handlers/roles/mocks/",
	}
	bannedPrefixes := []string{
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/repository"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/service/guard"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/service/roles"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/usecases/`,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !slices.ContainsFunc(targetPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}
		if slices.ContainsFunc(excludedPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		source := string(content)
		for _, bannedPrefix := range bannedPrefixes {
			if strings.Contains(source, bannedPrefix) {
				offenders = append(offenders, normalized+" -> "+bannedPrefix)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin role runtime sources: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("goadmin role runtime should consume integration/roles instead of role usecase/service packages directly:\n%s", strings.Join(offenders, "\n")) //nolint:lll // required
	}
}

func TestRoleTestsAndMocksDoNotImportGoauthRoleUsecasesDirectly(t *testing.T) {
	t.Helper()

	targetPrefixes := []string{
		"http/handlers/auth/",
		"http/handlers/permissions/",
		"http/handlers/roles/",
		"infrastructure/roles/adminroles/",
	}
	bannedPrefixes := []string{
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/repository"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/service/guard"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/service/roles"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/usecases/`,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") && !strings.Contains(normalized, "/mocks/") {
			return nil
		}
		if !slices.ContainsFunc(targetPrefixes, func(prefix string) bool {
			return strings.HasPrefix(normalized, prefix)
		}) {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		source := string(content)
		for _, bannedPrefix := range bannedPrefixes {
			if strings.Contains(source, bannedPrefix) {
				offenders = append(offenders, normalized+" -> "+bannedPrefix)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin role tests and mocks: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("goadmin role tests and mocks should consume integration/roles instead of role usecase/service packages directly:\n%s", strings.Join(offenders, "\n")) //nolint:lll // required
	}
}

func TestGoadminDoesNotImportGoauthRoleDomainPackagesDirectly(t *testing.T) {
	t.Helper()

	bannedImports := []string{
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/model"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/seeders/rolesseed"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/domain/roles/shared"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/http/fiber/legacysession"`,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || normalized == "compatibility_imports_test.go" {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		source := string(content)
		for _, bannedImport := range bannedImports {
			if strings.Contains(source, bannedImport) {
				offenders = append(offenders, normalized+" -> "+bannedImport)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin sources: %v", err)
	}

	if len(offenders) > 0 {
		slices.Sort(offenders)
		t.Fatalf("goadmin should consume goauth/integration/roles instead of role domain packages directly:\n%s", strings.Join(offenders, "\n")) //nolint:lll // required
	}
}

func TestGoadminDoesNotImportGoauthStorageAdaptersDirectly(t *testing.T) {
	t.Helper()

	bannedImports := []string{
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/confirmationcoderepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/confirmationrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/emailchangerepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/identitylinkrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/oidcrefreshtokenrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/passwordresettokenrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/refreshtokenrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/sessionrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/pgsql/subjectrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/goauth/storage/redis/oidcstate"`,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || normalized == "compatibility_imports_test.go" {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		source := string(content)
		for _, bannedImport := range bannedImports {
			if strings.Contains(source, bannedImport) {
				offenders = append(offenders, normalized+" -> "+bannedImport)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin sources: %v", err)
	}

	if len(offenders) > 0 {
		slices.Sort(offenders)
		t.Fatalf("goadmin should consume goauth/integration/storage instead of storage adapter packages directly:\n%s", strings.Join(offenders, "\n")) //nolint:lll // required
	}
}

func TestRuntimeDoesNotImportRemovedCompatAuthRepos(t *testing.T) {
	t.Helper()

	bannedImports := []string{
		`"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/passwordresettokenrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminpasswordresettokenrepo"`,
		`"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminemailchangerepo"`,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipSourcePolicyDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		normalized := filepath.ToSlash(path)
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // required
		if readErr != nil {
			return readErr
		}

		source := string(content)
		for _, bannedImport := range bannedImports {
			if strings.Contains(source, bannedImport) {
				offenders = append(offenders, normalized+" -> "+bannedImport)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk goadmin runtime sources: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("runtime goadmin sources still import removed compat auth repos:\n%s", strings.Join(offenders, "\n"))
	}
}

package externalconsumer

// EmbeddingPackages are the stable packages used to embed goadmin into a host
// application process.
var EmbeddingPackages = [...]string{
	"github.com/assurrussa/goadmin/host",
	"github.com/assurrussa/goadmin/migrations",
}

// TestSupportPackages are stable packages intended for host/admin feature tests.
var TestSupportPackages = [...]string{
	"github.com/assurrussa/goadmin/hosttest",
}

// FeaturePackages are reusable admin feature packages supported for host
// projects. Future features must be added explicitly before they are stable.
var FeaturePackages = [...]string{
	"github.com/assurrussa/goadmin/features/operations",
	"github.com/assurrussa/goadmin/features/access",
	"github.com/assurrussa/goadmin/features/jobs",
	"github.com/assurrussa/goadmin/features/uploads",
	"github.com/assurrussa/goadmin/features/queues",
	"github.com/assurrussa/goadmin/features/notifications",
	"github.com/assurrussa/goadmin/features/realtime",
	"github.com/assurrussa/goadmin/features/authmail",
	"github.com/assurrussa/goadmin/features/users",
}

// ToolkitSupportPackages are stable helper packages for host-owned admin
// features.
var ToolkitSupportPackages = [...]string{
	"github.com/assurrussa/goadmin/toolkit/datagrid",
	"github.com/assurrussa/goadmin/toolkit/formvalidator",
}

var SupportedPackages = joinPackageGroups(
	EmbeddingPackages[:],
	TestSupportPackages[:],
	FeaturePackages[:],
	ToolkitSupportPackages[:],
)

var SupportedPackageCount = len(SupportedPackages)

func joinPackageGroups(groups ...[]string) []string {
	var count int
	for _, group := range groups {
		count += len(group)
	}

	packages := make([]string, 0, count)
	for _, group := range groups {
		packages = append(packages, group...)
	}

	return packages
}

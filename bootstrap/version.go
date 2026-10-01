package bootstrap

import "runtime/debug"

const (
	adminModulePath         = "github.com/assurrussa/goadmin"
	adminDevelopmentVersion = "dev"
)

func adminVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return adminDevelopmentVersion
	}

	return resolveAdminVersion(info)
}

// resolveAdminVersion selects the library version, never the host's version.
func resolveAdminVersion(info *debug.BuildInfo) string {
	if info.Main.Path == adminModulePath {
		return adminModuleVersion(&info.Main)
	}
	for _, dependency := range info.Deps {
		if dependency.Path == adminModulePath {
			return adminModuleVersion(dependency)
		}
	}

	return adminDevelopmentVersion
}

func adminModuleVersion(module *debug.Module) string {
	// A local replacement has no release version, even if require names a tag.
	if module.Replace != nil {
		module = module.Replace
	}
	if module.Version == "" || module.Version == "(devel)" {
		return adminDevelopmentVersion
	}

	return module.Version
}

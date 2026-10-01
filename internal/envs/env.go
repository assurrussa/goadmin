package envs

const (
	EnvProd  = "production"
	EnvStage = "stage"
	EnvDev   = "development"
	EnvLocal = "local"
)

func IsProd(env string) bool       { return env == EnvProd }
func IsStage(env string) bool      { return env == EnvStage }
func IsDev(env string) bool        { return env == EnvDev }
func IsLocalOrDev(env string) bool { return env == EnvLocal || IsDev(env) }

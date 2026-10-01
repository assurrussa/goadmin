package inertia

import (
	"context"
	"html/template"
	"io/fs"

	goinertia "github.com/assurrussa/goinertia"
	"github.com/gofiber/fiber/v3"
)

type (
	Inertia                                  = goinertia.Inertia
	Option                                   = goinertia.Option
	ValidationErrors                         = goinertia.ValidationErrors
	ValidationError                          = goinertia.ValidationError
	Error                                    = goinertia.Error
	FlashError                               = goinertia.FlashError
	FlashLevel                               = goinertia.FlashLevel
	LazyProp                                 = goinertia.LazyProp
	CSRFTokenProvider                        = goinertia.CSRFTokenProvider
	CSRFTokenCheckProvider                   = goinertia.CSRFTokenCheckProvider
	FiberSessionStore                        = goinertia.FiberSessionStore
	SessionAdapter[T FiberSessionStore]      = goinertia.SessionAdapter[T]
	FiberSessionAdapter[T FiberSessionStore] = goinertia.FiberSessionAdapter[T]
	Logger                                   = goinertia.Logger
)

const (
	HeaderInertia = goinertia.HeaderInertia
	HeaderVersion = goinertia.HeaderVersion

	ContextPropsCSRFToken = goinertia.ContextPropsCSRFToken

	FlashLevelSuccess = goinertia.FlashLevelSuccess
	FlashLevelInfo    = goinertia.FlashLevelInfo
	FlashLevelWarning = goinertia.FlashLevelWarning
	FlashLevelError   = goinertia.FlashLevelError
)

func New(baseURL string, opts ...Option) *Inertia {
	return goinertia.New(baseURL, opts...)
}

func NewWithValidation(baseURL string, opts ...Option) (*Inertia, error) {
	return goinertia.NewWithValidation(baseURL, opts...)
}

func NewError(code int, message string, errs ...error) *Error {
	return goinertia.NewError(code, message, errs...)
}

func NewValidationError(code int, userMessage string, errs ValidationErrors) *ValidationError {
	return goinertia.NewValidationError(code, userMessage, errs)
}

func NewFlashError(level FlashLevel, userMessage string) *FlashError {
	return goinertia.NewFlashError(level, userMessage)
}

func Redirect(c fiber.Ctx, url string) error {
	return goinertia.Redirect(c, url)
}

func NewFiberSessionAdapter[T FiberSessionStore](store SessionAdapter[T]) *FiberSessionAdapter[T] {
	return goinertia.NewFiberSessionAdapter[T](store)
}

func WithFS(files fs.FS) Option {
	return goinertia.WithFS(files)
}

func WithPublicFS(files fs.ReadFileFS) Option {
	return goinertia.WithPublicFS(files)
}

func WithRootTemplate(rootTemplate string) Option {
	return goinertia.WithRootTemplate(rootTemplate)
}

func WithRootHotTemplate(rootHotTemplate string) Option {
	return goinertia.WithRootHotTemplate(rootHotTemplate)
}

func WithRootErrorTemplate(rootErrorTemplate string) Option {
	return goinertia.WithRootErrorTemplate(rootErrorTemplate)
}

func WithAssetVersion(assetVersion string) Option {
	return goinertia.WithAssetVersion(assetVersion)
}

func WithLogger(logger Logger) Option {
	return goinertia.WithLogger(logger)
}

func WithSetSharedFuncMap(data template.FuncMap) Option {
	return goinertia.WithSetSharedFuncMap(data)
}

func WithSharedProps(data map[string]any) Option {
	return goinertia.WithSharedProps(data)
}

func WithCSRFPropName(prop string) Option {
	return goinertia.WithCSRFPropName(prop)
}

func WithCanExposeDetails(fn func(ctx context.Context, headers map[string][]string) bool) Option {
	return goinertia.WithCanExposeDetails(fn)
}

func WithCustomErrorGettingHandler(fn func(err error) *Error) Option {
	return goinertia.WithCustomErrorGettingHandler(fn)
}

func WithCustomErrorDetailsHandler(fn func(errReturn *Error, isCanDetails bool) string) Option {
	return goinertia.WithCustomErrorDetailsHandler(fn)
}

func WithDevMode() Option {
	return goinertia.WithDevMode()
}

func WithSessionStore(sessionStore goinertia.SessionStore) Option {
	return goinertia.WithSessionStore(sessionStore)
}

func WithCSRFTokenProvider(provider CSRFTokenProvider) Option {
	return goinertia.WithCSRFTokenProvider(provider)
}

func WithCSRFTokenCheckProvider(provider CSRFTokenCheckProvider) Option {
	return goinertia.WithCSRFTokenCheckProvider(provider)
}

package testsupport

import (
	goinertia "github.com/assurrussa/goinertia"
	"github.com/assurrussa/goinertia/inertiat"
)

type (
	MockSessionStore = inertiat.MockSessionStore
	TestApp          = inertiat.TestApp
)

func NewForTest(baseURL string, opts ...goinertia.Option) *goinertia.Inertia {
	return inertiat.NewForTest(baseURL, opts...)
}

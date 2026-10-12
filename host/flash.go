package host

import (
	"errors"

	"github.com/assurrussa/goinertia"
	"github.com/gofiber/fiber/v3"

	coreinertia "github.com/assurrussa/goadmin/infrastructure/core/inertia"
)

// PersistFlash saves only Inertia's redirect-surviving flash values. Public
// extension routes that finish before the core Inertia middleware must call it
// after setting flash and before redirecting. Ordinary protected routes already
// persist through the middleware and do not need this method.
// The native session bridge follows a newly issued authentication cookie.
func (a *App) PersistFlash(c fiber.Ctx) error {
	if a == nil || a.inner == nil || a.inner.HTTPManager() == nil || a.inner.Session() == nil {
		return errors.New("admin flash persistence is unavailable")
	}
	data := a.inner.HTTPManager().State(c).FlashToPersist()
	if len(data) == 0 {
		return nil
	}
	bridge := coreinertia.SessionBridge{Store: a.inner.Session()}
	return bridge.Flash(c, string(goinertia.ContextKeyProps), data)
}

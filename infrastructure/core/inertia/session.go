package inertia

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/valyala/fasthttp"
)

// SessionBridge follows the cookie issued by authentication in this request.
// Fiber 3.5 Store.Get retains the pre-Regenerate ID in request locals; reloading
// that ID while persisting a redirect's flash would replace the authenticated
// cookie with a new anonymous session.
type SessionBridge struct{ Store *session.Store }

func (b SessionBridge) load(c fiber.Ctx) (*session.Session, error) {
	name := b.Store.Extractor.Key
	if name == "" {
		name = "session_id"
	}
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(name)
	if c.Response().Header.Cookie(cookie) && len(cookie.Value()) != 0 {
		return b.Store.GetByID(c, string(cookie.Value()))
	}
	return b.Store.Get(c)
}

func (b SessionBridge) Get(c fiber.Ctx, key string) (any, error) {
	sess, err := b.load(c)
	if err != nil {
		return nil, err
	}
	defer sess.Release()
	return sess.Get(key), nil
}

func (b SessionBridge) Set(c fiber.Ctx, key string, value any) error {
	sess, err := b.load(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	sess.Set(key, value)
	return sess.SaveWithContext(c)
}

func (b SessionBridge) Delete(c fiber.Ctx, key string) error {
	sess, err := b.load(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	sess.Delete(key)
	return sess.SaveWithContext(c)
}

func (b SessionBridge) Flash(c fiber.Ctx, key string, value any) error { return b.Set(c, key, value) }

func (b SessionBridge) GetFlash(c fiber.Ctx, key string) (any, error) {
	sess, err := b.load(c)
	if err != nil {
		return nil, err
	}
	defer sess.Release()
	value := sess.Get(key)
	if value != nil {
		sess.Delete(key)
		if err := sess.SaveWithContext(c); err != nil {
			return nil, err
		}
	}
	return value, nil
}

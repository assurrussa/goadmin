package adminservice

import (
	"errors"
	"reflect"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
)

// Fiber v3.5 validates this key in Store.GetByID but exposes no deadline
// accessor. Read the same typed entry through the public session methods so the
// PostgreSQL receipt check can use the exact cutoff after GetByID has returned.
func fiberAbsoluteExpiry(sess *session.Session, timeout time.Duration) (time.Time, error) {
	if timeout <= 0 {
		return time.Time{}, nil
	}
	fiberPackage := reflect.TypeFor[session.Session]().PkgPath()
	for _, key := range sess.Keys() {
		kind := reflect.TypeOf(key)
		if kind == nil || kind.PkgPath() != fiberPackage || kind.Name() != "absExpirationKeyType" {
			continue
		}
		expiry, ok := sess.Get(key).(time.Time)
		if kind.Kind() == reflect.Int && reflect.ValueOf(key).Int() == 0 && ok && !expiry.IsZero() {
			return expiry, nil
		}
	}
	return time.Time{}, errors.New("browser session absolute expiry is unavailable")
}

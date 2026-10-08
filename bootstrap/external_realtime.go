package bootstrap

import (
	"time"

	"github.com/assurrussa/gowebsocket/websocketstream"
	libwebsocket "github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/models"
)

// externalDeadlineUpgrader keeps the existing bounded stream implementation.
// It copies only the immutable proof deadline before Fiber releases its pooled
// request context. Already-open external sockets close at that deadline even
// when the provider is offline, silent, or no further HTTP request occurs.
// Reconnect uses the ordinary authentication boundary and fresh live authority.
type externalDeadlineUpgrader struct{ websocketstream.Upgrader }

func (u externalDeadlineUpgrader) UpgradeFastHTTP(ctx *fasthttp.RequestCtx, handler libwebsocket.FastHTTPHandler) error {
	var deadline time.Time
	if admin, ok := ctx.UserValue(sessioncore.AuthAdminKey.String()).(*models.SessionAdmin); ok && admin != nil {
		deadline = admin.ExternalValidUntil
	}
	return u.Upgrader.UpgradeFastHTTP(ctx, func(ws *libwebsocket.Conn) {
		if !deadline.IsZero() {
			if !time.Now().Before(deadline) {
				_ = ws.Close()
				return
			}
			timer := time.AfterFunc(time.Until(deadline), func() { _ = ws.Close() })
			defer timer.Stop()
		}
		handler(ws)
	})
}

func (u externalDeadlineUpgrader) ReadLimit() int64 {
	if known, ok := u.Upgrader.(interface{ ReadLimit() int64 }); ok {
		return known.ReadLimit()
	}
	return 0
}

package bootstrap

import (
	"time"

	"github.com/assurrussa/gowebsocket/websocketstream"
	libwebsocket "github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/realtimesession"
	"github.com/assurrussa/goadmin/models"
)

// externalDeadlineUpgrader keeps the existing bounded stream implementation.
// It copies only the immutable proof deadline before Fiber releases its pooled
// request context. Already-open external sockets close at that deadline even
// when the provider is offline, silent, or no further HTTP request occurs.
// Reconnect uses the ordinary authentication boundary and fresh live authority.
type externalDeadlineUpgrader struct{ websocketstream.Upgrader }

type realtimeAdmissionKey struct{}

func (u externalDeadlineUpgrader) UpgradeFastHTTP(ctx *fasthttp.RequestCtx, handler libwebsocket.FastHTTPHandler) error {
	admin, ok := ctx.UserValue(sessioncore.AuthAdminKey.String()).(*models.SessionAdmin)
	admission, _ := ctx.UserValue(realtimeAdmissionKey{}).(*realtimesession.Admission)
	if !ok || admin == nil {
		return rejectRealtimeUpgrade(ctx)
	}
	lease := admission.Bind(realtimesession.Key{SubjectID: admin.SubjectID, AuthSessionID: admin.AuthSessionID}, 5*time.Second)
	if lease == nil {
		return rejectRealtimeUpgrade(ctx)
	}
	deadline := admin.ExternalValidUntil
	handedOff := false
	defer func() {
		if !handedOff {
			lease.Release()
		}
	}()
	err := u.Upgrader.UpgradeFastHTTP(ctx, func(ws *libwebsocket.Conn) {
		if !lease.Attach(func() { _ = ws.Close() }) {
			_ = ws.Close()
			return
		}
		defer lease.Release()
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
	handedOff = err == nil
	return err
}

func rejectRealtimeUpgrade(ctx *fasthttp.RequestCtx) error {
	ctx.Error("Authentication required", fasthttp.StatusUnauthorized)
	ctx.Response.Header.Set("Cache-Control", "no-store")
	return libwebsocket.HandshakeError{}
}

func (u externalDeadlineUpgrader) ReadLimit() int64 {
	if known, ok := u.Upgrader.(interface{ ReadLimit() int64 }); ok {
		return known.ReadLimit()
	}
	return 0
}

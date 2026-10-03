# WebSocket client contract

Enable `features/realtime.New(nil)` in `host.New` for a runtime-owned stream, or
pass a `host.EventStream` supervised by the host. The dependency is pinned to
`github.com/assurrussa/gowebsocket v0.2.1`; ordinary consumer checks resolve the
published dependency. `GOWEBSOCKET_LOCAL_PATH` is an explicit development override.

## Connection and authentication

Connect to `GET /ws` on the admin origin using the `tgmulti-service-protocol`
subprotocol. Use `wss:` when the admin page uses HTTPS. The existing admin browser
session cookies authenticate the connection. Current canonical auth and admin
membership are checked before upgrading to HTTP 101; `WithUserIDExtractor`
passes only the authenticated admin projection UUID to the event stream.
Client-supplied user IDs do not determine subscription identity. The configured
server origin allowlist also applies to the handshake.

```ts
const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
const socket = new WebSocket(`${protocol}//${location.host}/ws`, 'tgmulti-service-protocol')
socket.onmessage = ({ data }) => {
  const event = JSON.parse(data)
  // Dispatch using event.eventType and the corresponding event schema.
}
```

## Messages

Application frames contain plain UTF-8 JSON text. Do not encode JSON in Base64
or send binary frames. No versioned envelope is enabled. All events contain
`eventType`; their nonzero UUID identity field follows the model:

- `files.upload.status` uses `eventId`, with `taskId`, `status` and optional
  file/error/metadata fields.
- `file.deleted` uses `id`, with `fileId`, `filePath`, `status` and optional `error`.
- `file.after.process` uses `id`, with `fileId`, `filePath`, `status`,
  `eventTrigger` and `error`.

For example, an after-process notification retains its existing schema:

```json
{"id":"e7366c15-61b2-4f68-b97a-f6cdd951b11c","eventType":"file.after.process","fileId":42,"filePath":"https://admin.example.com/uploads/file.png","status":"completed","eventTrigger":"avatar","error":""}
```

The built-in stream delivers notifications to browsers. It registers no inbound
business commands. Incoming JSON must select a known, valid event model and an
installed processor; unsupported input closes the connection. Use the admin
HTTP APIs for actions. Ping/pong uses WebSocket control frames, not JSON commands.

The pinned handler limits incoming wire messages to 96 KiB and decoded payloads
to 64 KiB. Output is bounded to 96 KiB; the default in-memory stream accepts
events up to 64 KiB and queues at most 64 events / 1 MiB per subscription.

## Closure and recovery

- `1003` — unsupported binary application frame.
- `1008` — invalid JSON/Base64 input, unknown or invalid event, or unsupported command.
- `1009` — message/payload size limit exceeded.
- `1013` — processing overload or terminated delivery subscription, including
  eviction of a slow subscriber when its queue fills.
- `1000` — normal teardown, including handler shutdown.
- `1011` — internal failure; transport failures can also end without a close frame.

After a transient disconnect, reconnect with a delay and reload current state
from HTTP APIs. Delivery is local and best-effort: no replay, acknowledgement or
offline persistence is provided. Fix invalid input before retrying policy/size
failures. The embedded client currently retries disconnected sessions every 5s;
HTTP upload polling remains available when realtime is disabled.

The session is authenticated at handshake. Existing sockets are not automatically
reauthorized on logout, membership changes or token expiry; immediate revocation
requires a separate connection revocation mechanism in the host integration.

## Server lifetime

`host.ServerConfig` and `host.ServerConfigInput` expose `ReadTimeout`,
`WriteTimeout` and `IdleTimeout`. Zero selects 10s, 10s and 120s respectively;
negative values are rejected. Hosts can choose longer bounded values for their
HTTP upload/response workload. These HTTP deadlines complement the handler's
own message, ping/pong and write limits.

Cancel and join `Runtime.Run`, then call `Runtime.Close`. Runtime drains the
WebSocket handler before its owned stream and HTTP listener. Close also handles
an assembled runtime that never started. A supplied stream remains host-owned;
call its `Shutdown(ctx)` after the admin handler has stopped, using a fresh
deadline context rather than the canceled signal context.

For direct server management, `Server.Shutdown(ctx)` reports handler/stream and
HTTP shutdown errors. Fiber `App.Shutdown` also triggers a bounded fallback hook,
but Fiber logs hook errors instead of returning them. Do not treat a timed-out
shutdown as proof that all callbacks have finished.

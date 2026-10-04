package bootstrap

import (
	"mime"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
)

func responseCompression(hasPublicFS bool) fiber.Handler {
	// fasthttp's streaming compressor can release a static file reader before
	// its compression goroutine finishes when a client disconnects. Keep owned
	// public GET/HEAD identity-only; compression outside that scope is unchanged.
	return compress.New(compress.Config{Next: func(c fiber.Ctx) bool {
		return hasPublicFS && publicAssetRequest(c)
	}})
}

func publicAssetRequest(c fiber.Ctx) bool {
	if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
		return false
	}
	// Path applies Fiber's UnescapePath setting, just like route matching.
	// Do not normalize it separately or expand the /public segment boundary.
	const prefix = "/public"
	requestPath := c.Path()
	if len(requestPath) < len(prefix) || (len(requestPath) > len(prefix) && requestPath[len(prefix)] != '/') {
		return false
	}
	// Only the fixed-width ASCII prefix may fold; Fiber does not lowercase
	// Unicode lookalikes when CaseSensitive is false.
	prefixPath := requestPath[:len(prefix)]
	return prefixPath == prefix || (!c.App().Config().CaseSensitive && strings.EqualFold(prefixPath, prefix))
}

func publicAssetResponse(c fiber.Ctx) error {
	status := c.Response().StatusCode()
	if status != fiber.StatusOK && status != fiber.StatusPartialContent && status != fiber.StatusNotModified {
		return nil
	}
	if acceptsPublicIdentity(c) {
		return nil
	}
	// Negotiate only after finding an asset. Missing/forbidden files still fall
	// through the existing authentication and error routes, without a new 406.
	// No asynchronous compressor owns this body, so closing it here is safe.
	c.Response().ResetBody()
	c.Response().Header.SetContentLength(0)
	for _, header := range []string{
		fiber.HeaderContentType, fiber.HeaderLastModified, fiber.HeaderETag,
		fiber.HeaderAcceptRanges, fiber.HeaderContentRange,
	} {
		c.Response().Header.Del(header)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Status(fiber.StatusNotAcceptable)
	return nil
}

func acceptsPublicIdentity(c fiber.Ctx) bool {
	// Identity is implicitly acceptable. Only exact identity/* coding tokens
	// can exclude it; other codings (including unknown ones) have no effect.
	identitySeen, identityAllowed, wildcardAllowed := false, true, true
	for part := range strings.SplitSeq(c.AcceptEncoding(), ",") {
		coding, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil || (coding != "identity" && coding != "*") {
			continue
		}
		quality := 1.0
		if value, ok := params["q"]; ok {
			quality, err = strconv.ParseFloat(value, 64)
			if err != nil || !(quality >= 0 && quality <= 1) {
				continue
			}
		}
		if coding == "identity" {
			identitySeen = true
			identityAllowed = identityAllowed && quality > 0
		} else {
			wildcardAllowed = wildcardAllowed && quality > 0
		}
	}
	if identitySeen {
		return identityAllowed
	}
	return wildcardAllowed
}

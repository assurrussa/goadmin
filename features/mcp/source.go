package mcp

import (
	"errors"
	"net/netip"

	"github.com/gofiber/fiber/v3"
)

func ParseTrustedProxy(raw string) (netip.Addr, error) {
	if raw == "" {
		return netip.Addr{}, nil
	}
	ip, ok := pairingIP(raw)
	if !ok {
		return netip.Addr{}, errors.New("mcp: configure one trusted proxy IP address")
	}
	return ip, nil
}

func pairingIP(raw string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(raw)
	if err != nil || ip.Zone() != "" {
		return netip.Addr{}, false
	}
	ip = ip.Unmap()
	return ip, !ip.IsUnspecified() && !ip.IsMulticast()
}

// Only the explicitly trusted socket peer can supply the client address. It
// must overwrite X-Real-IP with exactly one address on every request.
func (h *resourceHandler) pairingSource(c fiber.Ctx) (netip.Addr, bool) {
	peer, ok := netip.AddrFromSlice(c.RequestCtx().RemoteIP())
	if !ok {
		return netip.Addr{}, false
	}
	peer = peer.Unmap()
	if peer != h.trustedProxy {
		return peer, true
	}
	values := c.Request().Header.PeekAll("X-Real-IP")
	if len(values) != 1 {
		return netip.Addr{}, false
	}
	return pairingIP(string(values[0]))
}

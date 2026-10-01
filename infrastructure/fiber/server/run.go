package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/gofiber/fiber/v3"
)

// Run serves until cancellation or listener completion. A supplied listener is
// owned by the server; as in Fiber, prefork is only available without one.
func (s *Server) Run(ctx context.Context, listens ...net.Listener) error {
	defer s.shutdown.Close()
	if ctx.Err() != nil {
		shutdownErr := s.shutdownWithTimeout(ctx)
		if len(listens) > 0 {
			return errors.Join(shutdownErr, listens[0].Close())
		}
		return shutdownErr
	}

	if s.enablePrefork && len(listens) == 0 {
		return errors.Join(s.runPrefork(ctx), s.shutdownWithTimeout(ctx))
	}

	listener, err := s.listener(ctx, listens)
	if err != nil {
		return errors.Join(fmt.Errorf("listen and serve: %w", err), s.shutdownWithTimeout(ctx))
	}
	defer func() { _ = listener.Close() }()

	ready := make(chan struct{})
	done := make(chan struct{})
	stopped := make(chan error, 1)
	go func() {
		select {
		case <-done:
			s.shutdown.Close()
			stopped <- s.shutdownWithTimeout(ctx)
			return
		case <-ctx.Done():
			s.shutdown.Close()
		}
		// Wait until Serve has registered its listener before Shutdown. This
		// avoids a cancellation racing ahead of Fiber/fasthttp startup.
		select {
		case <-done:
			stopped <- s.shutdownWithTimeout(ctx)
		case <-ready:
			stopped <- s.shutdownWithTimeout(ctx)
		}
	}()

	s.lg.InfoContext(ctx, "listen and serve", slog.String("listen", listener.Addr().String()))
	err = s.App.Listener(&readyListener{Listener: listener, ready: ready}, fiber.ListenConfig{
		DisableStartupMessage: s.disableStartupMessage,
	})
	close(done)
	shutdownErr := <-stopped
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Join(fmt.Errorf("listen and serve: %w", err), shutdownErr)
	}
	return shutdownErr
}

func (s *Server) listener(ctx context.Context, listens []net.Listener) (net.Listener, error) {
	if len(listens) > 0 {
		return listens[0], nil
	}
	if s.tlsCert != "" && s.tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(s.tlsCert, s.tlsKey)
		if err != nil {
			return nil, err
		}
		tlsHandler := &fiber.TLSHandler{}
		s.App.SetTLSHandler(tlsHandler)
		return tls.Listen("tcp4", s.addr, &tls.Config{
			GetCertificate: tlsHandler.GetClientInfo,
			MinVersion:     tls.VersionTLS12,
			Certificates:   []tls.Certificate{cert},
		})
	}
	var config net.ListenConfig
	return config.Listen(ctx, "tcp4", s.addr)
}

func (s *Server) runPrefork(ctx context.Context) error {
	// Fiber owns prefork worker processes. GracefulContext is forwarded, but
	// prefork master shutdown remains subject to Fiber's process lifecycle.
	err := s.App.Listen(s.addr, fiber.ListenConfig{
		DisableStartupMessage: s.disableStartupMessage,
		EnablePrefork:         true,
		GracefulContext:       ctx,
		ShutdownTimeout:       shutdownTimeout,
		CertFile:              s.tlsCert,
		CertKeyFile:           s.tlsKey,
	})
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

type readyListener struct {
	net.Listener
	ready chan struct{}
	once  sync.Once
}

func (l *readyListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.ready) })
	return l.Listener.Accept()
}

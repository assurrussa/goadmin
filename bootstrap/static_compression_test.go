package bootstrap //nolint:testpackage // exercises the owned middleware and static mount together

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/public"
)

const (
	publicEntryFile      = "dist/js/app.js"
	publicStyleFile      = "dist/css/app.css"
	publicEntryPath      = "/public/" + publicEntryFile
	publicStylePath      = "/public/" + publicStyleFile
	publicGzip           = "gzip"
	publicRefuseIdentity = "gzip, identity;q=0"
)

func staticPolicyApp(files fs.ReadFileFS, config fiber.Config) *fiber.App {
	app := fiber.New(config)
	app.Use(responseCompression(files != nil))
	app.Use(func(c fiber.Ctx) error {
		c.Vary("Origin")
		c.Set("X-Test-Security", "retained")
		return c.Next()
	})
	registerStaticRoutes(app, files, nil, nil, "", nil, nil, func(c fiber.Ctx) error {
		if c.Get("X-Test-Admin") != testAuthorizedHeader {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.Next()
	})
	app.Get("/publicity", func(c fiber.Ctx) error { return c.SendString(strings.Repeat("dynamic ", 100)) })
	app.Get("/auth/example", func(c fiber.Ctx) error { return c.SendString(strings.Repeat("auth ", 100)) })
	return app
}

func publicPolicyResponse(t *testing.T, app *fiber.App, method, target string, headers http.Header) staticTestResponse {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	request.Header = headers
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return staticTestResponse{status: response.StatusCode, header: response.Header, body: body}
}

func TestPublicIdentityNegotiation(t *testing.T) {
	app := staticPolicyApp(public.Files, fiber.Config{CaseSensitive: true, StrictRouting: true})
	expected, err := public.Files.ReadFile(publicEntryFile)
	require.NoError(t, err)
	for _, test := range []struct {
		encoding []string
		status   int
	}{
		{nil, http.StatusOK},
		{[]string{""}, http.StatusOK},
		{[]string{"identity"}, http.StatusOK},
		{[]string{publicGzip}, http.StatusOK},
		{[]string{"br, gzip, deflate, zstd"}, http.StatusOK},
		{[]string{"gzip;q=0"}, http.StatusOK},
		{[]string{"*"}, http.StatusOK},
		{[]string{publicRefuseIdentity}, http.StatusNotAcceptable},
		{[]string{"*;q=0"}, http.StatusNotAcceptable},
		{[]string{"*;q=1, identity;q=0"}, http.StatusNotAcceptable},
		{[]string{"*;q=0, identity;q=0.1"}, http.StatusOK},
		{[]string{publicGzip, "IDENTITY; Q=0.000"}, http.StatusNotAcceptable},
		{[]string{"*;q=0", "identity;q=1"}, http.StatusOK},
		{[]string{"*;q=0.5, i*;q=0"}, http.StatusOK},
		{[]string{"identity;q=1", "identity;q=0"}, http.StatusNotAcceptable},
	} {
		t.Run(strings.Join(test.encoding, "|"), func(t *testing.T) {
			got := publicPolicyResponse(t, app, http.MethodGet, publicEntryPath,
				http.Header{fiber.HeaderAcceptEncoding: test.encoding})
			require.Equal(t, test.status, got.status)
			require.Empty(t, got.header.Get(fiber.HeaderContentEncoding))
			require.Contains(t, got.header.Values(fiber.HeaderVary), "Origin, Accept-Encoding")
			require.Equal(t, "retained", got.header.Get("X-Test-Security"))
			if test.status == http.StatusOK {
				require.Equal(t, expected, got.body)
				require.Contains(t, got.header.Get(fiber.HeaderContentType), "javascript")
				require.Equal(t, "no-cache, must-revalidate", got.header.Get(fiber.HeaderCacheControl))
			} else {
				require.Empty(t, got.body)
				require.Equal(t, "no-store", got.header.Get(fiber.HeaderCacheControl))
			}
			require.Empty(t, got.header.Get(fiber.HeaderLastModified))
		})
	}
}

func TestPublicIdentityHTTPContract(t *testing.T) {
	modified := time.Date(2026, time.October, 1, 10, 0, 0, 0, time.UTC)
	files := fstest.MapFS{
		publicEntryFile: &fstest.MapFile{Data: []byte("entry bytes"), ModTime: modified},
		publicStyleFile: &fstest.MapFile{Data: []byte("style bytes"), ModTime: modified},
		"chunk.js":      &fstest.MapFile{Data: []byte("chunk bytes"), ModTime: modified},
	}
	app := staticPolicyApp(files, fiber.Config{CaseSensitive: true, StrictRouting: true})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range []string{publicEntryPath, publicStylePath} {
			for _, extra := range []http.Header{
				{},
				{"Range": {"bytes=0-3"}},
				{"Range": {"bytes=999-"}},
				{"If-Modified-Since": {modified.Format(http.TimeFormat)}},
				{"If-None-Match": {`"old-tag"`}},
				{"Cache-Control": {"no-transform"}},
			} {
				extra.Set(fiber.HeaderAcceptEncoding, publicGzip)
				got := publicPolicyResponse(t, app, method, path, extra)
				require.Equal(t, http.StatusOK, got.status, "%s %s %v", method, path, extra)
				require.Empty(t, got.header.Get(fiber.HeaderContentEncoding))
				require.Empty(t, got.header.Get(fiber.HeaderLastModified))
				require.Empty(t, got.header.Get(fiber.HeaderContentRange))
				require.Empty(t, got.header.Get(fiber.HeaderAcceptRanges))
				require.Empty(t, got.header.Get(fiber.HeaderETag))
				require.Equal(t, "no-cache, must-revalidate", got.header.Get(fiber.HeaderCacheControl))
				require.Equal(t, "11", got.header.Get(fiber.HeaderContentLength))
				switch {
				case method == http.MethodHead:
					require.Empty(t, got.body)
				case path == publicEntryPath:
					require.Equal(t, "entry bytes", string(got.body))
					require.Contains(t, got.header.Get(fiber.HeaderContentType), "javascript")
				default:
					require.Equal(t, "style bytes", string(got.body))
					require.Contains(t, got.header.Get(fiber.HeaderContentType), "text/css")
				}
			}
		}
	}
	for _, encoding := range []string{publicGzip, "identity;q=0"} {
		got := publicPolicyResponse(t, app, http.MethodGet, "/public/chunk.js", http.Header{
			fiber.HeaderAcceptEncoding: {encoding}, "If-Modified-Since": {modified.Format(http.TimeFormat)},
		})
		require.Contains(t, got.header.Get(fiber.HeaderVary), fiber.HeaderAcceptEncoding)
		if encoding == publicGzip {
			require.Equal(t, http.StatusNotModified, got.status)
			require.Empty(t, got.header.Get(fiber.HeaderLastModified))
		} else {
			require.Equal(t, http.StatusNotAcceptable, got.status)
			require.Empty(t, got.header.Get(fiber.HeaderLastModified))
		}
		require.Empty(t, got.body)
	}
}

func TestPublicIdentityRoutingAndAuthBoundary(t *testing.T) {
	app := staticPolicyApp(public.Files, fiber.Config{CaseSensitive: true, StrictRouting: true})
	for _, path := range []string{
		publicEntryPath, "/public/dist/js/%61pp.js", "/public/dist/./js/app.js",
		"/public/dist/css/../js/app.js", "/public//dist/js/app.js", publicEntryPath + "/",
		"/public/%2564ist/js/app.js",
	} {
		got := publicPolicyResponse(t, app, http.MethodGet, path, http.Header{fiber.HeaderAcceptEncoding: {publicGzip}})
		require.Equal(t, http.StatusOK, got.status, path)
		require.Empty(t, got.header.Get(fiber.HeaderContentEncoding), path)
	}
	for _, path := range []string{
		"/public", "/public/", "/public/missing.js", "/PUBLIC/dist/js/app.js",
		"/publicity", "/public%2fdist/js/app.js", "/%70ublic/dist/js/app.js",
	} {
		for _, encoding := range []string{publicGzip, publicRefuseIdentity} {
			got := publicPolicyResponse(t, app, http.MethodGet, path, http.Header{fiber.HeaderAcceptEncoding: {encoding}})
			require.Equal(t, http.StatusUnauthorized, got.status, "%s %s", path, encoding)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		got := publicPolicyResponse(t, app, method, "/public/missing.js", http.Header{
			fiber.HeaderAcceptEncoding: {publicRefuseIdentity}, "X-Test-Admin": {testAuthorizedHeader},
		})
		require.Equal(t, http.StatusNotFound, got.status, method)
	}
	for _, path := range []string{"/publicity", "/auth/example"} {
		got := publicPolicyResponse(t, app, http.MethodGet, path, http.Header{
			fiber.HeaderAcceptEncoding: {publicGzip}, "X-Test-Admin": {testAuthorizedHeader},
		})
		require.Equal(t, http.StatusOK, got.status)
		require.Equal(t, publicGzip, got.header.Get(fiber.HeaderContentEncoding), path)
		reader, err := gzip.NewReader(bytes.NewReader(got.body))
		require.NoError(t, err)
		decoded, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.Greater(t, len(decoded), 200)
	}
}

func TestPublicIdentityMatchesFiberPathSettings(t *testing.T) {
	app := staticPolicyApp(public.Files, fiber.Config{UnescapePath: true})
	for _, path := range []string{"/PUBLIC/dist/js/app.js", "/%70ublic/dist/js/app.js", "/public%2fdist/js/app.js"} {
		got := publicPolicyResponse(t, app, http.MethodGet, path, http.Header{fiber.HeaderAcceptEncoding: {publicGzip}})
		require.Equal(t, http.StatusOK, got.status, path)
		require.Empty(t, got.header.Get(fiber.HeaderContentEncoding), path)
	}
}

func TestPublicIdentityEarlyDisconnect(t *testing.T) {
	app := staticPolicyApp(public.Files, fiber.Config{
		CaseSensitive: true, StrictRouting: true,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: time.Second,
	})
	address := startPublicPolicyListener(t, app)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	expected, err := public.Files.ReadFile(publicEntryFile)
	require.NoError(t, err)
	require.Greater(t, len(expected), 256*1024, "exercise early disconnect while streaming the real embedded client")
	for _, encoding := range []string{"", publicGzip, "br", "deflate", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			for range 20 {
				response := publicWireRequest(t, client, address+publicEntryPath, encoding)
				require.Equal(t, http.StatusOK, response.StatusCode)
				// Abandon the real HTTP stream immediately after headers. Do not
				// drain: this is the original upstream lifetime-race trigger.
				require.NoError(t, response.Body.Close())
				require.False(t, response.Uncompressed)
				require.Empty(t, response.Header.Get(fiber.HeaderContentEncoding))
			}
			response := publicWireRequest(t, client, address+publicEntryPath, encoding)
			defer response.Body.Close()
			body, readErr := io.ReadAll(io.LimitReader(response.Body, int64(len(expected))+1))
			require.NoError(t, readErr)
			require.Equal(t, expected, body)
		})
	}
}

func publicWireRequest(t *testing.T, client *http.Client, target, encoding string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	require.NoError(t, err)
	if encoding != "" {
		request.Header.Set(fiber.HeaderAcceptEncoding, encoding)
	}
	response, err := client.Do(request)
	require.NoError(t, err)
	return response
}

func startPublicPolicyListener(t *testing.T, app *fiber.App) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		require.NoError(t, app.ShutdownWithContext(ctx))
		select {
		case serveErr := <-done:
			require.NoError(t, serveErr)
		case <-ctx.Done():
			t.Error("owned public asset listener did not stop")
		}
	})
	return "http://" + listener.Addr().String()
}

func TestPublicIdentityRefusedHeadAndPreservedErrors(t *testing.T) {
	app := staticPolicyApp(public.Files, fiber.Config{CaseSensitive: true, StrictRouting: true})
	got := publicPolicyResponse(t, app, http.MethodHead, publicEntryPath,
		http.Header{fiber.HeaderAcceptEncoding: {publicRefuseIdentity}})
	require.Equal(t, http.StatusNotAcceptable, got.status)
	require.Empty(t, got.body)
	require.Equal(t, "0", got.header.Get(fiber.HeaderContentLength))
	for _, status := range []int{
		fiber.StatusFound, fiber.StatusUnauthorized, fiber.StatusForbidden,
		fiber.StatusNotFound, fiber.StatusRequestedRangeNotSatisfiable, fiber.StatusInternalServerError,
	} {
		app := fiber.New()
		app.Get("/", func(c fiber.Ctx) error {
			c.Response().SetStatusCode(status)
			c.Response().SetBodyString("original response")
			return publicAssetResponse(c)
		})
		got := publicPolicyResponse(t, app, http.MethodGet, "/",
			http.Header{fiber.HeaderAcceptEncoding: {publicRefuseIdentity}})
		require.Equal(t, status, got.status)
		require.Equal(t, "original response", string(got.body))
	}
}

func TestPublicIdentityPreservesWildcardVary(t *testing.T) {
	app := fiber.New()
	app.Use(responseCompression(true))
	app.Use(func(c fiber.Ctx) error { c.Set(fiber.HeaderVary, "*"); return c.Next() })
	registerStaticRoutes(app, public.Files, nil, nil, "", nil, nil)
	got := publicPolicyResponse(t, app, http.MethodGet, publicEntryPath,
		http.Header{fiber.HeaderAcceptEncoding: {publicGzip}})
	require.Equal(t, "*", got.header.Get(fiber.HeaderVary))
}

func TestPublicIdentityDoesNotChangeAbsentMount(t *testing.T) {
	app := fiber.New()
	app.Use(responseCompression(false))
	app.Get(publicEntryPath, func(c fiber.Ctx) error { return c.SendString(strings.Repeat("dynamic ", 100)) })
	got := publicPolicyResponse(t, app, http.MethodGet, publicEntryPath,
		http.Header{fiber.HeaderAcceptEncoding: {publicGzip}})
	require.Equal(t, publicGzip, got.header.Get(fiber.HeaderContentEncoding))
}

func TestPublicIdentityFallthroughKeepsErrorAndMethodRouting(t *testing.T) {
	body := strings.Repeat("authenticated error ", 100)
	app := fiber.New(fiber.Config{CaseSensitive: true, StrictRouting: true})
	app.Use(responseCompression(true))
	registerStaticRoutes(app, public.Files, nil, nil, "", nil, nil, func(c fiber.Ctx) error {
		if c.Get("X-Test-Admin") != testAuthorizedHeader {
			return c.Status(fiber.StatusUnauthorized).SendString(body)
		}
		return c.Next()
	})
	app.All("/public/missing", func(c fiber.Ctx) error { return c.Status(fiber.StatusForbidden).SendString(body) })
	for _, authorized := range []bool{false, true} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			headers := http.Header{fiber.HeaderAcceptEncoding: {publicGzip}}
			status := fiber.StatusUnauthorized
			if authorized {
				headers.Set("X-Test-Admin", testAuthorizedHeader)
				status = fiber.StatusForbidden
			}
			got := publicPolicyResponse(t, app, method, "/public/missing", headers)
			require.Equal(t, status, got.status)
			if method == http.MethodGet {
				require.Empty(t, got.header.Get(fiber.HeaderContentEncoding))
				require.Equal(t, body, string(got.body))
			} else {
				require.Equal(t, publicGzip, got.header.Get(fiber.HeaderContentEncoding))
			}
		}
	}
}

func TestPublicIdentityLeavesUnicodeLookalikeDynamic(t *testing.T) {
	app := fiber.New(fiber.Config{UnescapePath: true})
	app.Use(responseCompression(true))
	app.Get("/publİc/example", func(c fiber.Ctx) error { return c.SendString(strings.Repeat("dynamic ", 100)) })
	got := publicPolicyResponse(t, app, http.MethodGet, "/publ%C4%B0c/example",
		http.Header{fiber.HeaderAcceptEncoding: {publicGzip}})
	require.Equal(t, http.StatusOK, got.status)
	require.Equal(t, publicGzip, got.header.Get(fiber.HeaderContentEncoding))
}

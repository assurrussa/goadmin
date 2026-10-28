package bootstrap //nolint:testpackage // Real browser exercises library auth/CSRF, never live data.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
)

// Opt-in like the existing admission browser harness; no browser installation,
// certificate trust changes or external account access occurs in this test.
func TestSessionMaintenanceRealBrowser(t *testing.T) {
	module, executable := os.Getenv("GOADMIN_PLAYWRIGHT_MODULE"), os.Getenv("GOADMIN_BROWSER_EXECUTABLE")
	if module == "" || executable == "" {
		t.Skip("explicit Playwright module and browser executable required")
	}
	store := fibersession.NewStore(fibersession.Config{IdleTimeout: 2 * time.Second, AbsoluteTimeout: time.Minute})
	f := newRealtimeLogoutFixture(t, store)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				f.clock.Store(now.UnixNano())
			}
		}
	}()
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + listener.Addr().String()
	cfg := adminconfig.Config{Domain: "127.0.0.1", DomainURL: url, CSRFTokenName: testCSRFToken, CSRFTokenTTL: time.Hour}
	deps := Dependencies{System: SystemDependencies{AdminAuthService: f.service, CSRF: newTestCSRFService(t), SessionStore: store}}
	var checks, commands atomic.Int32
	f.app.All("/auth/session/maintain", func(c fiber.Ctx) error { checks.Add(1); return sessionMaintenanceHandler(deps, cfg)(c) })
	f.app.Get("/page-session", func(c fiber.Ctx) error {
		admin, authErr := f.service.GetAdminAuth(c)
		if authErr != nil {
			return authErr
		}
		c.Locals(coresession.AuthAdminKey.String(), admin)
		return c.JSON(sessionMaintenancePageProp(c, f.service))
	})
	f.app.Post("/command", func(c fiber.Ctx) error {
		admin, authErr := f.service.GetAdminAuth(c)
		if authErr != nil {
			return maintenanceFailure(c, authErr)
		}
		if admin == nil {
			return c.SendStatus(http.StatusUnauthorized)
		}
		c.Locals(coresession.AuthAdminKey.String(), admin)
		if csrfErr := newCheckCSRFTokenProvider(deps.System.SessionStore, deps.System.CSRF, cfg)(c); csrfErr != nil {
			return csrfErr
		}
		commands.Add(1)
		return c.SendStatus(http.StatusNoContent)
	})
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "entry.ts")
	facade, err := filepath.Abs("../resources/src/js/http/index.ts")
	require.NoError(t, err)
	quoted, err := json.Marshal(facade)
	require.NoError(t, err)
	source := `import {adminFetch,configureSessionMaintenance,ensureSessionReady,sessionCoordinator} from ` + string(quoted) + `;
window.harness={ready:false};
await fetch('/login',{method:'POST'});
configureSessionMaintenance({props:{adminSessionMaintenance:await(await fetch('/page-session')).json()}});
await ensureSessionReady();
window.harness.ready=true;
document.querySelector('form').addEventListener('submit',async event=>{event.preventDefault();
 const response=await adminFetch('/command',{method:'POST',body:document.querySelector('textarea').value});
 window.harness.status=response.status;window.harness.body=await response.text();
});
window.harness.revision=()=>sessionCoordinator.getMaintenance()?.revision;
window.harness.check=()=>ensureSessionReady({force:true});`
	require.NoError(t, os.WriteFile(entry, []byte(source), 0o600))
	bundle := filepath.Join(tmp, "client.js")
	builder := exec.CommandContext(t.Context(), "../resources/node_modules/esbuild/bin/esbuild", entry,
		"--bundle", "--format=esm", "--platform=browser", "--define:import.meta.env={}", "--outfile="+bundle)
	output, err := builder.CombinedOutput()
	require.NoError(t, err, string(output))
	client, err := os.ReadFile(bundle)
	require.NoError(t, err)
	f.app.Get("/client.js", func(c fiber.Ctx) error { c.Type("js"); return c.Send(client) })
	f.app.Get("/", func(c fiber.Ctx) error {
		c.Type("html")
		return c.SendString(`<form><textarea aria-label="Draft"></textarea><button>Save</button></form>` +
			`<script type="module" src="/client.js"></script>`)
	})
	done := make(chan error, 1)
	go func() { done <- f.app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { require.NoError(t, f.app.Shutdown()); require.NoError(t, <-done) })
	runner := `const {chromium}=require(process.env.GOADMIN_PLAYWRIGHT_MODULE);
(async()=>{const browser=await chromium.launch({executablePath:process.env.GOADMIN_BROWSER_EXECUTABLE,headless:true});
try{const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
let navigations=0;page.on('framenavigated',f=>{if(f===page.mainFrame())navigations++});
await page.goto(process.env.GOADMIN_MAINTENANCE_TEST_URL);await page.waitForFunction(()=>window.harness?.ready);
await page.getByLabel('Draft').fill('Preserved draft');
await page.waitForFunction(()=>window.harness.revision()>=6,{},{timeout:20000});
if(await page.getByLabel('Draft').inputValue()!=='Preserved draft')throw Error('draft lost');
if(navigations!==1)throw Error('unexpected document navigation');
await page.getByRole('button',{name:'Save',exact:true}).click();
await page.waitForFunction(()=>window.harness.status!==undefined);
if(await page.evaluate(()=>window.harness.status)!==204)throw Error(await page.evaluate(()=>JSON.stringify(window.harness)));
if(errors.length)throw Error(errors.join(';'));
console.log('real cookies, at least three native renewals, preserved draft and one command passed');
}finally{await browser.close()}})().catch(e=>{console.error(e);process.exit(1)});`
	script := filepath.Join(tmp, "browser.cjs")
	require.NoError(t, os.WriteFile(script, []byte(runner), 0o600))
	cmd := exec.CommandContext(t.Context(), "node", script)
	cmd.Env = append(os.Environ(), "GOADMIN_MAINTENANCE_TEST_URL="+url)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
	t.Log(strings.TrimSpace(string(output)))
	require.GreaterOrEqual(t, checks.Load(), int32(4))
	require.Equal(t, int32(1), commands.Load())
}

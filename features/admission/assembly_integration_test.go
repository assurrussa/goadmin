//go:build integration

package admission_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/assurrussa/goadmin/features/access"
	admission "github.com/assurrussa/goadmin/features/admission"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/tests"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)
const origin="https://admission.example.test"
type authority struct { identity admission.VerifiedIdentity }
func(a *authority)Verify(_ context.Context,reference string)(admission.VerifiedIdentity,error){
	if reference!="fixture-reference"{return admission.VerifiedIdentity{},goauth.ErrSessionRevoked}
	return a.identity,nil
}
func(a *authority)Revalidate(ctx context.Context,ref string,i admission.VerifiedIdentity)error{
	current,err:=a.Verify(ctx,ref);if err!=nil{return err}
	if current!=i{return goauth.ErrSessionRevoked};return nil
}
func request(t *testing.T,r *host.Runtime,method,path,body string,cookies map[string]*http.Cookie,csrf,requestOrigin string)(int,[]byte){
	t.Helper()
	req:=httptest.NewRequestWithContext(t.Context(),method,origin+path,strings.NewReader(body))
	req.Header.Set("Content-Type","application/json");req.Header.Set("X-Inertia","true")
	if requestOrigin!=""{req.Header.Set("Origin",requestOrigin)}
	if csrf!=""{req.Header.Set("X-CSRF-Token",csrf)}
	for _,c:=range cookies{req.AddCookie(c)}
	resp,err:=r.App().Test(req,fiber.TestConfig{Timeout:0,FailOnTimeout:false});require.NoError(t,err)
	raw,err:=io.ReadAll(resp.Body);require.NoError(t,err);require.NoError(t,resp.Body.Close())
	for _,c:=range resp.Cookies(){cookies[c.Name]=c}
	return resp.StatusCode,raw
}
func TestAdmissionAssemblyOffModeAndProtectedWaitingIdentity(t *testing.T){
	db,_,cleanup:=tests.PrepareDB(t.Context(),t,"AdmissionAssembly")
	t.Cleanup(func(){cleanup(context.Background())})
	cfg:=host.Config{
		Admin:host.AdminConfig{Env:"staging",Domain:"admission.example.test",DomainURL:origin,
			BaseDomain:"admission.example.test",BaseDomainURL:origin,Addr:"127.0.0.1:12345",
			SessionName:"admission_session",SessionTTL:time.Hour,SessionInactiveTTL:time.Minute,
			CSRFTokenName:"csrf_token",CSRFTokenTTL:time.Hour,CanonicalSessionBackend:"pgsql"},
		Server:host.ServerConfig{Addr:"127.0.0.1:12345",AllowOrigins:[]string{origin},DisableStartupMessage:true},
		Auth:tests.AuthRuntimeConfig(t),
		CSRF:host.CSRFConfig{AppDomain:"admission.example.test",SecretKey:strings.Repeat("s",32),AllowedOrigins:[]string{origin}},
	}
	cfg.Auth.NotificationDelivery=goauth.NotificationDeliveryDisabled
	auth,err:=host.NewAuthAdapter(host.AuthAdapterConfig{Database:db,TxManager:host.NewTxManager(db),Runtime:cfg.Auth})
	require.NoError(t,err);t.Cleanup(func(){require.NoError(t,auth.Close())})
	deps:=host.Dependencies{Database:db,Logger:host.DiscardLogger(),Auth:auth}
	off,err:=host.New(t.Context(),cfg,deps,access.New());require.NoError(t,err)
	for _,route:=range off.App().GetRoutes(true){require.NotEqual(t,"/auth/admission",route.Path);require.False(t,strings.HasPrefix(route.Path,"/admissions"))}
	require.NoError(t,off.Close())
	v:=&authority{identity:admission.VerifiedIdentity{
		Scope:admission.Scope{Issuer:"https://issuer.example.test",Project:"project",Client:"client"},
		Subject:"pending-person",Email:"pending-person@example.test",EmailVerified:true,ExpiresAt:time.Now().Add(time.Minute),
	}}
	component,err:=admission.New(t.Context(),admission.Config{Runtime:auth.Runtime(),Mode:admission.ProjectPending,
		Scope:v.identity.Scope,Verifier:v,SignInPath:"/sso/login",
		Reference:func(c fiber.Ctx)(string,error){return c.Get("X-Synthetic-Reference"),nil},
	})
	require.NoError(t,err)
	_,err=component.Complete(t.Context(),"fixture-reference","");require.Error(t,err)
	on,err:=host.New(t.Context(),cfg,deps,access.New(),component.Module());require.NoError(t,err)
	t.Cleanup(func(){require.NoError(t,on.Close())})
	result,err:=component.Complete(t.Context(),"fixture-reference","");require.NoError(t,err)
	require.Equal(t,"pending",result.Status)
	cookies:=map[string]*http.Cookie{}
	for _,path:=range []string{"/","/admins","/roles","/permissions","/files","/queues","/notifications","/ws"}{
		status,_:=request(t,on,"GET",path,"",cookies,"","")
		require.NotEqual(t,http.StatusOK,status,"pending identity must not be a native administrator")
	}
	// HTML requests preserve the error status. Inertia v2 errors intentionally
	// redirect back, so a redirect alone is not evidence of authorization.
	anonymous:=httptest.NewRequestWithContext(t.Context(),"GET",origin+"/auth/admission",nil)
	denied,err:=on.App().Test(anonymous,fiber.TestConfig{Timeout:0,FailOnTimeout:false});require.NoError(t,err)
	deniedBody,err:=io.ReadAll(denied.Body);require.NoError(t,err);require.NoError(t,denied.Body.Close())
	require.Equal(t,http.StatusUnauthorized,denied.StatusCode,"body=%s location=%s",deniedBody,denied.Header.Get("Location"))
	require.NotContains(t,string(deniedBody),result.LocalSubject)
	require.NotContains(t,string(deniedBody),"pending-person@example.test")
	req:=httptest.NewRequestWithContext(t.Context(),"GET",origin+"/auth/admission",nil)
	req.Header.Set("X-Synthetic-Reference","fixture-reference");req.Header.Set("X-Inertia","true")
	resp,err:=on.App().Test(req,fiber.TestConfig{Timeout:0,FailOnTimeout:false});require.NoError(t,err)
	raw,err:=io.ReadAll(resp.Body);require.NoError(t,err);require.NoError(t,resp.Body.Close())
	require.Equal(t,http.StatusOK,resp.StatusCode,"body=%s location=%s",raw,resp.Header.Get("Location"))
	require.Contains(t,string(raw),"auth/AdmissionWaitingPage")
	require.NotContains(t,string(raw),result.LocalSubject)
	var page map[string]any;require.NoError(t,json.Unmarshal(raw,&page))
	require.NotContains(t,string(raw),"pending-person@example.test")
	// Real native operator session: mutation must retain the existing CSRF gate.
	operator,err:=auth.Runtime().ProvisionTrustedLocalAccount(t.Context(),goauth.RegisterRequest{
		Email:"operator@example.test",Password:"Synthetic-Admission-Operator-2026!",
	});require.NoError(t,err)
	_,err=auth.Runtime().Database().ExecContext(t.Context(),"INSERT INTO administrations(subject_id,uuid) VALUES($1,$2)",operator.Subject.ID,uuid.NewString())
	require.NoError(t,err)
	roles,err:=auth.Runtime().RBAC(nil);require.NoError(t,err)
	key:=rbac.MustPermissionKey("admission","reject")
	_,err=roles.UpsertPermission(t.Context(),rbac.Permission{Key:key});require.NoError(t,err)
	role,err:=roles.CreateRole(t.Context(),rbac.Role{Slug:"admission-reviewer",Name:"Reviewer"},[]rbac.PermissionKey{key})
	require.NoError(t,err);require.NoError(t,roles.ReplaceSubjectRoles(t.Context(),operator.Subject.ID,[]int64{role.ID}))
	status,_:=request(t,on,"GET","/auth/login","",cookies,"","");require.Equal(t,http.StatusOK,status)
	require.NotNil(t,cookies["csrf_token"])
	status,_=request(t,on,"POST","/auth/login",
		`{"email":"operator@example.test","password":"Synthetic-Admission-Operator-2026!"}`,cookies,cookies["csrf_token"].Value,origin)
	require.Contains(t,[]int{302,303},status)
	body:=`{"subject":"pending-person","version":1}`
	assertPending:=func(){
		t.Helper()
		var state string
		require.NoError(t,auth.Runtime().Database().QueryRowContext(t.Context(),"SELECT status FROM admin_admissions WHERE subject_id=$1",result.LocalSubject).Scan(&state))
		require.Equal(t,"pending",state)
	}
	// Inertia v2 renders command errors as redirects with flash errors.
	// Assert the persistent effect after each denied request, not a guessed code.
	status,_=request(t,on,"POST","/admissions/reject",body,cookies,"",origin)
	require.Equal(t,http.StatusSeeOther,status)
	assertPending()
	status,_=request(t,on,"POST","/admissions/reject",body,cookies,cookies["csrf_token"].Value,"https://foreign.example.test")
	require.Equal(t,http.StatusSeeOther,status)
	assertPending()
	// Positive control: this same actor and route can mutate with valid CSRF.
	status,_=request(t,on,"POST","/admissions/reject",body,cookies,cookies["csrf_token"].Value,origin)
	require.Equal(t,http.StatusSeeOther,status)
	var state string
	require.NoError(t,auth.Runtime().Database().QueryRowContext(t.Context(),"SELECT status FROM admin_admissions WHERE subject_id=$1",result.LocalSubject).Scan(&state))
	require.Equal(t,"rejected",state)
}

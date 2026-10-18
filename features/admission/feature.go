// Package admission provides explicit, opt-in SSO administrator enrollment.
// Construct a Component, include Module() in host.New, and invoke Complete only
// from the host's verified SSO callback. Construction alone enables no routes.
package admission

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goadmin/host"
	policy "github.com/assurrussa/goadmin/internal/admission"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Mode = policy.Mode
type Scope = policy.Scope
type VerifiedIdentity = policy.Identity
type Verifier = policy.Verifier
const (
	InvitationOnly = policy.InvitationOnly
	ProjectPending = policy.ProjectPending
)
type Config struct {
	Runtime *postgres.Runtime
	Mode Mode
	Scope Scope
	Verifier Verifier
	// Reference reads only the opaque RP reference belonging to THIS browser.
	// Never return a user-supplied issuer/subject or shared operator reference.
	Reference func(fiber.Ctx)(string,error)
	// SignInPath starts a fresh, state/PKCE-bound host login (root-relative).
	SignInPath string
	Now func()time.Time
}
type Result struct {
	Status string
	// LocalSubject is server-only. For active results the host must construct a
	// fresh canonical ExternalBinding and call the existing LoginExternalAdmin.
	LocalSubject string
}
type Component struct {
	cfg Config
	service *policy.Service
	store *policy.Postgres
	definitions []host.PermissionDefinition
	built atomic.Bool
	registered atomic.Bool
}
func New(ctx context.Context,cfg Config)(*Component,error){
	if ctx==nil || cfg.Reference==nil || cfg.Verifier==nil || cfg.Runtime==nil || cfg.Runtime.Runtime==nil || cfg.Runtime.Database()==nil{return nil,policy.ErrInvalid}
	if cfg.Now==nil{cfg.Now=time.Now}
	u,err:=url.Parse(cfg.SignInPath)
	if err!=nil || !strings.HasPrefix(cfg.SignInPath,"/") || strings.HasPrefix(cfg.SignInPath,"//") ||
		strings.ContainsAny(cfg.SignInPath,"\\\r\n") || u.IsAbs() || u.Host!="" || u.Fragment!="" {return nil,policy.ErrInvalid}
	issuer,err:=url.Parse(cfg.Scope.Issuer)
	if err!=nil || issuer.Scheme!="https" || issuer.Host=="" || issuer.User!=nil ||
		issuer.RawQuery!="" || issuer.Fragment!="" || issuer.Path!="" {return nil,policy.ErrInvalid}
	// Readiness is read-only; schema changes remain in the normal migration runner.
	checkCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
	defer cancel()
	rows,err:=cfg.Runtime.Database().QueryContext(checkCtx,
		"SELECT a.id,i.id,e.id FROM admin_admissions a,admin_admission_invitations i,admin_admission_events e LIMIT 0")
	if err!=nil{return nil,errors.New("admission schema is not ready")}
	if err:=rows.Close();err!=nil{return nil,err}
	store,err:=policy.NewPostgres(cfg.Runtime,cfg.Now);if err!=nil{return nil,err}
	service,err:=policy.New(policy.Config{Mode:cfg.Mode,Scope:cfg.Scope,Now:cfg.Now,NewID:uuid.NewString},store,cfg.Verifier)
	if err!=nil{return nil,err}
	permissions,err:=store.Permissions(ctx);if err!=nil{return nil,err}
	defs:=make([]host.PermissionDefinition,0,len(permissions))
	for _,p:=range permissions{
		d,a,_:=strings.Cut(string(p.Key),":")
		defs=append(defs,host.NewPermissionDefinition(host.NewPermissionKey(host.PermissionDomain(d),host.PermissionAction(a)),p.Description))
	}
	return &Component{cfg:cfg,service:service,store:store,definitions:defs},nil
}
func (c *Component) Module()host.Module{
	return host.DefinedFeatureModule(host.ModuleDescriptor{Key:"admission",Requires:[]string{"access"},
		RouteNamespaces:[]string{"/admissions"},Routes:[]string{"GET /auth/admission"}},c)
}
func (c *Component) Descriptor()host.FeatureDescriptor{
	return host.FeatureDescriptor{Key:"admission",PermissionDefinitions:append([]host.PermissionDefinition(nil),c.definitions...),
		Menu:host.Menu{Sections:[]host.Section{{Key:"admission",Title:"SSO access",Order:80,
			Items:[]host.Item{{Name:"SSO access",Href:"/admissions",Icon:"users",
				PermissionKey:host.NewPermissionKey("admission","read")}}}}}}
}
func (c *Component) Build(host.Menu)host.ExtensionBuilder{
	return func(app *host.App)(*host.Extension,error){
		if app==nil || app.Unwrap()==nil || !app.Unwrap().AdminAuth().UsesCanonicalRuntime(c.cfg.Runtime){
			return nil,errors.New("admission must share the native canonical Runtime")
		}
		if !c.built.CompareAndSwap(false,true){return nil,errors.New("admission component already mounted")}
		h:=&handler{component:c,app:app}
		return host.NewExtension().WithHandlers(h).WithPublicRegister(func(router *fiber.App){
			router.Get("/auth/admission",h.Waiting)
		}),nil
	}
}
func (c *Component) Complete(ctx context.Context,reference,invitationID string)(Result,error){
	if !c.registered.Load(){return Result{},policy.ErrDenied}
	r,err:=c.service.Admit(ctx,reference,invitationID)
	if err!=nil{return Result{},err}
	return Result{Status:string(r.Status),LocalSubject:r.LocalSubject},nil
}
type handler struct { component *Component; app *host.App }
type inviteInput struct {
	Subject string `json:"subject"`
	RoleIDs []int64 `json:"roleIds"`
	ExpiresAt string `json:"expiresAt"`
}
type decisionInput struct {
	Subject string `json:"subject"`
	Version int64 `json:"version"`
	RoleIDs []int64 `json:"roleIds"`
	ExpiresAt string `json:"expiresAt"`
}
type revokeInput struct { ID string `json:"id"` }
func (h *handler) RegisterGroupRoutes(router fiber.Router,_ ...fiber.Handler){
	g:=router.Group("/admissions")
	g.Use(func(c fiber.Ctx)error{if len(c.Body())>16384{return fiber.ErrRequestEntityTooLarge};return c.Next()})
	g.Get("",h.app.Guard("admission","read"),h.Index)
	g.Post("/invitations",host.Command(h.app,host.NewPermissionKey("admission","invite"),h.Invite)...)
	g.Post("/approve",host.Command(h.app,host.NewPermissionKey("admission","approve"),h.Approve)...)
	g.Post("/reject",host.Command(h.app,host.NewPermissionKey("admission","reject"),h.Reject)...)
	g.Post("/revoke",host.Command(h.app,host.NewPermissionKey("admission","invite"),h.Revoke)...)
	h.component.registered.Store(true)
}
func httpError(err error)error{
	switch{
	case err==nil:return nil
	case errors.Is(err,policy.ErrDenied),errors.Is(err,goauth.ErrAccountUnavailable),errors.Is(err,goauth.ErrMembershipDenied):return fiber.ErrForbidden
	case errors.Is(err,goauth.ErrExplicitIdentityLink),errors.Is(err,goauth.ErrIdentifierAlreadyExists),errors.Is(err,goauth.ErrIdentityLinkConflict):return fiber.ErrConflict
	case errors.Is(err,policy.ErrInvalid),errors.Is(err,goauth.ErrInvalidIdentifier):return fiber.ErrBadRequest
	case errors.Is(err,policy.ErrNotFound):return fiber.ErrNotFound
	case errors.Is(err,policy.ErrConflict):return fiber.ErrConflict
	default:return fiber.ErrServiceUnavailable
	}
}
func (h *handler) Index(c fiber.Ctx)error{
	actor,ok:=h.app.CurrentActor(c);if !ok{return fiber.ErrUnauthorized}
	rows,err:=h.component.store.List(c,actor.SubjectID,h.component.cfg.Scope);if err!=nil{return httpError(err)}
	invites,err:=h.component.store.Invitations(c,actor.SubjectID,h.component.cfg.Scope);if err!=nil{return httpError(err)}
	inviteRoles,err:=h.component.store.GrantableRoles(c,actor.SubjectID,"invite");if err!=nil{return httpError(err)}
	approveRoles,err:=h.component.store.GrantableRoles(c,actor.SubjectID,"approve");if err!=nil{return httpError(err)}
	canInvite,err:=h.component.store.Allowed(c,actor.SubjectID,"invite");if err!=nil{return httpError(err)}
	canReject,err:=h.component.store.Allowed(c,actor.SubjectID,"reject");if err!=nil{return httpError(err)}
	return h.app.Render(c,"admission/IndexPage",map[string]any{
		"title":"SSO access","records":rows,"invitations":invites,"inviteRoles":inviteRoles,"approveRoles":approveRoles,
		"scope":h.component.cfg.Scope,"maxDisplayed":100,"canInvite":canInvite,"canReject":canReject,
	})
}
func (h *handler) Invite(c fiber.Ctx,a host.Actor,in inviteInput)error{
	until,err:=time.Parse(time.RFC3339,in.ExpiresAt);if err!=nil{return fiber.ErrBadRequest}
	_,err=h.component.service.Invite(c,a.SubjectID,in.Subject,in.RoleIDs,until)
	if err!=nil{return httpError(err)}
	return c.Redirect().Status(303).To("/admissions")
}
func (h *handler) Approve(c fiber.Ctx,a host.Actor,in decisionInput)error{
	until,err:=time.Parse(time.RFC3339,in.ExpiresAt);if err!=nil{return fiber.ErrBadRequest}
	err=h.component.service.Approve(c,a.SubjectID,in.Subject,in.Version,in.RoleIDs,until)
	if err!=nil{return httpError(err)}
	return c.Redirect().Status(303).To("/admissions")
}
func (h *handler) Reject(c fiber.Ctx,a host.Actor,in decisionInput)error{
	if err:=h.component.service.Reject(c,a.SubjectID,in.Subject,in.Version);err!=nil{return httpError(err)}
	return c.Redirect().Status(303).To("/admissions")
}
func (h *handler) Revoke(c fiber.Ctx,a host.Actor,in revokeInput)error{
	if err:=h.component.service.Revoke(c,a.SubjectID,in.ID);err!=nil{return httpError(err)}
	return c.Redirect().Status(303).To("/admissions")
}
func (h *handler) Waiting(c fiber.Ctx)error{
	c.Set("Cache-Control","no-store")
	reference,err:=h.component.cfg.Reference(c);if err!=nil{return fiber.ErrUnauthorized}
	r,err:=h.component.service.View(c,reference);if err!=nil{return httpError(err)}
	// No identities, role assignments or credentials are emitted to this page.
	return h.app.Render(c,"auth/AdmissionWaitingPage",map[string]any{
		"title":"SSO access","admissionStatus":r.Status,"signInPath":h.component.cfg.SignInPath,
	})
}

// GrantRolePermission returns the explicit permission needed to assign this
// immutable role ID. Include it in the normal permission catalogue for roles
// added after this component was assembled; it grants nothing by itself.
func GrantRolePermission(publicID string)(host.PermissionKey,error){
	key,err:=policy.GrantPermission(publicID);if err!=nil{return host.PermissionKey{},err}
	d,a,_:=strings.Cut(string(key),":")
	return host.NewPermissionKey(host.PermissionDomain(d),host.PermissionAction(a)),nil
}

// Package core defines the hook interfaces through which the extension zone
// (ext/) attaches to the official new-api code. Rules: ext/AGENTS.md §3.
//
// Every hook has a package-level variable holding the active implementation.
// The defaults are no-ops (observing hooks) or return handled=false (takeover
// hooks), so the official code behaves exactly as upstream when ext is absent.
// ext.Register replaces these variables at startup; nothing else may.
//
// This package must stay free of business logic and must not import any
// official business package, so that main.go and router/main.go can import it
// without creating cycles.
package core

import "github.com/gin-gonic/gin"

// Reserved hook identifiers. Keep in sync with ext/docs/HOOKS.md.
const (
	HookModelGate        = "H1" // admission/veto
	HookTenantResolver   = "H2" // observing
	HookFundingHooks     = "H3" // takeover
	HookPricingResolver  = "H4" // observing
	HookRouteRegistrar   = "H5" // observing
	HookMigrator         = "H6" // observing
	HookTaskFundingHooks = "H7" // takeover
)

// ---------------------------------------------------------------------------
// H2 TenantResolver
// ---------------------------------------------------------------------------

// TenantResolver runs as the first business-aware middleware on every request.
// It identifies the tenant (main site or a brand) from the request and may
// write the result into the gin context. It may also abort the request for
// hosts that are not open (e.g. an unverified brand domain). It must not query
// the database on the hot path for the main site; see ext/AGENTS.md §3.3.
type TenantResolver interface {
	Resolve(c *gin.Context)
}

type noopTenantResolver struct{}

func (noopTenantResolver) Resolve(c *gin.Context) { c.Next() }

// Tenant is the active H2 implementation. Default: pass-through.
var Tenant TenantResolver = noopTenantResolver{}

// TenantMiddleware is the single call site used by main.go.
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) { Tenant.Resolve(c) }
}

// ---------------------------------------------------------------------------
// H5 RouteRegistrar
// ---------------------------------------------------------------------------

// RouteRegistrar mounts every extension route group onto the official engine.
// It is called once from router.SetRouter before the official web fallback.
type RouteRegistrar interface {
	Register(engine *gin.Engine)
}

type noopRouteRegistrar struct{}

func (noopRouteRegistrar) Register(*gin.Engine) {}

// Routes is the active H5 implementation. Default: registers nothing.
var Routes RouteRegistrar = noopRouteRegistrar{}

// RegisterRoutes is the single call site used by router/main.go.
func RegisterRoutes(engine *gin.Engine) { Routes.Register(engine) }

// Package core defines the hook interfaces through which the extension zone
// (ext/) attaches to the official new-api code. Rules: ext/AGENTS.md §3.
//
// Every hook has a package-level variable holding the active implementation.
// The defaults are no-ops (observing hooks) or return handled=false (takeover
// hooks), so the official code behaves exactly as upstream when ext is absent.
// ext.Register replaces these variables at startup; nothing else may.
//
// Interfaces are intentionally empty in P0. Each hook gets its real signature
// in the commit that registers it in ext/docs/HOOKS.md (hook(Hn): <name>).
package core

// Reserved hook identifiers. Keep in sync with ext/docs/HOOKS.md.
const (
	HookModelGate        = "H1" // observing
	HookTenantResolver   = "H2" // observing
	HookFundingHooks     = "H3" // takeover
	HookPricingResolver  = "H4" // observing
	HookRouteRegistrar   = "H5" // observing
	HookMigrator         = "H6" // observing
	HookTaskFundingHooks = "H7" // takeover
)

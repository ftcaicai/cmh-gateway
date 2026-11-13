package core

import "context"

// PluginHookIdentity identifies only the exact compiled source and hook path.
// It carries no arguments, bodies, configuration, channel keys or credentials.
// Source approval is not approval of effective config, model or destination.
type PluginHookIdentity struct {
	SourceSHA256 string
	Export       string
	Members      []string
}

// PluginHookGate may veto runtime hook execution before a JS slot or runtime
// is acquired. Compilation and declarative metadata inspection are not gated.
type PluginHookGate interface {
	CheckHook(context.Context, PluginHookIdentity) error
}

type noopPluginHookGate struct{}

func (noopPluginHookGate) CheckHook(context.Context, PluginHookIdentity) error { return nil }

// PluginHooks may only be replaced during startup registration. The default
// preserves upstream behavior; no source policy is installed by this package.
var PluginHooks PluginHookGate = noopPluginHookGate{}

func PluginHookAdmissionEnabled() bool {
	_, noop := PluginHooks.(noopPluginHookGate)
	return !noop
}

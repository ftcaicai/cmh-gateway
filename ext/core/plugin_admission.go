package core

import (
	"context"
	"errors"
)

// ErrPluginAdmissionDenied identifies a host source-policy veto, not a plugin
// exception. Its fixed text never exposes the policy's configuration or reason.
var ErrPluginAdmissionDenied = errors.New("plugin execution rejected")

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

// ExecutorGate checks an exact source/config tuple without inventing a model or
// channel before selection. A passing tuple is not model/destination authority.
type ExecutorGate interface {
	CheckExecutor(context.Context, ExecutorIdentity) error
}

type noopExecutorGate struct{}

func (noopExecutorGate) CheckExecutor(context.Context, ExecutorIdentity) error { return nil }

// Executors is replaced only during startup; the default preserves host behavior.
var Executors ExecutorGate = noopExecutorGate{}

func ExecutorAdmissionEnabled() bool {
	_, noop := Executors.(noopExecutorGate)
	return !noop
}

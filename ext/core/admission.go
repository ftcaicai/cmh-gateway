// Package core owns value-only extension boundaries. It must not depend on
// model, service, relay or extension business implementations.
package core

import "context"

// AdmissionPhase describes how much authoritative identity the host has
// resolved. Public checks never stand in for later candidate/execution checks.
type AdmissionPhase string

const (
	AdmissionPublic      AdmissionPhase = "public"
	AdmissionChannelSave AdmissionPhase = "channel_save"
	AdmissionCandidate   AdmissionPhase = "candidate"
	AdmissionAttempt     AdmissionPhase = "attempt"
	AdmissionDispatch    AdmissionPhase = "dispatch"
)

// ModelIdentity keeps public authorization separate from a resolved upstream
// target. Public is the host's canonical public identifier, not raw client bytes.
// Empty Declared means the executor is not a task-model plugin.
type ModelIdentity struct {
	Public   string
	Declared string
	Upstream string
}

// ExecutorIdentity names the actual execution artifact, never a metadata owner
// obtained by looking up the first plugin declaring a shared model name.
type ExecutorIdentity struct {
	Kind              string
	Key               string
	SourceSHA256      string
	ConfigFingerprint string
	Generation        int64
}

// ChannelIdentity is a non-secret snapshot. It must not contain keys, headers,
// URLs with queries, credentials or opaque provider configuration.
type ChannelIdentity struct {
	Known bool
	ID    int
	Type  int
}

type Admission struct {
	Phase    AdmissionPhase
	Model    ModelIdentity
	Channel  ChannelIdentity
	Executor ExecutorIdentity
}

// ChannelSpec contains the complete effective post-patch state. Host code owns
// mapping resolution, plugin reconciliation and transaction semantics. An empty
// model list or unresolved target must not be reconstructed by the extension.
// Disabling a channel remains possible even when its former identity is denied.
type ChannelSpec struct {
	Channel  ChannelIdentity
	Enabled  bool
	Models   []ModelIdentity
	Executor ExecutorIdentity
}

// ModelGate is an admission/veto boundary, not an observing hook. A host must
// propagate errors before side effects and filter denied candidates before
// applying its original priority, affinity and weighting rules. Visible only
// controls publication; it is not permission to execute a request.
type ModelGate interface {
	Check(context.Context, Admission) error
	ValidateChannels(context.Context, []ChannelSpec) error
	Visible(context.Context, Admission) bool
}

type noopModelGate struct{}

func (noopModelGate) Check(context.Context, Admission) error                { return nil }
func (noopModelGate) ValidateChannels(context.Context, []ChannelSpec) error { return nil }
func (noopModelGate) Visible(context.Context, Admission) bool               { return true }

// Models defaults to upstream behavior. Only startup registration may replace
// it; per-request replacement or environment lookup is not supported.
var Models ModelGate = noopModelGate{}

// ModelAdmissionEnabled distinguishes the upstream-compatible default from an
// installed policy. Host code may avoid building projections or querying data
// in the default case; it must not use this to bypass an installed policy.
func ModelAdmissionEnabled() bool {
	_, noop := Models.(noopModelGate)
	return !noop
}

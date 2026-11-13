package domestic

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/ext/core"
)

// ErrAdmissionDenied deliberately contains no model, channel or executor identity.
var ErrAdmissionDenied = errors.New("model admission denied")

var errGateConfiguration = errors.New("invalid model admission configuration")

// Gate is an immutable admission policy. Construct it at startup; it neither
// discovers plugins nor reads environment/configuration during requests.
type Gate struct {
	public    *Policy
	upstream  *Policy
	approvals map[executorApproval]struct{}
}

type executorApproval struct {
	kind, key, sourceSHA256, configFingerprint string
}

var _ core.ModelGate = (*Gate)(nil)

// NewGate keeps public and upstream authorization independent. Public owns the
// channel-type allowlist. Nonempty Declared identities require exact membership
// in the upstream allowlist too, not automatic public-name authorization.
// Approvals pin actual plugin artifacts and effective configuration, not plugin
// names alone. Generation is a host snapshot marker, not an approval identity.
func NewGate(public, upstream *Policy, approvals []core.ExecutorIdentity) (*Gate, error) {
	if public == nil || upstream == nil {
		return nil, errGateConfiguration
	}
	g := &Gate{public: public, upstream: upstream, approvals: make(map[executorApproval]struct{}, len(approvals))}
	for _, executor := range approvals {
		approval, ok := approvedExecutorIdentity(executor)
		if !ok {
			return nil, errGateConfiguration
		}
		g.approvals[approval] = struct{}{}
	}
	return g, nil
}

// approvedExecutorIdentity accepts only known executor kinds and bounded,
// non-secret identities. Digests are canonical lowercase SHA-256 hex strings.
func approvedExecutorIdentity(executor core.ExecutorIdentity) (executorApproval, bool) {
	approval := executorApproval{executor.Kind, executor.Key, executor.SourceSHA256, executor.ConfigFingerprint}
	switch executor.Kind {
	case "builtin":
		return approval, executor.Key == "" && executor.SourceSHA256 == "" && executor.ConfigFingerprint == ""
	case "plugin":
		if len(executor.Key) == 0 || len(executor.Key) > 255 {
			return executorApproval{}, false
		}
		for _, c := range executor.Key {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
				return executorApproval{}, false
			}
		}
		for _, digest := range []string{executor.SourceSHA256, executor.ConfigFingerprint} {
			if len(digest) != 64 {
				return executorApproval{}, false
			}
			for _, c := range digest {
				if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
					return executorApproval{}, false
				}
			}
		}
		return approval, true
	default:
		return executorApproval{}, false
	}
}

func (g *Gate) Check(ctx context.Context, admission core.Admission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if g == nil || !g.public.AllowsModel(admission.Model.Public) {
		return ErrAdmissionDenied
	}
	switch admission.Phase {
	case core.AdmissionPublic:
		return nil
	case core.AdmissionCandidate, core.AdmissionAttempt, core.AdmissionDispatch, core.AdmissionChannelSave:
	default:
		return ErrAdmissionDenied
	}
	minimumID := 1
	if admission.Phase == core.AdmissionChannelSave {
		minimumID = 0 // A new channel has no persisted ID yet; never fabricate one.
	}
	if !admission.Channel.Known || admission.Channel.ID < minimumID || admission.Channel.Type <= 0 || !g.public.AllowsChannelType(admission.Channel.Type) {
		return ErrAdmissionDenied
	}
	if !g.upstream.AllowsModel(admission.Model.Upstream) || admission.Model.Declared != "" && !g.upstream.AllowsModel(admission.Model.Declared) {
		return ErrAdmissionDenied
	}
	approval, ok := approvedExecutorIdentity(admission.Executor)
	if !ok {
		return ErrAdmissionDenied
	}
	if approval.kind == "builtin" {
		return nil
	}
	if _, ok := g.approvals[approval]; !ok {
		return ErrAdmissionDenied
	}
	return nil
}

// ValidateChannels is pure prevalidation of the entire effective batch. The
// caller must wait for success before any write; disabled entries allow cleanup.
func (g *Gate) ValidateChannels(ctx context.Context, channels []core.ChannelSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, channel := range channels {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !channel.Enabled {
			continue
		}
		if len(channel.Models) == 0 {
			return ErrAdmissionDenied
		}
		for _, model := range channel.Models {
			if err := g.Check(ctx, core.Admission{Phase: core.AdmissionChannelSave, Model: model, Channel: channel.Channel, Executor: channel.Executor}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Visible requires resolved candidate authority. A public-only check cannot
// publish a model whose candidate or actual executor would subsequently fail.
func (g *Gate) Visible(ctx context.Context, admission core.Admission) bool {
	switch admission.Phase {
	case core.AdmissionCandidate, core.AdmissionAttempt, core.AdmissionDispatch:
		return g.Check(ctx, admission) == nil
	default:
		return false
	}
}

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrContinuationUnavailable is distinct from transport/hook failure. Future
// task hosts must hold for repair rather than turn this into a funded task's
// ordinary poll-failure counter, terminal transition, settlement or refund.
var ErrContinuationUnavailable = errors.New("task continuation unavailable")

type ContinuationPurpose string

const (
	ContinuationPoll    ContinuationPurpose = "poll"
	ContinuationContent ContinuationPurpose = "content"
	ContinuationReport  ContinuationPurpose = "report"
)

// ContinuationBinding prevents a reference from granting another task's
// capabilities. These IDs come from authoritative host storage, not client input.
type ContinuationBinding struct {
	TaskID    string
	UserID    int
	ChannelID int
}

// ContinuationRef alone is not a bearer credential or permission to submit new
// work. Only non-secret references may be copied into official private audit DTOs.
type ContinuationRef struct {
	ID     string
	Digest string
}

// ContinuationArtifact is a SENSITIVE host-only envelope. RuntimeJSON is exact
// host-produced versioned state, not a schema this interface interprets. It may
// contain private endpoints/configuration. Never expose it in public DTOs/logs.
// SourceHash verification does not itself grant operator source authorization.
type ContinuationArtifact struct {
	PluginKey    string
	SourceSHA256 string
	Source       []byte
	RuntimeJSON  []byte
}

// ContinuationStore owns immutable encrypted persistence, not executor selection,
// credential-account substitution, money mutation or new-request admission.
type ContinuationStore interface {
	Save(context.Context, ContinuationBinding, ContinuationArtifact) (ContinuationRef, error)
	Load(context.Context, ContinuationBinding, ContinuationRef, ContinuationPurpose) (ContinuationArtifact, error)
}

// ContextKeyTaskContinuation holds only the successfully persisted reference.
const ContextKeyTaskContinuation = "cmh_task_continuation"

// TaskContinuations is optional and startup-only. Nil preserves host behavior.
// Hosts must reject an installed archive without authoritative model admission;
// merely installing a byte store cannot certify the selected execution state.
var TaskContinuations ContinuationStore

// ContinuationGrantRequest binds authority to one authenticated archive artifact.
// Hashes and references are identifiers, never credentials or a permission by themselves.
type ContinuationGrantRequest struct {
	Binding                                ContinuationBinding
	Ref                                    ContinuationRef
	Purpose                                ContinuationPurpose
	PluginKey, SourceSHA256, RuntimeSHA256 string
}

type ContinuationGrant struct {
	Request   ContinuationGrantRequest
	Hooks     []string
	ExpiresAt int64
}

type ContinuationAuthorizer interface {
	Authorize(context.Context, ContinuationGrantRequest) (ContinuationGrant, bool, error)
}

// ContinuationGrants is optional, startup-only operator authority. The installed
// store and authorizer, like all trusted Go host code, belong to the TCB.
var ContinuationGrants ContinuationAuthorizer

func continuationDigestValid(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, c := range digest {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func continuationRequestValid(request ContinuationGrantRequest) bool {
	binding := request.Binding
	if binding.TaskID == "" || len(binding.TaskID) > 255 || !utf8.ValidString(binding.TaskID) || binding.UserID <= 0 || binding.ChannelID <= 0 || request.Purpose != ContinuationPoll {
		return false
	}
	for _, c := range binding.TaskID {
		if unicode.IsControl(c) {
			return false
		}
	}
	if len(request.PluginKey) == 0 || len(request.PluginKey) > 255 {
		return false
	}
	for _, c := range request.PluginKey {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return continuationDigestValid(request.Ref.ID) && continuationDigestValid(request.Ref.Digest) && continuationDigestValid(request.SourceSHA256) && continuationDigestValid(request.RuntimeSHA256)
}

// ValidateContinuationGrant validates an operator value; it never mints a scope.
func ValidateContinuationGrant(grant ContinuationGrant) error {
	if !continuationRequestValid(grant.Request) || grant.ExpiresAt <= time.Now().Unix() || len(grant.Hooks) < 2 || len(grant.Hooks) > 4 {
		return ErrContinuationUnavailable
	}
	seen := make(map[string]bool, len(grant.Hooks))
	for _, hook := range grant.Hooks {
		switch hook {
		case "buildQueryRequest", "parseTaskResult", "signRequest", "extractUsageOnComplete":
		default:
			return ErrContinuationUnavailable
		}
		if seen[hook] {
			return ErrContinuationUnavailable
		}
		seen[hook] = true
	}
	if !seen["buildQueryRequest"] || !seen["parseTaskResult"] {
		return ErrContinuationUnavailable
	}
	return nil
}

// ContinuationScope is minted only by LoadTaskContinuation after the installed
// store authenticates an artifact and the operator authorizes its exact identity.
// Its zero value is invalid. It grants no fetch, credential or target authority.
type ContinuationScope struct {
	request   ContinuationGrantRequest
	hooks     []string
	expiresAt int64
	valid     bool
}

func (scope *ContinuationScope) Check() error {
	if scope == nil || !scope.valid || time.Now().Unix() >= scope.expiresAt || Destinations == nil || !DestinationAdmissionEnabled() {
		return ErrContinuationUnavailable
	}
	return nil
}

func (scope *ContinuationScope) BoundTo(request ContinuationGrantRequest) bool {
	return scope.Check() == nil && scope.request == request
}

func (scope *ContinuationScope) CheckHook(identity PluginHookIdentity) error {
	if scope.Check() != nil || identity.SourceSHA256 != scope.request.SourceSHA256 || len(identity.Members) != 0 || !slices.Contains(scope.hooks, identity.Export) {
		return ErrContinuationUnavailable
	}
	return nil
}

type continuationScopeContextKey struct{}
type continuationScopeContextValue struct{ scope *ContinuationScope }

// WithContinuationScope preserves presence even for a nil/invalid scope, so an
// attached but unusable scope cannot silently fall back to current source policy.
func WithContinuationScope(ctx context.Context, scope *ContinuationScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, continuationScopeContextKey{}, continuationScopeContextValue{scope: scope})
}

func ContinuationScopeFromContext(ctx context.Context) (*ContinuationScope, bool) {
	if ctx == nil {
		return nil, false
	}
	value, present := ctx.Value(continuationScopeContextKey{}).(continuationScopeContextValue)
	return value.scope, present
}

// LoadTaskContinuation is the sole scope mint. Archive references or a public
// Restore/Bind call alone cannot create authority. No-match retains current policy.
func LoadTaskContinuation(ctx context.Context, binding ContinuationBinding, ref ContinuationRef, purpose ContinuationPurpose) (ContinuationArtifact, *ContinuationScope, error) {
	unavailable := func() (ContinuationArtifact, *ContinuationScope, error) {
		return ContinuationArtifact{}, nil, ErrContinuationUnavailable
	}
	if ctx == nil {
		return unavailable()
	}
	if err := ctx.Err(); err != nil {
		return ContinuationArtifact{}, nil, err
	}
	store := TaskContinuations
	if store == nil {
		return unavailable()
	}
	artifact, err := store.Load(ctx, binding, ref, purpose)
	if err != nil {
		if ctx.Err() != nil {
			return ContinuationArtifact{}, nil, ctx.Err()
		}
		return unavailable()
	}
	if err := ctx.Err(); err != nil {
		return ContinuationArtifact{}, nil, err
	}
	if len(artifact.Source) == 0 || len(artifact.Source) > 2<<20 || len(artifact.RuntimeJSON) == 0 || len(artifact.RuntimeJSON) > 256<<10 || !utf8.Valid(artifact.Source) || !utf8.Valid(artifact.RuntimeJSON) {
		return unavailable()
	}
	artifact.Source, artifact.RuntimeJSON = slices.Clone(artifact.Source), slices.Clone(artifact.RuntimeJSON)
	sourceHash, runtimeHash := sha256.Sum256(artifact.Source), sha256.Sum256(artifact.RuntimeJSON)
	if hex.EncodeToString(sourceHash[:]) != artifact.SourceSHA256 {
		return unavailable()
	}
	request := ContinuationGrantRequest{Binding: binding, Ref: ref, Purpose: ContinuationPoll, PluginKey: artifact.PluginKey, SourceSHA256: artifact.SourceSHA256, RuntimeSHA256: hex.EncodeToString(runtimeHash[:])}
	if !continuationRequestValid(request) {
		return unavailable()
	}
	request.Purpose = purpose
	switch purpose {
	case ContinuationPoll, ContinuationContent, ContinuationReport:
	default:
		return unavailable()
	}
	authorizer := ContinuationGrants
	if authorizer == nil {
		return artifact, nil, nil
	}
	// Content/report have no grant in this first slice, but remain governed by
	// their existing source/target checks when no scope is attached.
	if purpose != ContinuationPoll {
		return artifact, nil, nil
	}
	if !continuationRequestValid(request) {
		return unavailable()
	}
	grant, found, err := authorizer.Authorize(ctx, request)
	if ctx.Err() != nil {
		return ContinuationArtifact{}, nil, ctx.Err()
	}
	if err != nil {
		return unavailable()
	}
	if !found {
		return artifact, nil, nil
	}
	if grant.Request != request || ValidateContinuationGrant(grant) != nil || Destinations == nil || !DestinationAdmissionEnabled() {
		return unavailable()
	}
	scope := &ContinuationScope{request: request, hooks: slices.Clone(grant.Hooks), expiresAt: grant.ExpiresAt, valid: true}
	return artifact, scope, nil
}

package core

import (
	"context"
	"errors"
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

// No global installer is provided yet: an unconnected archive must not imply
// persisted task polling/content/reporting have adopted frozen authority.

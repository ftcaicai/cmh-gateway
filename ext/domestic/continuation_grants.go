package domestic

import (
	"context"
	"slices"

	"github.com/QuantumNous/new-api/ext/core"
)

// ContinuationPolicy is an immutable, exact operator grant snapshot. It does not
// discover tasks, install global hooks or treat archive references as permission.
// Removing a grant requires deploying a replacement snapshot on every worker.
type ContinuationPolicy struct {
	grants map[core.ContinuationGrantRequest]core.ContinuationGrant
}

var _ core.ContinuationAuthorizer = (*ContinuationPolicy)(nil)

func NewContinuationAuthorizer(grants []core.ContinuationGrant) (*ContinuationPolicy, error) {
	if len(grants) > 4096 {
		return nil, errGateConfiguration
	}
	policy := &ContinuationPolicy{grants: make(map[core.ContinuationGrantRequest]core.ContinuationGrant, len(grants))}
	for _, grant := range grants {
		if core.ValidateContinuationGrant(grant) != nil {
			return nil, errGateConfiguration
		}
		if _, duplicate := policy.grants[grant.Request]; duplicate {
			return nil, errGateConfiguration
		}
		grant.Hooks = slices.Clone(grant.Hooks)
		policy.grants[grant.Request] = grant
	}
	return policy, nil
}

func (policy *ContinuationPolicy) Authorize(ctx context.Context, request core.ContinuationGrantRequest) (core.ContinuationGrant, bool, error) {
	if ctx == nil {
		return core.ContinuationGrant{}, false, core.ErrContinuationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return core.ContinuationGrant{}, false, err
	}
	if policy == nil {
		return core.ContinuationGrant{}, false, nil
	}
	grant, found := policy.grants[request]
	if !found {
		return core.ContinuationGrant{}, false, nil
	}
	// An expired match must stay a match with an error, never turn into a
	// no-match fallback to a separately permissive current-source policy.
	if core.ValidateContinuationGrant(grant) != nil {
		return core.ContinuationGrant{}, true, core.ErrContinuationUnavailable
	}
	grant.Hooks = slices.Clone(grant.Hooks)
	return grant, true, nil
}

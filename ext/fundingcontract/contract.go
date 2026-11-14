// Package fundingcontract is an isolated synthetic value prototype. It has no
// production integration, persistence, transaction, cache, pricing or token API.
// Its conserved quantity is one synthetic asset partitioned into states; Consumed
// is not an additional platform receipt to add to a real customer's assets.
package fundingcontract

import (
	"errors"
	"maps"
	"strings"
)

// MaxUnits is a synthetic integer bound, not a price or quota conversion rule.
const MaxUnits int64 = 1<<53 - 1

var (
	ErrInvalid   = errors.New("invalid synthetic funding event")
	ErrConflict  = errors.New("conflicting synthetic event replay")
	ErrBinding   = errors.New("synthetic funding binding mismatch")
	ErrLifecycle = errors.New("invalid synthetic reservation lifecycle")
	ErrBounds    = errors.New("synthetic funding bounds exceeded")
)

// Binding is compared exactly, never inferred from host or mutable context.
type Binding struct{ Tenant, Request, Source string }
type Kind string

const (
	Reserve Kind = "reserve"
	Append  Kind = "append"
	Settle  Kind = "settle"
	Refund  Kind = "refund"
	// Violation records only an independently identified zero-value observation.
	// Fee amount, funding source and post-refund debit rules require a decision.
	Violation Kind = "violation"
)

// Event IDs are request-local and globally distinct across kinds in that request.
// An identical ID with different kind, amount or binding is never another debit.
type Event struct {
	ID      string
	Binding Binding
	Kind    Kind
	Units   int64
}

// Delta contains signed synthetic state transfers summing to zero, not a real
// double-entry ledger. Consumed is a synthetic sink, not a real account balance
// or platform receipt; it must not be counted again as another asset.
type Delta struct{ Available, Reserved, Consumed int64 }

func (d Delta) Balanced() bool {
	if d.Available < -MaxUnits || d.Available > MaxUnits || d.Reserved < -MaxUnits || d.Reserved > MaxUnits || d.Consumed < -MaxUnits || d.Consumed > MaxUnits {
		return false
	}
	return d.Available+d.Reserved+d.Consumed == 0
}

type Balances struct{ Available, Reserved, Consumed int64 }

// State is immutable to callers. Apply returns a copied event history and never
// mutates its receiver, including on rejected operations. The zero State is invalid.
type State struct {
	binding                Binding
	balances               Balances
	initial                int64
	started, closed, valid bool
	events                 map[string]Event
}

func New(binding Binding, available int64) (State, error) {
	if !validBinding(binding) || available < 0 || available > MaxUnits {
		return State{}, ErrInvalid
	}
	return State{binding: binding, balances: Balances{Available: available}, initial: available, valid: true, events: map[string]Event{}}, nil
}
func (s State) Balances() Balances { return s.balances }
func (s State) Binding() Binding   { return s.binding }
func (s State) Closed() bool       { return s.closed }

// Apply accepts partial settle/refund releases, with no implicit top-up, fee,
// rounding, overdraft or remainder disposition. Reaching zero reserved closes the
// reservation. Zero-value violation observations are allowed before, during or
// after a reservation; that says nothing about fee eligibility or charging policy.
// replay=true returns a zero delta: the original event was already accounted for.
func (s State) Apply(e Event) (next State, delta Delta, replay bool, err error) {
	if !s.valid || !validID(e.ID) {
		return s, Delta{}, false, ErrInvalid
	}
	if e.Binding != s.binding {
		return s, Delta{}, false, ErrBinding
	}
	if old, ok := s.events[e.ID]; ok {
		if old != e {
			return s, Delta{}, false, ErrConflict
		}
		return s, Delta{}, true, nil
	}
	if e.Units < 0 || e.Units > MaxUnits {
		return s, Delta{}, false, ErrBounds
	}
	switch e.Kind {
	case Reserve:
		if s.started {
			return s, Delta{}, false, ErrLifecycle
		}
		if e.Units == 0 {
			return s, Delta{}, false, ErrInvalid
		}
		delta = Delta{Available: -e.Units, Reserved: e.Units}
	case Append:
		if !s.started || s.closed {
			return s, Delta{}, false, ErrLifecycle
		}
		if e.Units == 0 {
			return s, Delta{}, false, ErrInvalid
		}
		delta = Delta{Available: -e.Units, Reserved: e.Units}
	case Settle, Refund:
		if !s.started || s.closed {
			return s, Delta{}, false, ErrLifecycle
		}
		if e.Units == 0 {
			return s, Delta{}, false, ErrInvalid
		}
		delta.Reserved = -e.Units
		if e.Kind == Settle {
			delta.Consumed = e.Units
		} else {
			delta.Available = e.Units
		}
	case Violation:
		if e.Units != 0 {
			return s, Delta{}, false, ErrInvalid
		}
	default:
		return s, Delta{}, false, ErrInvalid
	}
	b := s.balances
	// Each operand lies in [-MaxUnits, MaxUnits], so additions cannot wrap int64.
	b.Available += delta.Available
	b.Reserved += delta.Reserved
	b.Consumed += delta.Consumed
	if b.Available < 0 || b.Reserved < 0 || b.Consumed < 0 || b.Available > MaxUnits || b.Reserved > MaxUnits || b.Consumed > MaxUnits || b.Available+b.Reserved+b.Consumed != s.initial {
		return s, Delta{}, false, ErrBounds
	}
	next = s
	next.balances = b
	next.events = maps.Clone(s.events)
	next.events[e.ID] = e
	if e.Kind == Reserve {
		next.started = true
	}
	if next.started && b.Reserved == 0 {
		next.closed = true
	}
	return next, delta, false, nil
}
func validBinding(b Binding) bool {
	return validID(b.Tenant) && validID(b.Request) && validID(b.Source)
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

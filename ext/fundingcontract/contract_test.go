package fundingcontract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustState(t *testing.T, b Binding, available int64) State {
	t.Helper()
	s, err := New(b, available)
	require.NoError(t, err)
	return s
}

func TestFundingContractLifecycle(t *testing.T) {
	binding := Binding{Tenant: "tenant-1", Request: "request-1", Source: "source-1"}

	t.Run("partial settle then refund keeps one partition", func(t *testing.T) {
		s := mustState(t, binding, 100)
		next, delta, replay, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 60})
		require.NoError(t, err)
		assert.False(t, replay)
		assert.True(t, delta.Balanced())
		assert.Equal(t, Balances{Available: 40, Reserved: 60}, next.Balances())

		next, delta, _, err = next.Apply(Event{ID: "s1", Binding: binding, Kind: Settle, Units: 20})
		require.NoError(t, err)
		assert.True(t, delta.Balanced())
		assert.Equal(t, Balances{Available: 40, Reserved: 40, Consumed: 20}, next.Balances())

		next, delta, _, err = next.Apply(Event{ID: "f1", Binding: binding, Kind: Refund, Units: 15})
		require.NoError(t, err)
		assert.True(t, delta.Balanced())
		assert.Equal(t, Balances{Available: 55, Reserved: 25, Consumed: 20}, next.Balances())
		// Available and Reserved are states of one asset: they never exceed the opening amount.
		b := next.Balances()
		assert.LessOrEqual(t, b.Available+b.Reserved, int64(100))
		// Consumed is not a platform receipt and is never added again.
		assert.Equal(t, int64(100), b.Available+b.Reserved+b.Consumed)
	})

	t.Run("append reservation extends reserved without new money", func(t *testing.T) {
		s := mustState(t, binding, 100)
		s, _, _, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 40})
		require.NoError(t, err)
		s, delta, _, err := s.Apply(Event{ID: "a", Binding: binding, Kind: Append, Units: 10})
		require.NoError(t, err)
		assert.True(t, delta.Balanced())
		assert.Equal(t, Balances{Available: 50, Reserved: 50}, s.Balances())
	})

	t.Run("full settle then refund of remainder closes reservation", func(t *testing.T) {
		s := mustState(t, binding, 100)
		s, _, _, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 100})
		require.NoError(t, err)
		s, _, _, err = s.Apply(Event{ID: "s", Binding: binding, Kind: Settle, Units: 70})
		require.NoError(t, err)
		s, _, _, err = s.Apply(Event{ID: "f", Binding: binding, Kind: Refund, Units: 30})
		require.NoError(t, err)
		assert.Equal(t, Balances{Available: 30, Reserved: 0, Consumed: 70}, s.Balances())
		assert.True(t, s.Closed())
		_, _, _, err = s.Apply(Event{ID: "f2", Binding: binding, Kind: Refund, Units: 1})
		assert.ErrorIs(t, err, ErrLifecycle)
	})

	t.Run("exact replay is idempotent and conflicting replay is rejected", func(t *testing.T) {
		s := mustState(t, binding, 100)
		s, _, _, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 60})
		require.NoError(t, err)
		replayed, delta, replay, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 60})
		require.NoError(t, err)
		assert.True(t, replay)
		assert.Equal(t, Delta{}, delta)
		assert.Equal(t, s.Balances(), replayed.Balances())

		for _, conflicting := range []Event{
			{ID: "r", Binding: binding, Kind: Reserve, Units: 61},
			{ID: "r", Binding: binding, Kind: Append, Units: 60},
		} {
			_, _, _, err := s.Apply(conflicting)
			assert.ErrorIs(t, err, ErrConflict)
		}
		assert.Equal(t, Balances{Available: 40, Reserved: 60}, s.Balances())
	})

	t.Run("mismatched binding never touches another request", func(t *testing.T) {
		s := mustState(t, binding, 100)
		other := Binding{Tenant: "tenant-1", Request: "request-2", Source: "source-1"}
		_, _, _, err := s.Apply(Event{ID: "r", Binding: other, Kind: Reserve, Units: 10})
		assert.ErrorIs(t, err, ErrBinding)
		assert.Equal(t, Balances{Available: 100}, s.Balances())
	})

	t.Run("overflow and negative units are rejected", func(t *testing.T) {
		s := mustState(t, binding, 100)
		_, _, _, err := s.Apply(Event{ID: "big", Binding: binding, Kind: Reserve, Units: MaxUnits + 1})
		assert.ErrorIs(t, err, ErrBounds)
		_, _, _, err = s.Apply(Event{ID: "neg", Binding: binding, Kind: Reserve, Units: -1})
		assert.ErrorIs(t, err, ErrBounds)
		_, _, _, err = s.Apply(Event{ID: "over", Binding: binding, Kind: Reserve, Units: 101})
		assert.ErrorIs(t, err, ErrBounds)
		assert.Equal(t, Balances{Available: 100}, s.Balances())
	})

	t.Run("reserved release beyond reservation is rejected", func(t *testing.T) {
		s := mustState(t, binding, 100)
		s, _, _, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 10})
		require.NoError(t, err)
		_, _, _, err = s.Apply(Event{ID: "s", Binding: binding, Kind: Settle, Units: 11})
		assert.ErrorIs(t, err, ErrBounds)
		assert.Equal(t, Balances{Available: 90, Reserved: 10}, s.Balances())
	})

	t.Run("violation observation carries no money", func(t *testing.T) {
		s := mustState(t, binding, 100)
		s, _, _, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 10})
		require.NoError(t, err)
		s, delta, _, err := s.Apply(Event{ID: "v", Binding: binding, Kind: Violation})
		require.NoError(t, err)
		assert.Equal(t, Delta{}, delta)
		assert.Equal(t, Balances{Available: 90, Reserved: 10}, s.Balances())
		_, _, _, err = s.Apply(Event{ID: "v2", Binding: binding, Kind: Violation, Units: 5})
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("lifecycle order is enforced", func(t *testing.T) {
		s := mustState(t, binding, 100)
		_, _, _, err := s.Apply(Event{ID: "s", Binding: binding, Kind: Settle, Units: 1})
		assert.ErrorIs(t, err, ErrLifecycle)
		s, _, _, err = s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: 10})
		require.NoError(t, err)
		_, _, _, err = s.Apply(Event{ID: "r2", Binding: binding, Kind: Reserve, Units: 10})
		assert.ErrorIs(t, err, ErrLifecycle)
		_, _, _, err = s.Apply(Event{ID: "a", Binding: binding, Kind: Append, Units: 1})
		assert.NoError(t, err)
	})

	t.Run("boundary and immutable branches", func(t *testing.T) {
		s := mustState(t, binding, MaxUnits)
		reserved, d, _, err := s.Apply(Event{ID: "r", Binding: binding, Kind: Reserve, Units: MaxUnits})
		require.NoError(t, err)
		assert.True(t, d.Balanced())
		settled, _, _, err := reserved.Apply(Event{ID: "s", Binding: binding, Kind: Settle, Units: MaxUnits})
		require.NoError(t, err)
		assert.Equal(t, Balances{Consumed: MaxUnits}, settled.Balances())
		assert.Equal(t, Balances{Available: MaxUnits}, s.Balances())
		assert.Equal(t, Balances{Reserved: MaxUnits}, reserved.Balances())
		// The sibling has not inherited the other branch's event history.
		refunded, _, replay, err := reserved.Apply(Event{ID: "s", Binding: binding, Kind: Refund, Units: MaxUnits})
		require.NoError(t, err)
		assert.False(t, replay)
		assert.Equal(t, Balances{Available: MaxUnits}, refunded.Balances())
		assert.False(t, (Delta{Available: 1<<63 - 1, Reserved: 1, Consumed: -1 << 63}).Balanced())
		_, err = New(binding, MaxUnits+1)
		assert.ErrorIs(t, err, ErrInvalid)
		for _, wrong := range []Binding{
			{Tenant: "other", Request: binding.Request, Source: binding.Source},
			{Tenant: binding.Tenant, Request: "other", Source: binding.Source},
			{Tenant: binding.Tenant, Request: binding.Request, Source: "other"},
		} {
			_, _, _, err := reserved.Apply(Event{ID: "r", Binding: wrong, Kind: Reserve, Units: MaxUnits})
			assert.ErrorIs(t, err, ErrBinding)
		}
	})

	t.Run("observation before and after reservation is not a fee", func(t *testing.T) {
		s := mustState(t, binding, 10)
		s, d, _, err := s.Apply(Event{ID: "before", Binding: binding, Kind: Violation})
		require.NoError(t, err)
		assert.Equal(t, Delta{}, d)
		assert.False(t, s.Closed())
		assert.Equal(t, Balances{Available: 10}, s.Balances())
		s, _, _, err = s.Apply(Event{ID: "reserve", Binding: binding, Kind: Reserve, Units: 10})
		require.NoError(t, err)
		s, _, _, err = s.Apply(Event{ID: "refund", Binding: binding, Kind: Refund, Units: 10})
		require.NoError(t, err)
		s, d, _, err = s.Apply(Event{ID: "after", Binding: binding, Kind: Violation})
		require.NoError(t, err)
		assert.Equal(t, Delta{}, d)
		assert.True(t, s.Closed())
		assert.Equal(t, Balances{Available: 10}, s.Balances())
	})

	t.Run("rejected events leave the state untouched", func(t *testing.T) {
		s := mustState(t, binding, 100)
		before := s.Balances()
		next, _, _, err := s.Apply(Event{ID: "bad", Binding: Binding{Tenant: "t"}, Kind: Reserve, Units: 1})
		assert.Error(t, err)
		assert.Equal(t, before, next.Balances())
		assert.Equal(t, before, s.Balances())
	})
}

package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/ext/core"
	"github.com/stretchr/testify/require"
)

func TestAuthScopeIsolation(t *testing.T) {
	site, browser := strings.Repeat("a", 32), strings.Repeat("b", 64)
	a, err := core.NewBrandAuthScope(site, "https://a.example.test", browser)
	require.NoError(t, err)
	otherBrowser, err := core.NewBrandAuthScope(site, "https://a.example.test", strings.Repeat("c", 64))
	require.NoError(t, err)
	b, err := core.NewBrandAuthScope(site, "https://b.example.test", browser)
	require.NoError(t, err)
	require.True(t, a.SameSite(otherBrowser))
	require.False(t, a.SameBrowser(otherBrowser))
	require.False(t, a.SameSite(b))
	require.False(t, a.SameSite(core.MainAuthScope()))
	require.False(t, (core.AuthScope{}).SameSite(core.AuthScope{}))
	for _, ctx := range []context.Context{nil, context.Background(), core.WithAuthScope(context.Background(), core.AuthScope{})} {
		_, err := core.RequestAuthScope(ctx)
		require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	}
	got, err := core.RequestAuthScope(core.WithAuthScope(context.Background(), a))
	require.NoError(t, err)
	require.Equal(t, a, got)
	got, err = core.RequestAuthScope(core.WithAuthScope(context.Background(), core.MainAuthScope()))
	require.NoError(t, err)
	require.True(t, got.IsMain())
	for _, origin := range []string{
		"http://a.example.test", "https://A.example.test", "https://a.example.test/", "https://a.example.test:443",
		"https://user@a.example.test", "https://a.example.test?", "https://a.example.test#", "https://a.example.test/path",
		"https://*.example.test", "https://a.example.test.", "https://a.example.test:", "https://a.example.test:0",
		"https://a.example.test:65536", "https://a.example.test:0443", "https://a.example.test\n",
		"https://-bad.example", "https://bad_.example", "https://a..example", "https://[not-ip]", "https://[not-ip]:8443", "https://[2001:0db8::1]",
	} {
		t.Run(origin, func(t *testing.T) {
			_, err := core.NewBrandAuthScope(site, origin, browser)
			require.ErrorIs(t, err, core.ErrAuthScopeRejected)
		})
	}
	_, err = core.NewBrandAuthScope("bad", "https://a.example.test", browser)
	require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	_, err = core.NewBrandAuthScope(site, "https://a.example.test", "bad")
	require.ErrorIs(t, err, core.ErrAuthScopeRejected)
}

func TestAuthScopeBinding(t *testing.T) {
	const now int64 = 1700000000
	site, browser := strings.Repeat("a", 32), strings.Repeat("b", 64)
	a, err := core.NewBrandAuthScope(site, "https://a.example.test", browser)
	require.NoError(t, err)
	otherBrowser, err := core.NewBrandAuthScope(site, "https://a.example.test", strings.Repeat("c", 64))
	require.NoError(t, err)
	otherSite, err := core.NewBrandAuthScope(strings.Repeat("d", 32), "https://a.example.test", browser)
	require.NoError(t, err)
	for _, kind := range []core.AuthBindingKind{core.AuthBindingFlow, core.AuthBindingSession} {
		t.Run(string(kind), func(t *testing.T) {
			binding, err := core.NewAuthScopeBinding(a, kind, "official_record-1", 42, now+60, now)
			require.NoError(t, err)
			require.NoError(t, binding.Check(a, kind, "official_record-1", 42, now))
			for _, scope := range []core.AuthScope{core.MainAuthScope(), {}, otherBrowser, otherSite} {
				require.ErrorIs(t, binding.Check(scope, kind, "official_record-1", 42, now), core.ErrAuthScopeRejected)
			}
			require.ErrorIs(t, binding.Check(a, "different", "official_record-1", 42, now), core.ErrAuthScopeRejected)
			require.ErrorIs(t, binding.Check(a, kind, "different", 42, now), core.ErrAuthScopeRejected)
			require.ErrorIs(t, binding.Check(a, kind, "official_record-1", 43, now), core.ErrAuthScopeRejected)
			require.ErrorIs(t, binding.Check(a, kind, "official_record-1", 42, now+60), core.ErrAuthScopeRejected)
			require.ErrorIs(t, binding.Check(a, kind, "official_record-1", 42, 0), core.ErrAuthScopeRejected)
		})
	}
	for _, record := range []string{"", "contains space", "raw.secret", "https://host", "非ASCII", strings.Repeat("x", 256)} {
		_, err := core.NewAuthScopeBinding(a, core.AuthBindingSession, record, 42, now+60, now)
		require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	}
	_, err = core.NewAuthScopeBinding(core.AuthScope{}, core.AuthBindingSession, "id", 42, now+60, now)
	require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	_, err = core.NewAuthScopeBinding(a, "unknown", "id", 42, now+60, now)
	require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	_, err = core.NewAuthScopeBinding(a, core.AuthBindingSession, "id", 0, now+60, now)
	require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	_, err = core.NewAuthScopeBinding(a, core.AuthBindingSession, "id", 42, now, now)
	require.ErrorIs(t, err, core.ErrAuthScopeRejected)
	require.ErrorIs(t, (core.AuthScopeBinding{}).Check(core.MainAuthScope(), core.AuthBindingSession, "", 0, now), core.ErrAuthScopeRejected)
	mainBinding, err := core.NewAuthScopeBinding(core.MainAuthScope(), core.AuthBindingSession, "id", 42, now+60, now)
	require.NoError(t, err)
	require.NoError(t, mainBinding.Check(core.MainAuthScope(), core.AuthBindingSession, "id", 42, now))
	require.ErrorIs(t, mainBinding.Check(a, core.AuthBindingSession, "id", 42, now), core.ErrAuthScopeRejected)
}

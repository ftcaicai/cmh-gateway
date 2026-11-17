package core

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var ErrAuthScopeRejected = errors.New("authentication scope rejected")

// AuthScope is a trusted host projection, not an authentication grant.
// Constructors belong to trusted Go host code, never request JSON decoding.
// Zero and missing scopes are invalid rather than implicit main-site scopes.
type AuthScope struct {
	kind                  uint8
	site, origin, browser string
}

func MainAuthScope() AuthScope { return AuthScope{kind: 1} }

// NewBrandAuthScope requires a canonical HTTPS origin and a hash of a random
// browser secret. Domain ownership and proxy trust remain caller obligations.
func NewBrandAuthScope(site, origin, browser string) (AuthScope, error) {
	if !authScopeHex(site, 32) || !authScopeHex(browser, 64) {
		return AuthScope{}, ErrAuthScopeRejected
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || u.Host != strings.ToLower(u.Host) || strings.HasSuffix(u.Hostname(), ".") || strings.HasSuffix(u.Host, ":") {
		return AuthScope{}, ErrAuthScopeRejected
	}
	for _, ch := range u.Host {
		if ch <= 32 || ch >= 127 || ch == '\\' || ch == '%' || ch == '*' {
			return AuthScope{}, ErrAuthScopeRejected
		}
	}
	if u.Hostname() == "" || u.Port() == "443" {
		return AuthScope{}, ErrAuthScopeRejected
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.String() != host || ip.Zone() != "" || (ip.Is6() != strings.HasPrefix(u.Host, "[")) {
			return AuthScope{}, ErrAuthScopeRejected
		}
	} else {
		if strings.ContainsAny(u.Host, "[]") || strings.Contains(host, ":") || len(host) > 253 {
			return AuthScope{}, ErrAuthScopeRejected
		}
		for label := range strings.SplitSeq(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return AuthScope{}, ErrAuthScopeRejected
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
					return AuthScope{}, ErrAuthScopeRejected
				}
			}
		}
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return AuthScope{}, ErrAuthScopeRejected
		}
	}
	// Parsing can discard empty query/fragment delimiters; round-trip equality
	// prevents those alternate spellings from becoming separate authorities.
	if u.String() != origin || strings.Contains(origin, "#") {
		return AuthScope{}, ErrAuthScopeRejected
	}
	return AuthScope{kind: 2, site: site, origin: origin, browser: browser}, nil
}

func authScopeHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, ch := range value {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

func (s AuthScope) Valid() bool           { return s.kind == 1 || s.kind == 2 }
func (s AuthScope) IsMain() bool          { return s.kind == 1 }
func (s AuthScope) SiteID() string        { return s.site }
func (s AuthScope) Origin() string        { return s.origin }
func (s AuthScope) BrowserDigest() string { return s.browser }
func (s AuthScope) SameSite(other AuthScope) bool {
	return s.Valid() && other.Valid() && s.kind == other.kind && s.site == other.site && s.origin == other.origin
}
func (s AuthScope) SameBrowser(other AuthScope) bool {
	return s.SameSite(other) && s.browser == other.browser
}

// AuthBindingKind separates login-flow authorization from issued sessions.
type AuthBindingKind string

const (
	AuthBindingFlow    AuthBindingKind = "flow"
	AuthBindingSession AuthBindingKind = "session"
)

// AuthScopeBinding is an immutable, credential-free projection for a trusted
// persistence participant. Creating one does not authenticate its record ID.
// The participant must validate the official row and active relationship in
// the same model-owned transaction before persisting this projection.
type AuthScopeBinding struct {
	scope     AuthScope
	kind      AuthBindingKind
	recordID  string
	userID    int
	expiresAt int64
}

func NewAuthScopeBinding(scope AuthScope, kind AuthBindingKind, recordID string, userID int, expiresAt, now int64) (AuthScopeBinding, error) {
	if !scope.Valid() || (kind != AuthBindingFlow && kind != AuthBindingSession) || recordID == "" || len(recordID) > 255 || userID <= 0 || now <= 0 || expiresAt <= now {
		return AuthScopeBinding{}, ErrAuthScopeRejected
	}
	// Official identifiers are opaque ASCII values, never URLs or raw secrets.
	// This validates shape only; the caller supplies TokenHash for a flow and SID
	// for a session, not their usable bearer/refresh/flow credentials.
	for _, ch := range recordID {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return AuthScopeBinding{}, ErrAuthScopeRejected
		}
	}
	return AuthScopeBinding{scope: scope, kind: kind, recordID: recordID, userID: userID, expiresAt: expiresAt}, nil
}

func (b AuthScopeBinding) Scope() AuthScope      { return b.scope }
func (b AuthScopeBinding) Kind() AuthBindingKind { return b.kind }
func (b AuthScopeBinding) RecordID() string      { return b.recordID }
func (b AuthScopeBinding) UserID() int           { return b.userID }
func (b AuthScopeBinding) ExpiresAt() int64      { return b.expiresAt }

// Check requires the exact durable record/user/kind and initiating browser for
// both flow and session requests. Site equality alone must not be mistaken for
// sufficient credential or relationship verification. Expiry is inclusive.
// The explicit clock is supplied by trusted host code, not request input.
func (b AuthScopeBinding) Check(scope AuthScope, kind AuthBindingKind, recordID string, userID int, now int64) error {
	if b.userID <= 0 || now <= 0 || b.expiresAt <= now || b.kind != kind || b.recordID != recordID || b.userID != userID || !b.scope.SameBrowser(scope) {
		return ErrAuthScopeRejected
	}
	return nil
}

type authScopeContextKey struct{}

func WithAuthScope(ctx context.Context, scope AuthScope) context.Context {
	return context.WithValue(ctx, authScopeContextKey{}, scope)
}
func RequestAuthScope(ctx context.Context) (AuthScope, error) {
	if ctx == nil {
		return AuthScope{}, ErrAuthScopeRejected
	}
	scope, ok := ctx.Value(authScopeContextKey{}).(AuthScope)
	if !ok || !scope.Valid() {
		return AuthScope{}, ErrAuthScopeRejected
	}
	return scope, nil
}

package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DestinationAdmission contains only non-secret identities. URL paths and query
// strings may contain credentials, so neither raw addresses nor headers cross
// this boundary. A destination approval is not source/config or billing authority.
type DestinationAdmission struct {
	Kind, Method, AddressSHA256, HostSHA256 string
	Model                                   ModelIdentity
	Channel                                 ChannelIdentity
	// Plugin fetches may run before selection or after settlement. They bind
	// their actual source/capability and hook instead of fabricating a model.
	Executor ExecutorIdentity
	Hook     string
}

type DestinationGate interface {
	CheckDestination(context.Context, DestinationAdmission) error
}

type noopDestinationGate struct{}

func (noopDestinationGate) CheckDestination(context.Context, DestinationAdmission) error { return nil }

// Destinations is replaced only at startup. Defaults preserve host behavior.
var Destinations DestinationGate = noopDestinationGate{}

func DestinationAdmissionEnabled() bool {
	_, noop := Destinations.(noopDestinationGate)
	return !noop
}

// DestinationAddress hashes a strict, absolute transport URL. The operator and
// host must use this same projection. Query/order/escaping changes intentionally
// change approval; no wildcard, prefix, DNS or nationality inference occurs.
// Only scheme and host case and an absent root path are normalized. Port spelling
// and query bytes remain exact. Reject fragments/userinfo/scoped IPv6 zones
// instead of hiding them or collapsing case-sensitive interface identifiers.
func DestinationAddress(raw string) (string, error) {
	if raw == "" || len(raw) > 64*1024 || !utf8.ValidString(raw) || strings.ContainsAny(raw, "\\#") || strings.ContainsFunc(raw, unicode.IsSpace) || strings.ContainsFunc(raw, unicode.IsControl) {
		return "", errors.New("invalid destination")
	}
	target, err := url.Parse(raw)
	if err != nil || target.Opaque != "" || target.User != nil || target.Fragment != "" || target.Hostname() == "" || strings.Contains(target.Host, "%") {
		return "", errors.New("invalid destination")
	}
	target.Scheme = strings.ToLower(target.Scheme)
	switch target.Scheme {
	case "http", "https", "ws", "wss":
	default:
		return "", errors.New("invalid destination")
	}
	for _, c := range target.Host {
		if c > unicode.MaxASCII {
			return "", errors.New("invalid destination")
		}
	}
	target.Host = strings.ToLower(target.Host)
	if target.Path == "" {
		target.Path = "/"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(target.String()))), nil
}

// DestinationHost fingerprints the host value supplied by a transport bridge:
// HTTP req.Host, or the explicit WebSocket Host header (empty means URL authority).
// The URL hash separately binds the authority in both cases. Case differences
// intentionally require a new exact override approval.
func DestinationHost(hostOverride string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(hostOverride)))
}

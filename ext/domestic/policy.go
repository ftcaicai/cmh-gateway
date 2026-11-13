// Package domestic implements an explicit publication allowlist. Membership
// means operator authorization, not an inference about a model's nationality.
// Policies are immutable and fail closed; protocol/adaptor readiness, pricing,
// tenant authorization and upstream identity must still be checked by the host.
package domestic

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// These strings may be injected by the extension release builder with -ldflags
// -X. Defaults intentionally publish nothing. No credentials belong here.
var (
	BuildModelAllowlist       string
	BuildChannelTypeAllowlist string
)

// Policy is an immutable exact-match set. The zero/nil policy authorizes no
// models or channel types. Never interpret an empty policy as allow-all.
type Policy struct {
	models       map[string]struct{}
	channelTypes map[int]struct{}
}

// Load combines build-time and environment configuration at startup. It does
// not cache environment reads in a sync.Once or mutate shared state. A caller
// may install a validated policy atomically; a parse failure must not replace
// the installed policy with permissive defaults or echo raw configuration.
func Load() (*Policy, error) {
	return New(
		[]string{BuildModelAllowlist, os.Getenv("CMH_MODEL_ALLOWLIST")},
		[]string{BuildChannelTypeAllowlist, os.Getenv("CMH_CHANNEL_TYPE_ALLOWLIST")},
	)
}

// New unions sources with comma-separated entries. Whole empty sources are
// permitted for opt-in configuration. Empty entries, control characters and
// malformed type IDs are rejected rather than partially accepting a typo.
func New(modelSources, channelTypeSources []string) (*Policy, error) {
	p := &Policy{models: make(map[string]struct{}), channelTypes: make(map[int]struct{})}
	for _, source := range modelSources {
		if strings.TrimSpace(source) == "" {
			continue
		}
		for entry := range strings.SplitSeq(source, ",") {
			if !utf8.ValidString(entry) || strings.ContainsFunc(entry, unicode.IsControl) {
				return nil, errors.New("invalid extension model allowlist")
			}
			name := strings.TrimSpace(entry)
			if name == "" || len(name) > 255 {
				return nil, errors.New("invalid extension model allowlist")
			}
			p.models[name] = struct{}{}
		}
	}
	for _, source := range channelTypeSources {
		if strings.TrimSpace(source) == "" {
			continue
		}
		for entry := range strings.SplitSeq(source, ",") {
			if !utf8.ValidString(entry) || strings.ContainsFunc(entry, unicode.IsControl) {
				return nil, errors.New("invalid extension channel type allowlist")
			}
			entry = strings.TrimSpace(entry)
			// Canonical positive decimal IDs only: reject signs, scientific
			// notation and leading zeros that can hide configuration mistakes.
			id, err := strconv.Atoi(entry)
			if err != nil || id <= 0 || strconv.Itoa(id) != entry {
				return nil, errors.New("invalid extension channel type allowlist")
			}
			p.channelTypes[id] = struct{}{}
		}
	}
	return p, nil
}

// AllowsModel checks the public identifier verbatim. It deliberately performs
// no prefix, case folding, wildcard, alias, namespace or upstream-map expansion.
func (p *Policy) AllowsModel(publicName string) bool {
	if p == nil {
		return false
	}
	_, ok := p.models[publicName]
	return ok
}

func (p *Policy) AllowsChannelType(channelType int) bool {
	if p == nil {
		return false
	}
	_, ok := p.channelTypes[channelType]
	return ok
}

// FilterModels preserves the host's ordering and exact spelling, removes
// duplicates and always returns a new slice, so a cached catalog is not mutated.
func (p *Policy) FilterModels(publicNames []string) []string {
	filtered := make([]string, 0, len(publicNames))
	seen := make(map[string]struct{}, len(publicNames))
	for _, name := range publicNames {
		if !p.AllowsModel(name) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		filtered = append(filtered, name)
	}
	return filtered
}

package domestic

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
)

// OperatorManifest is a credential-free approval document, not installed-plugin
// discovery. Generate its canonical bytes with EncodeOperatorManifest. Addresses
// are hashes: private URLs, keys and configuration bytes do not belong here.
type OperatorManifest struct {
	Version        int
	PublicModels   []string
	UpstreamModels []string
	ChannelTypes   []int
	Executors      []core.ExecutorIdentity
	Destinations   []core.DestinationAdmission
}

// OperatorPolicies contains validated immutable policies. Construction never
// installs them: a partially integrated gateway must not expose an enable switch.
type OperatorPolicies struct {
	Models       *Gate
	Destinations *DestinationPolicy
	Fingerprint  string
}

const maxOperatorManifestBytes = 1 << 20

// EncodeOperatorManifest is the only supported wire projection. Strict canonical
// round-tripping in Parse rejects unknown fields, duplicate members, ambiguous
// spellings and unsupported versions rather than silently ignoring operator input.
// It is intentionally not a general-purpose JSON configuration format.
func EncodeOperatorManifest(manifest OperatorManifest) ([]byte, error) {
	if manifest.Version != 1 || len(manifest.PublicModels) > 4096 || len(manifest.UpstreamModels) > 4096 || len(manifest.ChannelTypes) > 256 || len(manifest.Executors) > 256 || len(manifest.Destinations) > 4096 {
		return nil, errGateConfiguration
	}
	for _, names := range [][]string{manifest.PublicModels, manifest.UpstreamModels} {
		for _, name := range names {
			if len(name) > 255 || !utf8.ValidString(name) {
				return nil, errGateConfiguration
			}
		}
	}
	for _, executor := range manifest.Executors {
		for _, value := range []string{executor.Kind, executor.Key, executor.SourceSHA256, executor.ConfigFingerprint} {
			if len(value) > 255 || !utf8.ValidString(value) {
				return nil, errGateConfiguration
			}
		}
	}
	for _, destination := range manifest.Destinations {
		for _, value := range []string{destination.Kind, destination.Method, destination.AddressSHA256, destination.HostSHA256, destination.Model.Public, destination.Model.Declared, destination.Model.Upstream, destination.Executor.Kind, destination.Executor.Key, destination.Executor.SourceSHA256, destination.Executor.ConfigFingerprint, destination.Hook} {
			if len(value) > 255 || !utf8.ValidString(value) {
				return nil, errGateConfiguration
			}
		}
	}
	encoded, err := common.Marshal(manifest)
	if err != nil || len(encoded) > maxOperatorManifestBytes {
		return nil, errGateConfiguration
	}
	if _, err = ParseOperatorManifest(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func ParseOperatorManifest(document []byte) (*OperatorPolicies, error) {
	if len(document) == 0 || len(document) > maxOperatorManifestBytes || !utf8.Valid(document) {
		return nil, errGateConfiguration
	}
	var manifest OperatorManifest
	if common.Unmarshal(document, &manifest) != nil || manifest.Version != 1 || len(manifest.PublicModels) > 4096 || len(manifest.UpstreamModels) > 4096 || len(manifest.ChannelTypes) > 256 || len(manifest.Executors) > 256 || len(manifest.Destinations) > 4096 {
		return nil, errGateConfiguration
	}
	canonical, err := common.Marshal(manifest)
	if err != nil || !bytes.Equal(bytes.TrimSpace(document), canonical) {
		return nil, errGateConfiguration
	}
	publicSet := make(map[string]struct{}, len(manifest.PublicModels))
	upstreamSet := make(map[string]struct{}, len(manifest.UpstreamModels))
	for index, names := range [][]string{manifest.PublicModels, manifest.UpstreamModels} {
		set := publicSet
		if index == 1 {
			set = upstreamSet
		}
		for _, name := range names {
			// Each entry is one exact model, never a CSV fragment or normalized alias.
			policy, err := New([]string{name}, nil)
			if err != nil || !policy.AllowsModel(name) || bytes.ContainsRune([]byte(name), ',') {
				return nil, errGateConfiguration
			}
			if _, duplicate := set[name]; duplicate {
				return nil, errGateConfiguration
			}
			set[name] = struct{}{}
		}
	}
	channelSet := make(map[int]struct{}, len(manifest.ChannelTypes))
	channelStrings := make([]string, 0, len(manifest.ChannelTypes))
	for _, id := range manifest.ChannelTypes {
		if id <= 0 {
			return nil, errGateConfiguration
		}
		if _, duplicate := channelSet[id]; duplicate {
			return nil, errGateConfiguration
		}
		channelSet[id] = struct{}{}
		channelStrings = append(channelStrings, fmt.Sprint(id))
	}
	public, err := New(manifest.PublicModels, channelStrings)
	if err != nil {
		return nil, errGateConfiguration
	}
	upstream, err := New(manifest.UpstreamModels, nil)
	if err != nil {
		return nil, errGateConfiguration
	}
	seen := make(map[executorApproval]struct{}, len(manifest.Executors))
	for _, executor := range manifest.Executors {
		approval, valid := approvedExecutorIdentity(executor)
		// Builtins need no artifact approval. Generation is a host marker,
		// not operator authority; reject it instead of suggesting a pinned grant.
		if !valid || executor.Kind != "plugin" || executor.Generation != 0 {
			return nil, errGateConfiguration
		}
		if _, duplicate := seen[approval]; duplicate {
			return nil, errGateConfiguration
		}
		seen[approval] = struct{}{}
	}
	models, err := NewGate(public, upstream, manifest.Executors)
	if err != nil {
		return nil, errGateConfiguration
	}
	for _, destination := range manifest.Destinations {
		if destination.Executor.Generation != 0 {
			return nil, errGateConfiguration
		}
	}
	destinations, err := NewDestinationPolicy(manifest.Destinations)
	if err != nil {
		return nil, errGateConfiguration
	}
	return &OperatorPolicies{Models: models, Destinations: destinations, Fingerprint: fmt.Sprintf("%x", sha256.Sum256(canonical))}, nil
}

package domestic

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
)

// ParseExecutorApprovals reads explicit operator approvals, not installed plugin
// discovery. Each nonblank record is exactly key<TAB>source-sha256<TAB>config-sha256.
// No wildcard, comment, case folding or whitespace cleanup is performed. An empty
// document approves no plugins. Failed parsing returns no partial approval set.
func ParseExecutorApprovals(document string) ([]core.ExecutorIdentity, error) {
	if len(document) > 64*1024 || !utf8.ValidString(document) {
		return nil, errGateConfiguration
	}
	approvals := make([]core.ExecutorIdentity, 0)
	seen := make(map[executorApproval]struct{})
	for line := range strings.SplitSeq(document, "\n") {
		line = strings.TrimSuffix(line, "\r") // LF records may have one terminal CR.
		if line == "" {
			continue
		}
		key, rest, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, errGateConfiguration
		}
		source, config, ok := strings.Cut(rest, "\t")
		if !ok || strings.Contains(config, "\t") {
			return nil, errGateConfiguration
		}
		executor := core.ExecutorIdentity{Kind: "plugin", Key: key, SourceSHA256: source, ConfigFingerprint: config}
		approval, valid := approvedExecutorIdentity(executor)
		if !valid || len(approvals) >= 256 {
			return nil, errGateConfiguration
		}
		if _, duplicate := seen[approval]; duplicate {
			return nil, errGateConfiguration
		}
		seen[approval] = struct{}{}
		approvals = append(approvals, executor)
	}
	return approvals, nil
}

// EffectiveConfigFingerprint hashes the EXACT effective JSON object bytes the
// host will pass to this plugin, after host schema validation/default expansion.
// It does not attempt to repeat or infer that expansion. Source and configuration
// have separate approvals; a plugin version label never substitutes for either.
//
// Serialization changes (including whitespace or key order) intentionally revoke
// the match. The caller must use the same frozen host-produced bytes when
// generating the approval and executing the hook. This is fail-closed, not a
// canonical-JSON promise. Do not log the bytes: they may contain private settings.
func EffectiveConfigFingerprint(effectiveJSON []byte) (string, error) {
	if len(effectiveJSON) == 0 || len(effectiveJSON) > 64*1024 || !utf8.Valid(effectiveJSON) {
		return "", errGateConfiguration
	}
	var object map[string]common.RawMessage
	if common.Unmarshal(effectiveJSON, &object) != nil || object == nil {
		return "", errGateConfiguration
	}
	digest := sha256.Sum256(effectiveJSON)
	return fmt.Sprintf("%x", digest), nil
}

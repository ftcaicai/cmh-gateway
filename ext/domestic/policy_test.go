package domestic

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContinuationGrantPolicyAndManifest(t *testing.T) {
	request := core.ContinuationGrantRequest{Binding: core.ContinuationBinding{TaskID: "local-task", UserID: 7, ChannelID: 9}, Ref: core.ContinuationRef{ID: strings.Repeat("a", 64), Digest: strings.Repeat("b", 64)}, Purpose: core.ContinuationPoll, PluginKey: "cmh-fixture", SourceSHA256: strings.Repeat("c", 64), RuntimeSHA256: strings.Repeat("d", 64)}
	grant := core.ContinuationGrant{Request: request, Hooks: []string{"buildQueryRequest", "parseTaskResult"}, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	policy, err := NewContinuationAuthorizer([]core.ContinuationGrant{grant})
	require.NoError(t, err)
	grant.Hooks[0] = "buildSubmitRequest"
	approved, found, err := policy.Authorize(t.Context(), request)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"buildQueryRequest", "parseTaskResult"}, approved.Hooks)
	approved.Hooks[0] = "mutated"
	again, found, err := policy.Authorize(t.Context(), request)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "buildQueryRequest", again.Hooks[0])
	mismatch := request
	mismatch.RuntimeSHA256 = strings.Repeat("e", 64)
	_, found, err = policy.Authorize(t.Context(), mismatch)
	require.NoError(t, err)
	assert.False(t, found)
	grant = again
	for _, alter := range []func(*core.ContinuationGrant){func(g *core.ContinuationGrant) { g.Hooks = []string{"buildSubmitRequest", "parseTaskResult"} }, func(g *core.ContinuationGrant) { g.Hooks = []string{"buildQueryRequest", "buildQueryRequest"} }, func(g *core.ContinuationGrant) { g.Hooks = []string{"parseTaskResult"} }, func(g *core.ContinuationGrant) { g.ExpiresAt = 0 }, func(g *core.ContinuationGrant) { g.ExpiresAt = time.Now().Add(-time.Hour).Unix() }, func(g *core.ContinuationGrant) { g.Request.Purpose = core.ContinuationContent }, func(g *core.ContinuationGrant) { g.Request.SourceSHA256 = strings.Repeat("A", 64) }, func(g *core.ContinuationGrant) { g.Request.Binding.UserID = 0 }} {
		invalid := grant
		alter(&invalid)
		got, err := NewContinuationAuthorizer([]core.ContinuationGrant{invalid})
		require.Error(t, err)
		assert.Nil(t, got)
	}
	_, err = NewContinuationAuthorizer([]core.ContinuationGrant{grant, grant})
	require.Error(t, err)
	empty, err := NewContinuationAuthorizer(nil)
	require.NoError(t, err)
	_, found, err = empty.Authorize(t.Context(), request)
	require.NoError(t, err)
	assert.False(t, found)
	before := core.ContinuationGrants
	wire, err := EncodeOperatorManifest(OperatorManifest{Version: 1})
	require.NoError(t, err)
	assert.Equal(t, `{"Version":1,"PublicModels":null,"UpstreamModels":null,"ChannelTypes":null,"Executors":null,"Destinations":null}`, string(wire), "empty optional grants preserve existing canonical approval bytes")
	withGrant, err := EncodeOperatorManifest(OperatorManifest{Version: 1, Continuations: []core.ContinuationGrant{grant}})
	require.NoError(t, err)
	compiled, err := ParseOperatorManifest(withGrant)
	require.NoError(t, err)
	require.NotNil(t, compiled.Continuations)
	_, found, err = compiled.Continuations.Authorize(t.Context(), request)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, before, core.ContinuationGrants)
}

func TestOperatorManifestFileVerification(t *testing.T) {
	originalBuild := BuildOperatorManifestSHA256
	BuildOperatorManifestSHA256 = ""
	t.Cleanup(func() { BuildOperatorManifestSHA256 = originalBuild })
	beforeModels, beforeTargets := core.Models, core.Destinations
	inactive, err := LoadOperatorPolicies(nil)
	require.NoError(t, err)
	assert.Nil(t, inactive)
	wire, err := EncodeOperatorManifest(OperatorManifest{Version: 1, PublicModels: []string{"public-chat"}, UpstreamModels: []string{"approved-target"}, ChannelTypes: []int{1}})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "approval.json")
	require.NoError(t, os.WriteFile(path, wire, 0600))
	digest := fmt.Sprintf("%x", sha256.Sum256(wire))
	environment := map[string]string{"CMH_OPERATOR_MANIFEST_FILE": path, "CMH_OPERATOR_MANIFEST_SHA256": digest}
	loaded, err := LoadOperatorPolicies(environment)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, beforeModels, core.Models)
	assert.Equal(t, beforeTargets, core.Destinations)
	BuildOperatorManifestSHA256 = digest
	delete(environment, "CMH_OPERATOR_MANIFEST_SHA256")
	built, err := LoadOperatorPolicies(environment)
	require.NoError(t, err)
	assert.Equal(t, loaded.Fingerprint, built.Fingerprint)
	environment["CMH_OPERATOR_MANIFEST_SHA256"] = strings.Repeat("a", 64)
	got, err := LoadOperatorPolicies(environment)
	require.Error(t, err)
	assert.Nil(t, got)
	environment["CMH_OPERATOR_MANIFEST_SHA256"] = digest
	require.NoError(t, os.WriteFile(path, append(wire, '\n'), 0600))
	got, err = LoadOperatorPolicies(environment)
	require.Error(t, err)
	assert.Nil(t, got, "even harmless trailing bytes need a new raw-file anchor")
	for _, bad := range []map[string]string{{"CMH_OPERATOR_MANIFEST_FILE": path}, {"CMH_OPERATOR_MANIFEST_SHA256": digest}, {"CMH_OPERATOR_MANIFEST_FILE": filepath.Dir(path), "CMH_OPERATOR_MANIFEST_SHA256": digest}, {"CMH_OPERATOR_MANIFEST_FILE": path + "-missing", "CMH_OPERATOR_MANIFEST_SHA256": digest}, {"CMH_OPERATOR_MANIFEST_FILE": path, "CMH_OPERATOR_MANIFEST_SHA256": "bad-hash"}} {
		BuildOperatorManifestSHA256 = ""
		got, err := LoadOperatorPolicies(bad)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.NotContains(t, err.Error(), path)
	}
	oversized := make([]byte, (1<<20)+1)
	require.NoError(t, os.WriteFile(path, oversized, 0600))
	environment["CMH_OPERATOR_MANIFEST_SHA256"] = fmt.Sprintf("%x", sha256.Sum256(oversized))
	got, err = LoadOperatorPolicies(environment)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, beforeModels, core.Models)
	assert.Equal(t, beforeTargets, core.Destinations)
}

func TestOperatorManifestCompilation(t *testing.T) {
	oldModels, oldDestinations := core.Models, core.Destinations
	manifest := OperatorManifest{Version: 1, PublicModels: []string{"public-chat"}, UpstreamModels: []string{"approved-target"}, ChannelTypes: []int{1}}
	wire, err := EncodeOperatorManifest(manifest)
	require.NoError(t, err)
	policies, err := ParseOperatorManifest(wire)
	require.NoError(t, err)
	require.Len(t, policies.Fingerprint, 64)
	assert.Equal(t, oldModels, core.Models)
	assert.Equal(t, oldDestinations, core.Destinations)
	candidate := core.Admission{Phase: core.AdmissionDispatch, Model: core.ModelIdentity{Public: "public-chat", Upstream: "approved-target"}, Channel: core.ChannelIdentity{Known: true, ID: 1, Type: 1}, Executor: core.ExecutorIdentity{Kind: "builtin"}}
	assert.NoError(t, policies.Models.Check(t.Context(), candidate))
	candidate.Model.Upstream = "unapproved-target"
	assert.ErrorIs(t, policies.Models.Check(t.Context(), candidate), ErrAdmissionDenied)
	manifest.PublicModels[0] = "mutated"
	candidate.Model.Upstream = "approved-target"
	assert.NoError(t, policies.Models.Check(t.Context(), candidate), "compiled maps cannot share caller mutable arrays")
	padding, err := ParseOperatorManifest(append([]byte(" \n"), append(wire, '\n')...))
	require.NoError(t, err)
	assert.Equal(t, policies.Fingerprint, padding.Fingerprint)
	for _, bad := range [][]byte{[]byte(`null`), []byte(`{}`), []byte(strings.Replace(string(wire), `"Version":1`, `"Version":2`, 1)), []byte(strings.Replace(string(wire), `"Version":1`, `"Version":1,"Version":1`, 1)), []byte(strings.Replace(string(wire), `"Version":1`, `"Unexpected":"private-detail","Version":1`, 1)), []byte(strings.Replace(string(wire), `"Version":1`, `"Version": 1`, 1)), {0xff}, make([]byte, (1<<20)+1)} {
		got, err := ParseOperatorManifest(bad)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.NotContains(t, err.Error(), "private-detail")
	}
	base := OperatorManifest{Version: 1, PublicModels: []string{"public-chat"}, UpstreamModels: []string{"approved-target"}, ChannelTypes: []int{1}}
	for _, change := range []func(*OperatorManifest){func(m *OperatorManifest) { m.PublicModels = []string{string([]byte{0xff})} }, func(m *OperatorManifest) { m.UpstreamModels = []string{string([]byte{0xff})} }, func(m *OperatorManifest) { m.PublicModels = []string{"public-chat", "public-chat"} }, func(m *OperatorManifest) { m.PublicModels = []string{" public-chat"} }, func(m *OperatorManifest) { m.PublicModels = []string{"public-chat,other"} }, func(m *OperatorManifest) { m.ChannelTypes = []int{1, 1} }, func(m *OperatorManifest) { m.ChannelTypes = []int{0} }, func(m *OperatorManifest) { m.Executors = []core.ExecutorIdentity{{Kind: "builtin"}} }, func(m *OperatorManifest) { m.Destinations = []core.DestinationAdmission{{Kind: "unknown"}} }} {
		changed := base
		change(&changed)
		_, err := EncodeOperatorManifest(changed)
		require.Error(t, err)
	}
	_, _, executor, _ := admissionFixture(t)
	executor.Generation = 0
	pluginManifest := base
	pluginManifest.Executors = []core.ExecutorIdentity{executor}
	address, err := core.DestinationAddress("https://fixture.invalid/task")
	require.NoError(t, err)
	destination := core.DestinationAdmission{Kind: "plugin_fetch", Method: "POST", AddressSHA256: address, HostSHA256: core.DestinationHost("fixture.invalid"), Executor: executor, Hook: "report"}
	pluginManifest.Destinations = []core.DestinationAdmission{destination}
	pluginWire, err := EncodeOperatorManifest(pluginManifest)
	require.NoError(t, err)
	pluginPolicies, err := ParseOperatorManifest(pluginWire)
	require.NoError(t, err)
	assert.NoError(t, pluginPolicies.Models.CheckExecutor(t.Context(), executor))
	assert.NoError(t, pluginPolicies.Destinations.CheckDestination(t.Context(), destination))
	invalidDestination := pluginManifest
	invalidDestination.Destinations = append([]core.DestinationAdmission{}, pluginManifest.Destinations...)
	invalidDestination.Destinations[0].Hook = string([]byte{0xff})
	_, err = EncodeOperatorManifest(invalidDestination)
	require.Error(t, err)
	duplicate := pluginManifest
	duplicate.Executors = append(append([]core.ExecutorIdentity{}, duplicate.Executors...), executor)
	_, err = EncodeOperatorManifest(duplicate)
	require.Error(t, err)
	generation := pluginManifest
	generation.Executors = []core.ExecutorIdentity{executor}
	generation.Executors[0].Generation = 1
	_, err = EncodeOperatorManifest(generation)
	require.Error(t, err)
	empty, err := EncodeOperatorManifest(OperatorManifest{Version: 1})
	require.NoError(t, err)
	closed, err := ParseOperatorManifest(empty)
	require.NoError(t, err)
	assert.Error(t, closed.Models.Check(t.Context(), candidate))
}

func TestPolicyPublicationBoundary(t *testing.T) {
	var nilPolicy *Policy
	require.False(t, nilPolicy.AllowsModel("kimi-k3"))
	require.False(t, nilPolicy.AllowsChannelType(1))
	p, err := New(nil, nil)
	require.NoError(t, err)
	require.False(t, p.AllowsModel("kimi-k3"))
	require.False(t, p.AllowsChannelType(1))

	p, err = New([]string{"kimi-k3, glm-5.3", "kimi-k3,step-5-preview"}, []string{"1,17", "17,61"})
	require.NoError(t, err)
	for _, name := range []string{"kimi-k3", "glm-5.3", "step-5-preview"} {
		require.True(t, p.AllowsModel(name), name)
	}
	for _, name := range []string{"Kimi-K3", "kimi-k3.1", " kimi-k3", "kimi-k3 ", "upstream/kimi-k3", "kіmi-k3", "kimi-ｋ3", "", "*"} {
		require.False(t, p.AllowsModel(name), name)
	}
	for _, id := range []int{1, 17, 61} {
		require.True(t, p.AllowsChannelType(id))
	}
	for _, id := range []int{0, -1, 59, 62} {
		require.False(t, p.AllowsChannelType(id))
	}

	input := []string{"glm-5.3", "Kimi-K3", "kimi-k3", "glm-5.3", "not-published"}
	got := p.FilterModels(input)
	require.Equal(t, []string{"glm-5.3", "kimi-k3"}, got)
	got[0] = "changed"
	require.Equal(t, "glm-5.3", input[0], "filter must not overwrite shared catalog storage")
}

func TestMalformedPolicyFailsClosed(t *testing.T) {
	cases := []struct {
		name          string
		models, types []string
	}{
		{"empty_model_entry", []string{"kimi-k3,"}, nil},
		{"newline_model", []string{"kimi\nk3"}, nil},
		{"trailing_newline_model", []string{"kimi-k3\n"}, nil},
		{"invalid_utf8_model", []string{string([]byte{0xff})}, nil},
		{"nul_model", []string{"kimi\x00k3"}, nil},
		{"oversized_model", []string{strings.Repeat("m", 256)}, nil},
		{"newline_type", nil, []string{"1\n"}},
		{"empty_type_entry", nil, []string{"1,,17"}},
		{"zero_type", nil, []string{"0"}},
		{"negative_type", nil, []string{"-1"}},
		{"signed_type", nil, []string{"+1"}},
		{"leading_zero", nil, []string{"01"}},
		{"scientific_notation", nil, []string{"1e2"}},
		{"invalid_type", nil, []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.models, tc.types)
			require.Error(t, err)
			require.Nil(t, p, "partially parsed configuration must not be installed")
			require.False(t, p.AllowsModel("kimi-k3"))
		})
	}
}

func TestLoadDoesNotFreezeOrMutateEnvironment(t *testing.T) {
	previousModels, previousTypes := BuildModelAllowlist, BuildChannelTypeAllowlist
	t.Cleanup(func() { BuildModelAllowlist, BuildChannelTypeAllowlist = previousModels, previousTypes })
	BuildModelAllowlist, BuildChannelTypeAllowlist = "kimi-k3", "1"
	t.Setenv("CMH_MODEL_ALLOWLIST", "glm-5.3")
	t.Setenv("CMH_CHANNEL_TYPE_ALLOWLIST", "17")
	first, err := Load()
	require.NoError(t, err)
	require.True(t, first.AllowsModel("kimi-k3"))
	require.True(t, first.AllowsModel("glm-5.3"))
	require.True(t, first.AllowsChannelType(17))
	t.Setenv("CMH_MODEL_ALLOWLIST", "step-5-preview")
	second, err := Load()
	require.NoError(t, err)
	require.False(t, second.AllowsModel("glm-5.3"))
	require.True(t, second.AllowsModel("step-5-preview"))
	require.True(t, first.AllowsModel("glm-5.3"), "existing immutable policy must remain unchanged")
	t.Setenv("CMH_MODEL_ALLOWLIST", "kimi-k3,,glm-5.3")
	broken, err := Load()
	require.Error(t, err)
	require.Nil(t, broken)
}

func admissionFixture(t *testing.T) (*Policy, *Policy, core.ExecutorIdentity, core.Admission) {
	t.Helper()
	public, err := New([]string{"public-chat"}, []string{"1,61"})
	require.NoError(t, err)
	upstream, err := New([]string{"mapped-chat,declared-chat"}, nil)
	require.NoError(t, err)
	executor := core.ExecutorIdentity{Kind: "plugin", Key: "cmh-test", SourceSHA256: strings.Repeat("a", 64), ConfigFingerprint: strings.Repeat("b", 64), Generation: 1}
	return public, upstream, executor, core.Admission{
		Phase:   core.AdmissionCandidate,
		Model:   core.ModelIdentity{Public: "public-chat", Declared: "declared-chat", Upstream: "mapped-chat"},
		Channel: core.ChannelIdentity{Known: true, ID: 7, Type: 61}, Executor: executor,
	}
}

func TestAdmissionGateCandidateAndVisibility(t *testing.T) {
	public, upstream, executor, candidate := admissionFixture(t)
	gate, err := NewGate(public, upstream, []core.ExecutorIdentity{executor})
	require.NoError(t, err)
	cases := []struct {
		name             string
		change           func(*core.Admission)
		allowed, visible bool
	}{
		{"approved candidate", func(*core.Admission) {}, true, true},
		{"attempt", func(a *core.Admission) { a.Phase = core.AdmissionAttempt }, true, true},
		{"dispatch", func(a *core.Admission) { a.Phase = core.AdmissionDispatch }, true, true},
		{"new channel save", func(a *core.Admission) { a.Phase = core.AdmissionChannelSave; a.Channel.ID = 0 }, true, false},
		{"negative channel save", func(a *core.Admission) { a.Phase = core.AdmissionChannelSave; a.Channel.ID = -1 }, false, false},
		{"public name alone", func(a *core.Admission) {
			*a = core.Admission{Phase: core.AdmissionPublic, Model: core.ModelIdentity{Public: "public-chat"}}
		}, true, false},
		{"unknown phase", func(a *core.Admission) { a.Phase = "future" }, false, false},
		{"denied alias to allowed upstream", func(a *core.Admission) { a.Model.Public = "other-chat" }, false, false},
		{"allowed alias to denied upstream", func(a *core.Admission) { a.Model.Upstream = "other-chat" }, false, false},
		{"upstream case exact", func(a *core.Admission) { a.Model.Upstream = "Mapped-chat" }, false, false},
		{"public case exact", func(a *core.Admission) { a.Model.Public = "Public-chat" }, false, false},
		{"public whitespace exact", func(a *core.Admission) { a.Model.Public += " " }, false, false},
		{"declared not implicitly public", func(a *core.Admission) { a.Model.Declared = "public-chat" }, false, false},
		{"undeclared native target", func(a *core.Admission) { a.Model.Declared = "" }, true, true},
		{"missing upstream", func(a *core.Admission) { a.Model.Upstream = "" }, false, false},
		{"unknown channel", func(a *core.Admission) { a.Channel.Known = false }, false, false},
		{"known zero channel", func(a *core.Admission) { a.Channel.ID = 0 }, false, false},
		{"negative channel", func(a *core.Admission) { a.Channel.ID = -7 }, false, false},
		{"known zero type", func(a *core.Admission) { a.Channel.Type = 0 }, false, false},
		{"unlisted type", func(a *core.Admission) { a.Channel.Type = 2 }, false, false},
		{"source changed", func(a *core.Admission) { a.Executor.SourceSHA256 = strings.Repeat("c", 64) }, false, false},
		{"config changed", func(a *core.Admission) { a.Executor.ConfigFingerprint = strings.Repeat("c", 64) }, false, false},
		{"executor changed", func(a *core.Admission) { a.Executor.Key = "cmh-other" }, false, false},
		{"executor key alone", func(a *core.Admission) { a.Executor.SourceSHA256 = "" }, false, false},
		{"generation is not approval", func(a *core.Admission) { a.Executor.Generation++ }, true, true},
		{"unknown executor kind", func(a *core.Admission) { a.Executor.Kind = "future" }, false, false},
		{"missing executor kind", func(a *core.Admission) { a.Executor.Kind = "" }, false, false},
		{"native builtin", func(a *core.Admission) { a.Executor = core.ExecutorIdentity{Kind: "builtin"} }, true, true},
		{"plugin disguised builtin", func(a *core.Admission) { a.Executor.Kind = "builtin" }, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := candidate
			tc.change(&input)
			before := input
			err := gate.Check(context.Background(), input)
			if tc.allowed {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrAdmissionDenied)
				assert.EqualError(t, err, "model admission denied")
			}
			assert.Equal(t, tc.visible, gate.Visible(context.Background(), input))
			assert.Equal(t, before, input, "checks must not mutate identity snapshots")
		})
	}
}

func TestAdmissionGateApprovalValidationAndCopy(t *testing.T) {
	public, upstream, executor, candidate := admissionFixture(t)
	cases := []struct {
		name   string
		change func(*core.ExecutorIdentity)
	}{
		{"future kind", func(e *core.ExecutorIdentity) { e.Kind = "future" }},
		{"empty kind", func(e *core.ExecutorIdentity) { e.Kind = "" }},
		{"empty key", func(e *core.ExecutorIdentity) { e.Key = "" }},
		{"wildcard key", func(e *core.ExecutorIdentity) { e.Key = "cmh-*" }},
		{"oversized key", func(e *core.ExecutorIdentity) { e.Key = strings.Repeat("a", 256) }},
		{"control key", func(e *core.ExecutorIdentity) { e.Key = "cmh-\ntest" }},
		{"path key", func(e *core.ExecutorIdentity) { e.Key = "../plugin" }},
		{"uppercase source", func(e *core.ExecutorIdentity) { e.SourceSHA256 = strings.Repeat("A", 64) }},
		{"short source", func(e *core.ExecutorIdentity) { e.SourceSHA256 = strings.Repeat("a", 63) }},
		{"nonhex source", func(e *core.ExecutorIdentity) { e.SourceSHA256 = strings.Repeat("g", 64) }},
		{"missing config", func(e *core.ExecutorIdentity) { e.ConfigFingerprint = "" }},
		{"uppercase config", func(e *core.ExecutorIdentity) { e.ConfigFingerprint = strings.Repeat("B", 64) }},
		{"oversized config", func(e *core.ExecutorIdentity) { e.ConfigFingerprint = strings.Repeat("a", 65) }},
		{"builtin with plugin tuple", func(e *core.ExecutorIdentity) { e.Kind = "builtin" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			invalid := executor
			tc.change(&invalid)
			gate, err := NewGate(public, upstream, []core.ExecutorIdentity{executor, invalid})
			assert.Nil(t, gate, "no partially validated gate may escape")
			assert.EqualError(t, err, "invalid model admission configuration")
		})
	}
	for _, policies := range [][2]*Policy{{nil, upstream}, {public, nil}, {nil, nil}} {
		gate, err := NewGate(policies[0], policies[1], nil)
		assert.Nil(t, gate)
		assert.EqualError(t, err, "invalid model admission configuration")
	}
	approvals := []core.ExecutorIdentity{executor}
	gate, err := NewGate(public, upstream, approvals)
	require.NoError(t, err)
	approvals[0].SourceSHA256 = strings.Repeat("c", 64)
	assert.NoError(t, gate.Check(context.Background(), candidate), "constructor must copy approval values")
	candidate.Executor = approvals[0]
	assert.ErrorIs(t, gate.Check(context.Background(), candidate), ErrAdmissionDenied)

	gate, err = NewGate(public, upstream, nil)
	require.NoError(t, err)
	candidate.Executor = executor
	assert.ErrorIs(t, gate.Check(context.Background(), candidate), ErrAdmissionDenied, "plugins are never implicitly trusted")
	candidate.Executor = core.ExecutorIdentity{Kind: "builtin"}
	assert.NoError(t, gate.Check(context.Background(), candidate))
	gate, err = NewGate(&Policy{}, &Policy{}, nil)
	require.NoError(t, err)
	assert.ErrorIs(t, gate.Check(context.Background(), candidate), ErrAdmissionDenied, "zero policy denies all")
}

func TestExecutorApprovalManifestAndConfig(t *testing.T) {
	before := core.Models
	source := strings.Repeat("a", 64)
	config, err := EffectiveConfigFingerprint([]byte(`{"temperature":0,"enabled":false}`))
	require.NoError(t, err)
	assert.Len(t, config, 64)
	line := "cmh-test\t" + source + "\t" + config
	approvals, err := ParseExecutorApprovals(line + "\r\n")
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	assert.Equal(t, core.ExecutorIdentity{Kind: "plugin", Key: "cmh-test", SourceSHA256: source, ConfigFingerprint: config}, approvals[0])
	assert.Equal(t, before, core.Models, "parsing operator approvals never installs or trusts a plugin")
	for _, raw := range []string{" ", "# comment", line + "\n" + line, strings.Replace(line, "cmh-test", "cmh-*", 1), strings.Replace(line, source, strings.ToUpper(source), 1), line + "\textra", "cmh-test\t" + source, strings.Repeat("x", 64*1024+1)} {
		got, err := ParseExecutorApprovals(raw)
		require.Error(t, err)
		assert.Nil(t, got, "no partial approvals on malformed input")
		assert.NotContains(t, err.Error(), source)
	}
	empty, err := ParseExecutorApprovals("")
	require.NoError(t, err)
	assert.Empty(t, empty)
	_, err = ParseExecutorApprovals(string([]byte{0xff}))
	require.Error(t, err)
	var tooMany strings.Builder
	for i := range 257 {
		fmt.Fprintf(&tooMany, "cmh-%d\t%s\t%s\n", i, source, config)
	}
	_, err = ParseExecutorApprovals(tooMany.String())
	require.Error(t, err)
	_, err = EffectiveConfigFingerprint([]byte{0xff})
	require.Error(t, err)
	withNull, err := EffectiveConfigFingerprint([]byte(`{"optional":null}`))
	require.NoError(t, err)
	assert.Len(t, withNull, 64)
	_, err = EffectiveConfigFingerprint([]byte(`{"a":0,"a":false}`))
	require.NoError(t, err, "host decoder accepts duplicate keys; exact original bytes are approved, this helper is not schema or unique-key validation")
	for _, raw := range []string{"", "null", "[]", "false", `{"bad":`, `{} {}`} {
		digest, err := EffectiveConfigFingerprint([]byte(raw))
		require.Error(t, err)
		assert.Empty(t, digest)
	}
	other, err := EffectiveConfigFingerprint([]byte(`{"temperature":0,"enabled":true}`))
	require.NoError(t, err)
	assert.NotEqual(t, config, other)
	formatted, err := EffectiveConfigFingerprint([]byte(`{ "temperature":0,"enabled":false}`))
	require.NoError(t, err)
	assert.NotEqual(t, config, formatted, "fingerprint is exact host-produced bytes, not inferred canonical JSON")
	public, upstream, executor, candidate := admissionFixture(t)
	executor.SourceSHA256, executor.ConfigFingerprint = source, config
	gate, err := NewGate(public, upstream, approvals)
	require.NoError(t, err)
	candidate.Executor = executor
	assert.NoError(t, gate.Check(context.Background(), candidate))
	assert.NoError(t, gate.CheckHook(context.Background(), core.PluginHookIdentity{SourceSHA256: source, Export: "run"}))
	assert.ErrorIs(t, gate.CheckHook(context.Background(), core.PluginHookIdentity{SourceSHA256: strings.Repeat("b", 64), Export: "run"}), ErrAdmissionDenied)
	assert.ErrorIs(t, gate.CheckHook(context.Background(), core.PluginHookIdentity{SourceSHA256: source}), ErrAdmissionDenied)
	candidate.Executor.ConfigFingerprint = other
	assert.ErrorIs(t, gate.Check(context.Background(), candidate), ErrAdmissionDenied)
	// Effective config is validated by the host before hashing. Preserve explicit
	// zero/false and unknown fields in that exact host value, never strip defaults.
	var object map[string]any
	require.NoError(t, common.Unmarshal([]byte(`{"temperature":0,"enabled":false}`), &object))
	assert.Equal(t, false, object["enabled"])
}

func TestDestinationPolicyExactRelations(t *testing.T) {
	address, err := core.DestinationAddress("https://EXAMPLE.invalid?deployment=approved")
	require.NoError(t, err)
	same, err := core.DestinationAddress("https://example.invalid/?deployment=approved")
	require.NoError(t, err)
	assert.Equal(t, address, same)
	for _, raw := range []string{"", "file:///etc/passwd", "https://user:secret@example.invalid/", "https://example.invalid/#secret", "https://example.invalid\\evil/", "https://example.invalid/ bad", "https:opaque", "http://[fe80::1%25ETH0]/x", "http://[fe80::1%25eth0]/x"} {
		_, err := core.DestinationAddress(raw)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), raw+"secret")
	}
	changed, err := core.DestinationAddress("https://example.invalid/?deployment=other")
	require.NoError(t, err)
	assert.NotEqual(t, address, changed, "deployment query is part of approval even when host unchanged")
	admission := core.DestinationAdmission{Kind: "relay_http", Method: "POST", AddressSHA256: address, HostSHA256: core.DestinationHost("example.invalid"), Model: core.ModelIdentity{Public: "public-chat", Upstream: "approved-target"}, Channel: core.ChannelIdentity{Known: true, ID: 1, Type: 1}}
	before := core.Destinations
	policy, err := NewDestinationPolicy([]core.DestinationAdmission{admission})
	require.NoError(t, err)
	assert.Equal(t, before, core.Destinations, "constructing approval relations cannot activate transport policy")
	assert.NoError(t, policy.CheckDestination(t.Context(), admission))
	for _, alter := range []func(*core.DestinationAdmission){func(a *core.DestinationAdmission) { a.AddressSHA256 = changed }, func(a *core.DestinationAdmission) { a.HostSHA256 = core.DestinationHost("else.invalid") }, func(a *core.DestinationAdmission) { a.Model.Upstream = "unapproved-target" }, func(a *core.DestinationAdmission) { a.Channel.ID = 2 }, func(a *core.DestinationAdmission) { a.Method = "GET" }} {
		next := admission
		alter(&next)
		assert.ErrorIs(t, policy.CheckDestination(t.Context(), next), ErrAdmissionDenied)
	}
	bad := admission
	bad.AddressSHA256 = "bad"
	got, err := NewDestinationPolicy([]core.DestinationAdmission{admission, bad})
	require.Error(t, err)
	assert.Nil(t, got)
	got, err = NewDestinationPolicy([]core.DestinationAdmission{admission, admission})
	require.Error(t, err)
	assert.Nil(t, got)
	empty, err := NewDestinationPolicy(nil)
	require.NoError(t, err)
	assert.ErrorIs(t, empty.CheckDestination(t.Context(), admission), ErrAdmissionDenied)
	_, _, executor, _ := admissionFixture(t)
	fetch := core.DestinationAdmission{Kind: "plugin_fetch", Method: "POST", AddressSHA256: address, HostSHA256: core.DestinationHost("example.invalid"), Executor: executor, Hook: "refresh"}
	fetchPolicy, err := NewDestinationPolicy([]core.DestinationAdmission{fetch})
	require.NoError(t, err)
	assert.NoError(t, fetchPolicy.CheckDestination(t.Context(), fetch))
	fake := fetch
	fake.Channel = admission.Channel
	_, err = NewDestinationPolicy([]core.DestinationAdmission{fake})
	require.Error(t, err)
	fake = fetch
	fake.Model = admission.Model
	_, err = NewDestinationPolicy([]core.DestinationAdmission{fake})
	require.Error(t, err)
	fake = fetch
	fake.Hook = "ingress"
	assert.ErrorIs(t, fetchPolicy.CheckDestination(t.Context(), fake), ErrAdmissionDenied)
	fake = admission
	fake.Executor = executor
	_, err = NewDestinationPolicy([]core.DestinationAdmission{fake})
	require.Error(t, err)
	content := admission
	content.Kind = "task_content"
	content.Method = "GET"
	contentPolicy, err := NewDestinationPolicy([]core.DestinationAdmission{content})
	require.NoError(t, err)
	assert.NoError(t, contentPolicy.CheckDestination(t.Context(), content))
	content.Method = "DELETE"
	_, err = NewDestinationPolicy([]core.DestinationAdmission{content})
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, policy.CheckDestination(ctx, admission), context.Canceled)
}

func TestAdmissionGateBatchCancellationAndNoop(t *testing.T) {
	public, upstream, executor, candidate := admissionFixture(t)
	original := core.Models
	gate, err := NewGate(public, upstream, []core.ExecutorIdentity{executor})
	require.NoError(t, err)
	assert.Equal(t, original, core.Models, "constructing a policy must not activate it")
	assert.NoError(t, core.Models.Check(context.Background(), core.Admission{}))
	assert.NoError(t, core.Models.ValidateChannels(context.Background(), []core.ChannelSpec{{Enabled: true}}))
	assert.True(t, core.Models.Visible(context.Background(), core.Admission{}), "default core boundary preserves host behavior")
	valid := core.ChannelSpec{Enabled: true, Channel: candidate.Channel, Models: []core.ModelIdentity{candidate.Model}, Executor: executor}
	invalid := valid
	invalid.Models = []core.ModelIdentity{{Public: "public-chat", Upstream: "denied-secret-model"}}
	disabled := core.ChannelSpec{Enabled: false}
	newChannel := valid
	newChannel.Channel.ID = 0
	negativeChannel := valid
	negativeChannel.Channel.ID = -1
	cases := []struct {
		name    string
		batch   []core.ChannelSpec
		allowed bool
	}{
		{"empty batch", nil, true},
		{"enabled valid", []core.ChannelSpec{valid}, true},
		{"enabled new channel", []core.ChannelSpec{newChannel}, true},
		{"negative channel ID", []core.ChannelSpec{negativeChannel}, false},
		{"disabled cleanup", []core.ChannelSpec{disabled}, true},
		{"enabled empty models", []core.ChannelSpec{{Enabled: true, Channel: candidate.Channel, Executor: executor}}, false},
		{"later channel invalid", []core.ChannelSpec{valid, invalid}, false},
		{"later model invalid", []core.ChannelSpec{{Enabled: true, Channel: candidate.Channel, Executor: executor, Models: []core.ModelIdentity{candidate.Model, invalid.Models[0]}}}, false},
		{"enabled unknown identity", []core.ChannelSpec{{Enabled: true, Models: valid.Models, Executor: executor}}, false},
		{"enabled unapproved executor", []core.ChannelSpec{{Enabled: true, Channel: candidate.Channel, Models: valid.Models, Executor: core.ExecutorIdentity{Kind: "plugin", Key: "cmh-test"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := make([]core.ChannelSpec, len(tc.batch))
			for i, channel := range tc.batch {
				before[i] = channel
				before[i].Models = append([]core.ModelIdentity(nil), channel.Models...)
			}
			writes := 0
			err := gate.ValidateChannels(context.Background(), tc.batch)
			if err == nil {
				writes = len(tc.batch) // host writes only after whole-batch validation
			}
			if tc.allowed {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.batch), writes)
			} else {
				assert.EqualError(t, err, "model admission denied")
				assert.Zero(t, writes, "later invalid entries veto the whole batch")
			}
			if tc.batch != nil {
				assert.Equal(t, before, tc.batch, "prevalidation never rewrites caller-owned state")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, gate.Check(ctx, candidate), context.Canceled)
	assert.ErrorIs(t, gate.Check(ctx, core.Admission{}), context.Canceled, "cancellation precedes policy denial")
	assert.ErrorIs(t, gate.ValidateChannels(ctx, []core.ChannelSpec{disabled}), context.Canceled)
	assert.ErrorIs(t, gate.ValidateChannels(ctx, nil), context.Canceled)
	assert.False(t, gate.Visible(ctx, candidate))
}

func TestPolicyOnlyStartup(t *testing.T) {
	oldModels, oldHooks, oldExecutors, oldTargets := core.Models, core.PluginHooks, core.Executors, core.Destinations
	oldStore, oldGrants := core.TaskContinuations, core.ContinuationGrants
	oldBuild := BuildOperatorManifestSHA256
	BuildOperatorManifestSHA256 = ""
	t.Cleanup(func() {
		core.Models, core.PluginHooks, core.Executors, core.Destinations = oldModels, oldHooks, oldExecutors, oldTargets
		core.TaskContinuations, core.ContinuationGrants = oldStore, oldGrants
		BuildOperatorManifestSHA256 = oldBuild
	})
	require.NoError(t, InstallPolicyOnly(nil))
	assert.Equal(t, []any{oldModels, oldHooks, oldExecutors, oldTargets, oldStore, oldGrants}, []any{core.Models, core.PluginHooks, core.Executors, core.Destinations, core.TaskContinuations, core.ContinuationGrants})
	assert.Equal(t, oldModels, core.Models)
	require.EqualError(t, InstallPolicyOnly(map[string]string{"CMH_OPERATOR_MANIFEST_FILE": "missing"}), "extension policy startup rejected")
	assert.Equal(t, []any{oldModels, oldHooks, oldExecutors, oldTargets, oldStore, oldGrants}, []any{core.Models, core.PluginHooks, core.Executors, core.Destinations, core.TaskContinuations, core.ContinuationGrants})
	assert.Equal(t, oldModels, core.Models)
	wire, err := EncodeOperatorManifest(OperatorManifest{Version: 1})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "policy.json")
	require.NoError(t, os.WriteFile(path, wire, 0600))
	env := map[string]string{"CMH_OPERATOR_MANIFEST_FILE": path, "CMH_OPERATOR_MANIFEST_SHA256": fmt.Sprintf("%x", sha256.Sum256(wire))}
	require.NoError(t, InstallPolicyOnly(env))
	assert.True(t, core.ModelAdmissionEnabled())
	assert.Same(t, core.Models, core.PluginHooks)
	assert.Same(t, core.Models, core.Executors)
	assert.True(t, core.DestinationAdmissionEnabled())
	assert.ErrorIs(t, core.Models.Check(t.Context(), core.Admission{Phase: core.AdmissionPublic, Model: core.ModelIdentity{Public: "public-chat"}}), ErrAdmissionDenied)
	assert.ErrorIs(t, core.PluginHooks.CheckHook(t.Context(), core.PluginHookIdentity{SourceSHA256: strings.Repeat("a", 64), Export: "run"}), ErrAdmissionDenied)
	assert.ErrorIs(t, core.Executors.CheckExecutor(t.Context(), core.ExecutorIdentity{Kind: "plugin", Key: "fixture", SourceSHA256: strings.Repeat("a", 64), ConfigFingerprint: strings.Repeat("b", 64)}), ErrAdmissionDenied)
	assert.ErrorIs(t, core.Destinations.CheckDestination(t.Context(), core.DestinationAdmission{}), ErrAdmissionDenied)
	assert.Nil(t, core.TaskContinuations)
	assert.Nil(t, core.ContinuationGrants)
	request := core.ContinuationGrantRequest{Binding: core.ContinuationBinding{TaskID: "task", UserID: 1, ChannelID: 1}, Ref: core.ContinuationRef{ID: strings.Repeat("a", 64), Digest: strings.Repeat("b", 64)}, Purpose: core.ContinuationPoll, PluginKey: "fixture", SourceSHA256: strings.Repeat("c", 64), RuntimeSHA256: strings.Repeat("d", 64)}
	grant := core.ContinuationGrant{Request: request, Hooks: []string{"buildQueryRequest", "parseTaskResult"}, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	wire, err = EncodeOperatorManifest(OperatorManifest{Version: 1, Continuations: []core.ContinuationGrant{grant}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, wire, 0600))
	env["CMH_OPERATOR_MANIFEST_SHA256"] = fmt.Sprintf("%x", sha256.Sum256(wire))
	installed := core.Models
	installedSet := []any{core.Models, core.PluginHooks, core.Executors, core.Destinations, core.TaskContinuations, core.ContinuationGrants}
	require.EqualError(t, InstallPolicyOnly(env), "extension policy startup rejected")
	assert.Equal(t, installedSet, []any{core.Models, core.PluginHooks, core.Executors, core.Destinations, core.TaskContinuations, core.ContinuationGrants})
	assert.Same(t, installed, core.Models)
	assert.Nil(t, core.ContinuationGrants)
}

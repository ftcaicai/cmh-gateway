package domestic

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/ext/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

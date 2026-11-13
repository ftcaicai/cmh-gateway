package domestic

import (
	"strings"
	"testing"

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

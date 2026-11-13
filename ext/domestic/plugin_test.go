package domestic

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamNormalizePluginHostContract(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "plugins", "tasks", "cmh-param-normalize", "plugin.js"))
	require.NoError(t, err)
	fixtureData, err := os.ReadFile(filepath.Join("..", "..", "plugins", "tasks", "cmh-param-normalize", "golden.json"))
	require.NoError(t, err)
	plugin, err := jsplugin.CompilePlugin(string(source), jsplugin.Options{})
	require.NoError(t, err)
	assert.True(t, plugin.Meta.HasTransformPart())
	assert.False(t, plugin.Meta.HasTaskPart())
	assert.False(t, plugin.Meta.HasProviderPart())
	assert.Equal(t, []string{"outbound"}, plugin.TransformHooks())

	t.Run("portable golden replay", func(t *testing.T) {
		var fixture jsplugin.Fixture
		require.NoError(t, common.Unmarshal(fixtureData, &fixture))
		report, err := jsplugin.ReplayFixture(context.Background(), string(source), fixtureData)
		require.NoError(t, err)
		assert.Equal(t, len(fixture.Cases), report.Total)
		assert.Equal(t, report.Total, report.Passed)

		// Use one loaded runtime sequentially, including after rejections. ReplayFixture
		// exercises the JS exports; CallTransform additionally validates host result shape.
		for _, tc := range fixture.Cases {
			if tc.Hook != "outbound" {
				continue
			}
			t.Run(tc.Name, func(t *testing.T) {
				var wireContext map[string]any
				require.NoError(t, common.Unmarshal(tc.Args[0], &wireContext))
				if channel, ok := wireContext["channel"].(map[string]any); ok {
					if _, stringID := channel["id"].(string); stringID {
						return // Replay covers malformed wire IDs; Go's channel ID is always int.
					}
				}
				var ctx jsplugin.TransformContext
				var req jsplugin.TransformRequest
				require.NoError(t, common.Unmarshal(tc.Args[0], &ctx))
				require.NoError(t, common.Unmarshal(tc.Args[1], &req))
				if ctx.Stage != "outbound" {
					return // The host never invokes this export under another stage.
				}
				before, err := common.Marshal(req)
				require.NoError(t, err)
				result, err := plugin.CallTransform(context.Background(), ctx, req)
				require.NoError(t, err)
				var expected struct {
					Action  string          `json:"action"`
					Code    string          `json:"code"`
					Effects json.RawMessage `json:"effects"`
				}
				require.NoError(t, common.Unmarshal(tc.Expected, &expected))
				assert.Equal(t, expected.Action, result.Action)
				assert.Equal(t, expected.Code, result.Code)
				if len(expected.Effects) == 0 {
					assert.Empty(t, result.Effects)
				} else {
					assert.JSONEq(t, string(expected.Effects), string(result.Effects))
					effects, err := requestpatch.DecodeTransformEffects(result.Effects, requestpatch.EffectJSONPatch)
					require.NoError(t, err, "the real host must accept the aggregate operation count")
					_, _, err = requestpatch.Apply(req.Body, nil, effects, nil)
					require.NoError(t, err)
				}
				after, err := common.Marshal(req)
				require.NoError(t, err)
				assert.Equal(t, string(before), string(after), "hook arguments remain read-only")
			})
		}
	})

	t.Run("defaults and malformed targeting", func(t *testing.T) {
		defaults, err := plugin.ValidateConfig(nil)
		require.NoError(t, err)
		assert.JSONEq(t, `{"channelIds":"","expectedModel":"","expectedUpstreamModel":"","removeTemperature":false,"removeTopP":false,"convertDeveloperToSystem":false}`, string(defaults))
		for _, config := range []string{
			`null`, `[]`, `{"channelIds":[7]}`, `{"channelIds":"01"}`, `{"channelIds":"7,7"}`,
			`{"channelIds":"7, 9"}`, `{"channelIds":"7"}`, `{"removeTemperature":"false"}`,
			`{"channelIds":"7","expectedModel":"public-chat","expectedUpstreamModel":""}`,
			`{"channelIds":"7","expectedModel":"public-*","expectedUpstreamModel":"mapped-chat"}`,
			`{"channelIds":"7","expectedModel":" public-chat","expectedUpstreamModel":"mapped-chat"}`,
			`{"unknown":true}`, `{"expectedModel":null}`,
		} {
			_, err := plugin.ValidateConfig(json.RawMessage(config))
			require.Error(t, err, config)
		}
	})

	t.Run("real patch preserves media zero false headers and billing inputs", func(t *testing.T) {
		config, err := plugin.ValidateConfig(json.RawMessage(`{"channelIds":"7","expectedModel":"public-chat","expectedUpstreamModel":"mapped-chat","convertDeveloperToSystem":true}`))
		require.NoError(t, err)
		body := json.RawMessage(`{"model":"mapped-chat","temperature":0,"top_p":0,"max_tokens":0,"max_completion_tokens":0,"n":1,"stream":false,"stream_options":{"include_usage":false},"usage":{"untouched":true},"billing":{"untouched":0},"messages":[{"role":"developer","name":"keep","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}],"extra":{"zero":0,"enabled":false}},{"role":"user","content":"unchanged"}]}`)
		// The real host sends masked media to JS, then applies the leaf patch to
		// the original body. No media placeholder is ever copied into an effect.
		masked := json.RawMessage(`{"model":"mapped-chat","temperature":0,"top_p":0,"messages":[{"role":"developer","content":[{"type":"image_url","image_url":{"url":"[media:0]"}}]}]}`)
		ctx := jsplugin.TransformContext{Stage: "outbound", Format: "openai", Path: "/v1/chat/completions", Model: "public-chat", UpstreamModel: "mapped-chat", Channel: &jsplugin.TransformChannel{ID: 7, Type: 999}, Config: config}
		result, err := plugin.CallTransform(context.Background(), ctx, jsplugin.TransformRequest{Body: masked})
		require.NoError(t, err)
		require.Equal(t, "continue", result.Action)
		assert.JSONEq(t, `[{"type":"json.patch","ops":[{"op":"replace","path":"/messages/0/role","value":"system"}]}]`, string(result.Effects))
		repeated, err := plugin.CallTransform(context.Background(), ctx, jsplugin.TransformRequest{Body: masked})
		require.NoError(t, err)
		assert.Equal(t, result, repeated, "the same input must not retain normalization state")
		effects, err := requestpatch.DecodeTransformEffects(result.Effects, requestpatch.EffectJSONPatch)
		require.NoError(t, err)
		headers := http.Header{"X-Test": {"keep"}}
		patched, patchedHeaders, err := requestpatch.Apply(body, headers, effects, nil)
		require.NoError(t, err)
		var want map[string]any
		require.NoError(t, common.Unmarshal(body, &want))
		want["messages"].([]any)[0].(map[string]any)["role"] = "system"
		wantJSON, err := common.Marshal(want)
		require.NoError(t, err)
		assert.JSONEq(t, string(wantJSON), string(patched))
		assert.Equal(t, headers, patchedHeaders)
		assert.Contains(t, string(body), `"role":"developer"`, "the original request must not be mutated")
	})
}

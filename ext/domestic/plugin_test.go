package domestic

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginSourceIdentity(t *testing.T) {
	const source = `export const meta = {apiVersion: 1, key: "cmh-source-identity", name: "Source identity", version: "1.0.0", author: {name: "Test"}};
export function ingress() { return {action: "continue"}; }`
	sources := []string{
		source,
		source + "\nexport const revision = 2;",
		source + "\n// source identity comment",
		" \n" + source + "\n\t",
	}
	compiled := make([]*jsplugin.LoadedPlugin, len(sources))
	for i, raw := range sources {
		plugin, err := jsplugin.CompilePlugin(raw, jsplugin.Options{})
		require.NoError(t, err)
		compiled[i] = plugin
		assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(raw))), plugin.SourceSHA256())
		assert.Equal(t, plugin.SourceSHA256(), plugin.Engine.SourceSHA256())
		assert.Equal(t, "cmh-source-identity", plugin.Meta.Key)
		assert.Equal(t, "1.0.0", plugin.Meta.Version)
		for _, previous := range compiled[:i] {
			assert.NotEqual(t, previous.SourceSHA256(), plugin.SourceSHA256(), "code, comments and whitespace all contribute to source identity")
		}
	}

	registry := jsplugin.NewRegistry()
	factory, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, compiled[0].SourceSHA256(), factory.SourceSHA256())
	factoryGeneration := registry.Generation()

	override, err := registry.Register(sources[1], jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, compiled[1].SourceSHA256(), override.SourceSHA256())
	overrideGeneration := registry.Generation()
	require.NoError(t, registry.ReplaceOverrides([]*jsplugin.LoadedPlugin{compiled[2]}))
	current, ok := registry.Get(factory.Meta.Key)
	require.True(t, ok)
	assert.Same(t, compiled[2], current)
	assert.Equal(t, compiled[2].SourceSHA256(), current.SourceSHA256())

	retainedFactory, ok := factoryGeneration.Get(factory.Meta.Key)
	require.True(t, ok)
	assert.Same(t, factory, retainedFactory)
	assert.Equal(t, compiled[0].SourceSHA256(), retainedFactory.SourceSHA256())
	retainedOverride, ok := overrideGeneration.Get(override.Meta.Key)
	require.True(t, ok)
	assert.Same(t, override, retainedOverride)
	assert.Equal(t, compiled[1].SourceSHA256(), retainedOverride.SourceSHA256())
}

type sourceHookProbe struct {
	allow bool
	seen  []core.PluginHookIdentity
}

func (p *sourceHookProbe) CheckHook(_ context.Context, identity core.PluginHookIdentity) error {
	recorded := identity
	recorded.Members = append([]string(nil), identity.Members...)
	p.seen = append(p.seen, recorded)
	// A policy must not be able to rewrite the caller's member path.
	if len(identity.Members) > 0 {
		identity.Members[0] = "policy-mutated"
	}
	if !p.allow {
		return errors.New("private policy error must not escape")
	}
	return nil
}

func TestPluginSourceHookArtifactVeto(t *testing.T) {
	previous := core.PluginHooks
	t.Cleanup(func() { core.PluginHooks = previous })
	probe := &sourceHookProbe{}
	core.PluginHooks = probe
	require.True(t, core.PluginHookAdmissionEnabled())
	const source = `let calls = 0;
export const meta = {name: "inspectable"};
function invoked() { calls++; return {calls, payload: {ok: true}}; }
export function run() { return invoked(); }
export const native = {go: invoked, deep: {go: invoked}};`
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	type result struct {
		Calls int `json:"calls"`
	}
	cases := []struct {
		name, export string
		members      []string
		invoke       func(*jsplugin.Engine, []string) (int, error)
	}{
		{"Call", "run", nil, func(e *jsplugin.Engine, _ []string) (int, error) {
			var out result
			value, err := e.Call(t.Context(), "run")
			if err == nil {
				raw, merr := common.Marshal(value)
				require.NoError(t, merr)
				require.NoError(t, common.Unmarshal(raw, &out))
			}
			return out.Calls, err
		}},
		{"CallInto", "run", nil, func(e *jsplugin.Engine, _ []string) (int, error) {
			var out result
			err := e.CallInto(t.Context(), &out, "run")
			return out.Calls, err
		}},
		{"CallJSONInto", "run", nil, func(e *jsplugin.Engine, _ []string) (int, error) {
			var out result
			err := e.CallJSONInto(t.Context(), &out, "run")
			return out.Calls, err
		}},
		{"CallMember", "native", []string{"go"}, func(e *jsplugin.Engine, _ []string) (int, error) {
			value, err := e.CallMember(t.Context(), "native", "go")
			var out result
			if err == nil {
				raw, merr := common.Marshal(value)
				require.NoError(t, merr)
				require.NoError(t, common.Unmarshal(raw, &out))
			}
			return out.Calls, err
		}},
		{"CallPath", "native", []string{"deep", "go"}, func(e *jsplugin.Engine, path []string) (int, error) {
			value, err := e.CallPath(t.Context(), "native", path)
			var out result
			if err == nil {
				raw, merr := common.Marshal(value)
				require.NoError(t, merr)
				require.NoError(t, common.Unmarshal(raw, &out))
			}
			return out.Calls, err
		}},
		{"CallPathInto", "native", []string{"deep", "go"}, func(e *jsplugin.Engine, path []string) (int, error) {
			var out result
			err := e.CallPathInto(t.Context(), &out, "native", path)
			return out.Calls, err
		}},
		{"CallPathWithAdmissionTimeout", "native", []string{"deep", "go"}, func(e *jsplugin.Engine, path []string) (int, error) {
			value, err := e.CallPathWithAdmissionTimeout(t.Context(), time.Second, "native", path)
			var out result
			if err == nil {
				raw, merr := common.Marshal(value)
				require.NoError(t, merr)
				require.NoError(t, common.Unmarshal(raw, &out))
			}
			return out.Calls, err
		}},
		{"CallPathWithMemberJSON", "native", []string{"deep", "go"}, func(e *jsplugin.Engine, path []string) (int, error) {
			value, text, err := e.CallPathWithMemberJSON(t.Context(), time.Second, "payload", "native", path)
			var out result
			if err == nil {
				assert.JSONEq(t, `{"ok":true}`, string(text))
				raw, merr := common.Marshal(value)
				require.NoError(t, merr)
				require.NoError(t, common.Unmarshal(raw, &out))
			}
			return out.Calls, err
		}},
		{"CallPathWithMembersJSON", "native", []string{"deep", "go"}, func(e *jsplugin.Engine, path []string) (int, error) {
			value, text, err := e.CallPathWithMembersJSON(t.Context(), time.Second, []string{"payload"}, "native", path)
			var out result
			if err == nil {
				assert.JSONEq(t, `{"ok":true}`, string(text))
				raw, merr := common.Marshal(value)
				require.NoError(t, merr)
				require.NoError(t, common.Unmarshal(raw, &out))
			}
			return out.Calls, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe.allow, probe.seen = false, nil
			engine, err := jsplugin.Compile(source, jsplugin.Options{Key: "caller-label-is-not-identity", Concurrency: 1})
			require.NoError(t, err, "source inspection must not require runtime hook approval")
			meta, err := engine.Export(t.Context(), "meta")
			require.NoError(t, err)
			assert.NotNil(t, meta)
			assert.Empty(t, probe.seen, "declarative exports are not runtime hook calls")
			path := append([]string(nil), tc.members...)
			_, err = tc.invoke(engine, path)
			require.EqualError(t, err, "plugin execution rejected")
			require.Len(t, probe.seen, 1)
			assert.Equal(t, core.PluginHookIdentity{SourceSHA256: digest, Export: tc.export, Members: tc.members}, probe.seen[0])
			assert.Equal(t, tc.members, path)
			probe.allow = true
			calls, err := tc.invoke(engine, path)
			require.NoError(t, err)
			assert.Equal(t, 1, calls, "vetoed call must never execute hook or modify module state")
			assert.Equal(t, tc.members, path)
		})
	}
	engine, err := jsplugin.Compile(source+"\n// changed bytes", jsplugin.Options{Key: "same-options-key", Concurrency: 1})
	require.NoError(t, err)
	probe.allow = false
	probe.seen = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = engine.Call(ctx, "run")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, probe.seen, "cancellation precedes installed policy")
	_, err = engine.Call(t.Context(), "run")
	require.EqualError(t, err, "plugin execution rejected")
	require.Len(t, probe.seen, 1)
	assert.NotEqual(t, digest, probe.seen[0].SourceSHA256)
	core.PluginHooks = previous
	assert.False(t, core.PluginHookAdmissionEnabled(), "no source policy remains installed after this fixture")
	_, err = engine.Call(t.Context(), "run")
	require.NoError(t, err)
}

func TestPluginSourceHookVetoBeforeCapacity(t *testing.T) {
	previous := core.PluginHooks
	t.Cleanup(func() { core.PluginHooks = previous })
	entered, release := make(chan struct{}), make(chan struct{})
	engine, err := jsplugin.Compile(`export function hold() { utils.unixNow(); return 1; }`, jsplugin.Options{Concurrency: 1, Now: func() time.Time { close(entered); <-release; return time.Unix(1, 0) }})
	require.NoError(t, err)
	finished := make(chan error, 1)
	go func() { _, callErr := engine.Call(t.Context(), "hold"); finished <- callErr }()
	<-entered
	probe := &sourceHookProbe{}
	core.PluginHooks = probe
	_, err = engine.CallPathWithAdmissionTimeout(t.Context(), time.Millisecond, "hold", nil)
	assert.EqualError(t, err, "plugin execution rejected", "source veto must precede occupied JS capacity")
	close(release)
	require.NoError(t, <-finished)
	require.Len(t, probe.seen, 1)
}

func TestBoundPluginProjection(t *testing.T) {
	previousGate, previousRegistry := core.Models, jsplugin.DefaultRegistry
	t.Cleanup(func() { core.Models, jsplugin.DefaultRegistry = previousGate, previousRegistry })
	const source = `export const meta = {apiVersion:1,key:"cmh-bound",name:"Bound provider",version:"1.0.0",author:{name:"Test"},relay:[{incoming_path:"/v1/chat/completions",upstream_path:"/generate",converter:"none"}],channelConfig:{type:"object",properties:{tier:{type:"string",title:{en:"Tier",zh:"档位"},default:"standard"},enabled:{type:"boolean",title:{en:"Enabled",zh:"启用"},default:false}}}};`
	plugin, err := jsplugin.CompilePlugin(source, jsplugin.Options{})
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	registered, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	binding, mapping := `{"task_plugin_key":"cmh-bound"}`, `{"public-chat":"approved-target"}`
	base := "https://fixture.invalid"
	channel := &model.Channel{Id: 95001, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "public-chat", Group: "default", BaseURL: &base, Setting: &binding, ModelMapping: &mapping, OtherSettings: `{"plugin_config":{"tier":"standard","enabled":false}}`}
	identity, err := model.PluginExecutorIdentity(channel, plugin, registry.Generation().Number, "provider", "/v1/chat/completions")
	require.NoError(t, err)
	assert.Equal(t, plugin.SourceSHA256(), identity.SourceSHA256)
	assert.Len(t, identity.ConfigFingerprint, 64)
	public, err := New([]string{"public-chat"}, []string{"61"})
	require.NoError(t, err)
	upstream, err := New([]string{"approved-target"}, nil)
	require.NoError(t, err)
	gate, err := NewGate(public, upstream, []core.ExecutorIdentity{identity})
	require.NoError(t, err)
	core.Models = gate
	pinned := jsplugin.PinnedPlugin{Plugin: registered, Generation: registry.Generation()}
	admission, err := model.ChannelPluginAdmission(channel, "public-chat", core.AdmissionAttempt, pinned, "provider", "/v1/chat/completions")
	require.NoError(t, err)
	assert.NoError(t, gate.Check(t.Context(), admission))
	for _, change := range []func(*model.Channel){
		func(ch *model.Channel) { ch.OtherSettings = `{"plugin_config":{"tier":"premium","enabled":false}}` },
		func(ch *model.Channel) { v := "https://changed.invalid"; ch.BaseURL = &v },
		func(ch *model.Channel) { v := `{"public-chat":"changed-target"}`; ch.ModelMapping = &v },
	} {
		changed := *channel
		change(&changed)
		next, err := model.PluginExecutorIdentity(&changed, plugin, registry.Generation().Number, "provider", "/v1/chat/completions")
		require.NoError(t, err)
		assert.NotEqual(t, identity.ConfigFingerprint, next.ConfigFingerprint)
		candidate := admission
		candidate.Executor = next
		assert.ErrorIs(t, gate.Check(t.Context(), candidate), ErrAdmissionDenied)
	}
	roleIdentity, err := model.PluginExecutorIdentity(channel, plugin, registry.Generation().Number, "task", "/v1/tasks/cmh-bound")
	if err == nil {
		assert.NotEqual(t, identity.ConfigFingerprint, roleIdentity.ConfigFingerprint)
	}
	bad := *channel
	bad.OtherSettings = `{"plugin_config":null}`
	_, err = model.PluginExecutorIdentity(&bad, plugin, registry.Generation().Number, "provider", "/v1/chat/completions")
	require.Error(t, err)
	altered, err := jsplugin.Compile(source+"\nexport const changed=true;", jsplugin.Options{})
	require.NoError(t, err)
	wrapperCopy := *plugin
	wrapperCopy.Engine = altered
	_, err = model.PluginExecutorIdentity(channel, &wrapperCopy, registry.Generation().Number, "provider", "/v1/chat/completions")
	require.Error(t, err, "wrapper source digest must bind the actual Engine")
	wrongBinding := *channel
	wrong := `{"task_plugin_key":"some-other-plugin"}`
	wrongBinding.Setting = &wrong
	_, err = model.ChannelPluginAdmission(&wrongBinding, "public-chat", core.AdmissionAttempt, pinned, "provider", "/v1/chat/completions")
	require.Error(t, err)
	filters := []hostdto.ChannelFilter{{Kind: hostdto.FilterTaskPluginIdentity, RequestPath: "/v1/chat/completions"}}
	assert.Equal(t, identity, model.ChannelModelAdmission(channel, "public-chat", core.AdmissionCandidate, filters...).Executor)
	_, err = registry.Register(source+"\nexport const updated=1;", jsplugin.Options{})
	require.NoError(t, err)
	assert.ErrorIs(t, gate.Check(t.Context(), model.ChannelModelAdmission(channel, "public-chat", core.AdmissionCandidate, filters...)), ErrAdmissionDenied, "candidate has current generation only; old pinned selection cannot be promised through candidate filters")
	retained, err := model.ChannelPluginAdmission(channel, "public-chat", core.AdmissionAttempt, pinned, "provider", "/v1/chat/completions")
	require.NoError(t, err)
	assert.NoError(t, gate.Check(t.Context(), retained), "already selected exact approved pinned artifact remains independently checkable")
}

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

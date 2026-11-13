package domestic

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/requestpatch"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	taskjsplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/request_plugin_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
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
		assert.Equal(t, []byte(raw), plugin.SourceBytes())
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

func TestRequestStageExecutorPreflight(t *testing.T) {
	previousRegistry, previousExecutors := jsplugin.DefaultRegistry, core.Executors
	previousInstances, err := common.Marshal(request_plugin_setting.GetInstances())
	require.NoError(t, err)
	t.Cleanup(func() {
		jsplugin.DefaultRegistry = previousRegistry
		core.Executors = previousExecutors
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(previousInstances)}))
	})
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	service.InitHttpClient()
	const source = `export const meta={apiVersion:1,key:"cmh-stage",name:"Stage",version:"1.0.0",author:{name:"Test"},optional:true,configSchema:{type:"object",properties:{url:{type:"string",title:{en:"Service URL",zh:"服务地址"},format:"url"},value:{type:"string",title:{en:"Value",zh:"值"}}}}};
export function ingress(ctx,req){utils.fetch({url:ctx.config.url});return {action:"continue"};}`
	_, err = registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	instances := []request_plugin_setting.Instance{
		{ID: "first", Name: "First", Plugin: "cmh-stage", Enabled: true, Global: true, Config: common.RawMessage(fmt.Sprintf(`{"url":%q,"value":"one"}`, server.URL))},
		{ID: "second", Name: "Second", Plugin: "cmh-stage", Enabled: true, Global: true, Config: common.RawMessage(fmt.Sprintf(`{"url":%q,"value":"two"}`, server.URL))},
	}
	encoded, err := common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(encoded)}))
	call := requestplugin.Call{Stage: requestplugin.StageIngress, Path: "/v1/chat/completions", Format: "openai", Model: "public-chat", Body: []byte(`{"model":"public-chat","messages":[]}`)}
	probe := &executorCapture{}
	core.Executors = probe
	out := requestplugin.NewPlan().Run(t.Context(), call)
	require.Nil(t, out.Failed)
	assert.Equal(t, int32(2), fetches.Load())
	require.Len(t, probe.seen, 2)
	assert.NotEqual(t, probe.seen[0].ConfigFingerprint, probe.seen[1].ConfigFingerprint, "instance/config scopes require different exact approvals")
	public, upstream, _, _ := admissionFixture(t)
	firstOnly, err := NewGate(public, upstream, probe.seen[:1])
	require.NoError(t, err)
	core.Executors = firstOnly
	fetches.Store(0)
	deniedPlan := requestplugin.NewPlan()
	out = deniedPlan.Run(t.Context(), call)
	require.NotNil(t, out.Failed, "optional=true cannot swallow host admission veto")
	late := call
	late.Late = true
	lateOutcome := deniedPlan.Run(t.Context(), late)
	require.NotNil(t, lateOutcome.Failed, "denied preflight must not mark instances already executed")
	assert.Zero(t, fetches.Load(), "preflight later denied instance before earlier approved hook can fetch")
	assert.Equal(t, call.Body, out.Body, "no stage effects may commit before full scoped preflight")
	require.NoError(t, i18n.Init())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, call.Path, strings.NewReader(string(call.Body)))
	c.Request.Header.Set("Content-Type", "application/json")
	_, _, apiErr := requestplugin.Ingress(c, kittypes.RelayFormatOpenAI)
	require.NotNil(t, apiErr)
	assert.False(t, service.ShouldRetryRelayError(c, apiErr, 2), "public caller must preserve nonoptional host veto")
	assert.Zero(t, fetches.Load())
	all, err := NewGate(public, upstream, probe.seen)
	require.NoError(t, err)
	core.Executors = all
	out = requestplugin.NewPlan().Run(t.Context(), call)
	require.Nil(t, out.Failed)
	assert.Equal(t, int32(2), fetches.Load())
	changed := call
	changed.Path = "/v1/responses"
	fetches.Store(0)
	out = requestplugin.NewPlan().Run(t.Context(), changed)
	require.NotNil(t, out.Failed)
	assert.Zero(t, fetches.Load(), "approval must bind actual path/stage, not plugin name alone")
	instances[1].Config = common.RawMessage(fmt.Sprintf(`{"url":%q,"value":"changed"}`, server.URL))
	encoded, err = common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(encoded)}))
	out = requestplugin.NewPlan().Run(t.Context(), call)
	require.NotNil(t, out.Failed)
	assert.Zero(t, fetches.Load(), "changing a raw instance config must invalidate stage approval")
	unchecked := call
	unchecked.Body = nil
	out = requestplugin.NewPlan().Run(t.Context(), unchecked)
	require.Nil(t, out.Failed)
	assert.Zero(t, fetches.Load(), "nil stage body is unchecked, not an executed hook")
}

type executorCapture struct{ seen []core.ExecutorIdentity }

func (p *executorCapture) CheckExecutor(_ context.Context, executor core.ExecutorIdentity) error {
	p.seen = append(p.seen, executor)
	return nil
}

func TestReplyExecutorPreflight(t *testing.T) {
	previousRegistry, previousExecutors := jsplugin.DefaultRegistry, core.Executors
	previousInstances, err := common.Marshal(request_plugin_setting.GetInstances())
	require.NoError(t, err)
	t.Cleanup(func() {
		jsplugin.DefaultRegistry = previousRegistry
		core.Executors = previousExecutors
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(previousInstances)}))
	})
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	var fetches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	service.InitHttpClient()
	require.NoError(t, i18n.Init())
	const source = `export const meta={apiVersion:1,key:"cmh-reply-gate",name:"Reply gate",version:"1.0.0",author:{name:"Test"},optional:true,configSchema:{type:"object",properties:{url:{type:"string",title:{en:"Service URL",zh:"服务地址"},format:"url"}}}};
export function reply(ctx,outputs){utils.fetch({url:ctx.config.url});return {action:"continue"};}`
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	instances := []request_plugin_setting.Instance{{ID: "reply-first", Name: "First", Plugin: plugin.Meta.Key, Enabled: true, Global: true, Config: common.RawMessage(fmt.Sprintf(`{"url":%q}`, upstream.URL))}, {ID: "reply-second", Name: "Second", Plugin: plugin.Meta.Key, Enabled: true, Global: true, Config: common.RawMessage(fmt.Sprintf(`{"url":%q}`, upstream.URL))}}
	encoded, err := common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(encoded)}))
	instances = request_plugin_setting.GetInstances()
	var approvals []core.ExecutorIdentity
	for i := range instances {
		identity, err := requestplugin.RequestExecutorIdentity(plugin, &instances[i], "request_reply", "/v1/chat/completions", "reply")
		require.NoError(t, err)
		approvals = append(approvals, identity)
	}
	public, upstreamPolicy, _, _ := admissionFixture(t)
	firstOnly, err := NewGate(public, upstreamPolicy, approvals[:1])
	require.NoError(t, err)
	all, err := NewGate(public, upstreamPolicy, approvals)
	require.NoError(t, err)
	answer := []byte(`{"id":"local","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"local answer"},"finish_reason":"stop"}]}`)
	for _, scenario := range []struct {
		name       string
		gate       core.ExecutorGate
		wantFetch  int32
		wantStatus int
	}{{"denied optional second", firstOnly, 0, 500}, {"allowed", all, 2, 200}} {
		t.Run(scenario.name, func(t *testing.T) {
			core.Executors = scenario.gate
			fetches.Store(0)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-chat","messages":[]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "public-chat")
			_, _, apiErr := requestplugin.Ingress(c, kittypes.RelayFormatOpenAI)
			require.Nil(t, apiErr)
			info := &relaycommon.RelayInfo{OriginModelName: "public-chat", RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: kittypes.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1, ChannelType: 1, UpstreamModelName: "approved-target"}}
			finish := requestplugin.Reply(c, kittypes.RelayFormatOpenAI, info)
			c.Writer.Header().Set("Content-Type", "application/json")
			c.Writer.WriteHeader(http.StatusOK)
			_, err := c.Writer.Write(answer)
			require.NoError(t, err)
			finish()
			assert.Equal(t, scenario.wantStatus, w.Code)
			assert.Equal(t, scenario.wantFetch, fetches.Load())
			if scenario.wantStatus == 500 {
				assert.NotContains(t, w.Body.String(), "local answer", "unapproved optional target must not release a successful answer")
			} else {
				assert.JSONEq(t, string(answer), w.Body.String())
			}
		})
	}
	t.Run("stream revocation between batches", func(t *testing.T) {
		core.Executors = all
		fetches.Store(0)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-chat","messages":[],"stream":true}`))
		c.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "public-chat")
		_, _, apiErr := requestplugin.Ingress(c, kittypes.RelayFormatOpenAI)
		require.Nil(t, apiErr)
		info := &relaycommon.RelayInfo{OriginModelName: "public-chat", IsStream: true, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: kittypes.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1, ChannelType: 1, UpstreamModelName: "approved-target"}}
		finish := requestplugin.Reply(c, kittypes.RelayFormatOpenAI, info)
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		_, err := c.Writer.Write([]byte("data: {\"id\":\"local\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"first\"}}]}\n\n"))
		require.NoError(t, err)
		require.Equal(t, int32(2), fetches.Load())
		core.Executors = firstOnly
		_, _ = c.Writer.Write([]byte("data: {\"id\":\"local\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"revoked text must never be released in this stream batch\"}}]}\n\n"))
		finish()
		assert.Equal(t, int32(2), fetches.Load(), "revoked second batch must stop before either hook fetches")
		assert.Contains(t, w.Body.String(), "first")
		assert.NotContains(t, w.Body.String(), "revoked text must never")
		assert.Contains(t, w.Body.String(), "request_plugin_failed")
	})
}

func TestRelayDestinationBeforeNetwork(t *testing.T) {
	old := core.Destinations
	t.Cleanup(func() { core.Destinations = old })
	var received, redirected atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer other.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, other.URL, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	service.InitHttpClient()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelId, 7)
	common.SetContextKey(c, constant.ContextKeyChannelType, 1)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "public-chat")
	info := &relaycommon.RelayInfo{OriginModelName: "public-chat", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7, ChannelType: 1, UpstreamModelName: "approved-target"}}
	address, err := core.DestinationAddress(upstream.URL + "/approved?deployment=one")
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, upstream.URL+"/approved?deployment=one", nil)
	require.NoError(t, err)
	approval := core.DestinationAdmission{Kind: "relay_http", Method: http.MethodPost, AddressSHA256: address, HostSHA256: core.DestinationHost(request.Host), Model: core.ModelIdentity{Public: "public-chat", Upstream: "approved-target"}, Channel: core.ChannelIdentity{Known: true, ID: 7, Type: 1}}
	policy, err := NewDestinationPolicy([]core.DestinationAdmission{approval})
	require.NoError(t, err)
	core.Destinations = policy
	response, err := relaychannel.DoRequest(c, request, info)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(1), received.Load())
	for _, change := range []struct{ name, url, host string }{{"same-host unapproved deployment", upstream.URL + "/approved?deployment=two", ""}, {"different destination", other.URL + "/approved", ""}, {"custom Host", upstream.URL + "/approved?deployment=one", "unapproved.invalid"}} {
		t.Run(change.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, change.url, nil)
			require.NoError(t, err)
			if change.host != "" {
				req.Host = change.host
			}
			_, err = relaychannel.DoRequest(c, req, info)
			require.Error(t, err)
			typed := kittypes.NewOpenAIError(err, kittypes.ErrorCodeDoRequestFailed, 500)
			assert.Equal(t, 403, typed.StatusCode)
			assert.False(t, service.ShouldRetryRelayError(c, typed, 2))
			assert.Equal(t, int32(1), received.Load())
			assert.Zero(t, redirected.Load())
		})
	}
	redirectURL := upstream.URL + "/redirect"
	redirectHash, err := core.DestinationAddress(redirectURL)
	require.NoError(t, err)
	redirectApproval := approval
	redirectApproval.AddressSHA256 = redirectHash
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{redirectApproval})
	require.NoError(t, err)
	core.Destinations = policy
	request, err = http.NewRequest(http.MethodPost, redirectURL, nil)
	require.NoError(t, err)
	response, err = relaychannel.DoRequest(c, request, info)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, response.StatusCode)
	require.NoError(t, response.Body.Close())
	assert.Zero(t, redirected.Load(), "official relay must keep 3xx rather than follow to unapproved target")
	core.Destinations = old
	assert.NoError(t, relaychannel.CheckDestination(nil, nil, "", "", "", ""), "default must not add nil/context dependencies")
}

func TestPluginFetchDestinationBeforeNetwork(t *testing.T) {
	previous := core.Destinations
	t.Cleanup(func() { core.Destinations = previous })
	var calls, redirects atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirects.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer second.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, second.URL, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	service.InitHttpClient()
	const source = `export const meta={apiVersion:1,key:"cmh-fetch-target",name:"Fetch target",version:"1.0.0",author:{name:"Test"},relay:[{incoming_path:"/v1/chat/completions",upstream_path:"/relay"}],configSchema:{type:"object",properties:{url:{type:"string",title:{en:"Service URL",zh:"服务地址"},format:"url"},marker:{type:"string",title:{en:"Marker",zh:"标记"}}}}};
export function ingress(ctx,req){utils.fetch({url:ctx.config.url});return {action:"continue"};}
export function refresh(ctx){utils.fetch({url:ctx.baseUrl+"/credential",method:"POST"});return {credential:{fixture:true},expiresAt:utils.unixNow()+3600};}`
	plugin, err := jsplugin.CompilePlugin(source, jsplugin.Options{})
	require.NoError(t, err)
	configuration := common.RawMessage(fmt.Sprintf(`{"url":%q,"marker":"first"}`, server.URL+"/service?deployment=one"))
	fetchExecutor, err := service.RequestPluginFetchExecutor(plugin, configuration)
	require.NoError(t, err)
	urlHash, err := core.DestinationAddress(server.URL + "/service?deployment=one")
	require.NoError(t, err)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	approval := core.DestinationAdmission{Kind: "plugin_fetch", Method: "GET", AddressSHA256: urlHash, HostSHA256: core.DestinationHost(parsed.Host), Executor: fetchExecutor, Hook: "ingress"}
	invoke := func(configValue common.RawMessage) error {
		ctx := jsplugin.WithFetcher(t.Context(), service.RequestPluginFetcher(plugin, configValue))
		_, err := plugin.CallTransform(ctx, jsplugin.TransformContext{Stage: "ingress", Config: configValue}, jsplugin.TransformRequest{Body: []byte(`{"model":"local"}`)})
		return err
	}
	deny, err := NewDestinationPolicy(nil)
	require.NoError(t, err)
	core.Destinations = deny
	require.Error(t, invoke(configuration))
	assert.Zero(t, calls.Load())
	policy, err := NewDestinationPolicy([]core.DestinationAdmission{approval})
	require.NoError(t, err)
	core.Destinations = policy
	require.NoError(t, invoke(configuration))
	assert.Equal(t, int32(1), calls.Load())
	_, directErr := service.RequestPluginFetcher(plugin, configuration)(t.Context(), jsplugin.FetchRequest{URL: server.URL + "/service?deployment=one", Method: "GET", Hook: "report"})
	require.Error(t, directErr)
	assert.Equal(t, int32(1), calls.Load(), "ingress target approval must not authorize report")
	replacement, err := jsplugin.CompilePlugin(source+"\nexport const replacement=true;", jsplugin.Options{})
	require.NoError(t, err)
	_, directErr = service.RequestPluginFetcher(replacement, configuration)(t.Context(), jsplugin.FetchRequest{URL: server.URL + "/service?deployment=one", Method: "GET", Hook: "ingress"})
	require.Error(t, directErr)
	assert.Equal(t, int32(1), calls.Load(), "same key/version cannot authorize replacement source")
	stream := &deniedFetchBody{}
	_, directErr = service.RequestPluginFetcher(plugin, configuration)(t.Context(), jsplugin.FetchRequest{URL: server.URL + "/service?deployment=one", Method: "POST", Hook: "ingress", BodyStream: stream, BodySize: 1})
	require.Error(t, directErr)
	assert.Zero(t, stream.reads, "denied fetch body cannot be read before destination approval")
	changedConfig := common.RawMessage(fmt.Sprintf(`{"url":%q,"marker":"second"}`, server.URL+"/service?deployment=one"))
	require.Error(t, invoke(changedConfig))
	assert.Equal(t, int32(1), calls.Load(), "same URL cannot authorize different original instance capability")
	credentialExecutor, err := service.PluginFetchExecutor(plugin, server.URL, "", kitdto.ChannelSettings{})
	require.NoError(t, err)
	credentialURL, err := core.DestinationAddress(server.URL + "/credential")
	require.NoError(t, err)
	credentialApproval := core.DestinationAdmission{Kind: "plugin_fetch", Method: "POST", AddressSHA256: credentialURL, HostSHA256: core.DestinationHost(parsed.Host), Executor: credentialExecutor, Hook: "refresh"}
	ctx := service.PluginCredentialHookContext(t.Context(), plugin, server.URL, "")
	_, err = plugin.CallRefresh(ctx, jsplugin.CredentialContext{BaseURL: server.URL, Credential: common.RawMessage(`{"fixture":true}`)})
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "request-service approval cannot authorize credential fetch")
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{credentialApproval})
	require.NoError(t, err)
	core.Destinations = policy
	_, err = plugin.CallRefresh(ctx, jsplugin.CredentialContext{BaseURL: server.URL, Credential: common.RawMessage(`{"fixture":true}`)})
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	redirectConfig := common.RawMessage(fmt.Sprintf(`{"url":%q}`, server.URL+"/redirect"))
	redirectExecutor, err := service.RequestPluginFetchExecutor(plugin, redirectConfig)
	require.NoError(t, err)
	redirectHash, err := core.DestinationAddress(server.URL + "/redirect")
	require.NoError(t, err)
	redirectApproval := approval
	redirectApproval.AddressSHA256 = redirectHash
	redirectApproval.Executor = redirectExecutor
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{redirectApproval})
	require.NoError(t, err)
	core.Destinations = policy
	require.NoError(t, invoke(redirectConfig))
	assert.Zero(t, redirects.Load(), "utils.fetch must retain redirect response instead of following to a new target")
}

type deniedFetchBody struct{ reads int }

func (p *deniedFetchBody) Read([]byte) (int, error) { p.reads++; return 0, io.EOF }

func TestTaskQueryDestinationBeforeNetwork(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	previousFetch := *fetchSetting
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = []string{"1-65535"}
	t.Cleanup(func() { *fetchSetting = previousFetch })
	previous := core.Destinations
	t.Cleanup(func() { core.Destinations = previous })
	var firstCalls, redirectCalls atomic.Int32
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer next.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls.Add(1)
		if r.URL.Path == "/redirect" {
			status := http.StatusFound
			if r.Method == http.MethodPost {
				status = http.StatusSeeOther
			}
			http.Redirect(w, r, next.URL+"/result", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	service.InitHttpClient()
	const source = `export const meta={apiVersion:1,key:"cmh-query-target",name:"Query target",version:"1.0.0",author:{name:"Test"},models:["local-task"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit"};}
export function parseSubmitResponse(){return {taskId:"local"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+(ctx.state&&ctx.state.redirect?"/redirect":"/query"),method:ctx.state&&ctx.state.post?"POST":"GET",body:ctx.state&&ctx.state.post?{fixture:true}:undefined};}
export function buildBatchQueryRequest(ctx,tasks){return {url:ctx.baseUrl+"/batch",method:"POST",body:{ids:tasks.map(t=>t.taskId)}};}
export function parseTaskResult(){return {status:"SUCCESS"};}
export function parseBatchResult(){return [];}`
	plugin, err := jsplugin.CompilePlugin(source, jsplugin.Options{})
	require.NoError(t, err)
	capability, err := service.PluginFetchExecutor(plugin, server.URL, "", kitdto.ChannelSettings{})
	require.NoError(t, err)
	makeApproval := func(raw, method, hook string) core.DestinationAdmission {
		hash, err := core.DestinationAddress(raw)
		require.NoError(t, err)
		host, err := url.Parse(raw)
		require.NoError(t, err)
		return core.DestinationAdmission{Kind: "task_query", Method: method, AddressSHA256: hash, HostSHA256: core.DestinationHost(host.Host), Executor: capability, Hook: hook}
	}
	task := &model.Task{TaskID: "task-local", Properties: model.Properties{OriginModelName: "not-currently-published"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "up-local"}}
	adaptor := taskjsplugin.New(plugin)
	deny, err := NewDestinationPolicy(nil)
	require.NoError(t, err)
	core.Destinations = deny
	_, err = adaptor.FetchTask(server.URL, "fixture", task, "")
	require.Error(t, err)
	assert.Zero(t, firstCalls.Load())
	single := makeApproval(server.URL+"/query", http.MethodGet, "buildQueryRequest")
	policy, err := NewDestinationPolicy([]core.DestinationAdmission{single})
	require.NoError(t, err)
	core.Destinations = policy
	response, err := adaptor.FetchTask(server.URL, "fixture", task, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(1), firstCalls.Load(), "existing task query does not need newly published model approval")
	_, err = adaptor.FetchBatchTasks(server.URL, "fixture", []*model.Task{task}, "")
	require.Error(t, err)
	assert.Equal(t, int32(1), firstCalls.Load(), "single hook cannot authorize batch URL/method")
	batch := makeApproval(server.URL+"/batch", http.MethodPost, "buildBatchQueryRequest")
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{batch})
	require.NoError(t, err)
	core.Destinations = policy
	response, err = adaptor.FetchBatchTasks(server.URL, "fixture", []*model.Task{task}, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(2), firstCalls.Load())
	task.PrivateData.PluginState = common.RawMessage(`{"redirect":true}`)
	redirect := makeApproval(server.URL+"/redirect", http.MethodGet, "buildQueryRequest")
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{redirect})
	require.NoError(t, err)
	core.Destinations = policy
	_, err = adaptor.FetchTask(server.URL, "fixture", task, "")
	require.Error(t, err)
	assert.Zero(t, redirectCalls.Load(), "redirect target must be approved before second network hop")
	secondTarget := makeApproval(next.URL+"/result", http.MethodGet, "buildQueryRequest")
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{redirect, secondTarget})
	require.NoError(t, err)
	core.Destinations = policy
	response, err = adaptor.FetchTask(server.URL, "fixture", task, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(1), redirectCalls.Load())
	task.PrivateData.PluginState = common.RawMessage(`{"redirect":true,"post":true}`)
	postStart := makeApproval(server.URL+"/redirect", http.MethodPost, "buildQueryRequest")
	wrongMethod := makeApproval(next.URL+"/result", http.MethodPost, "buildQueryRequest")
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{postStart, wrongMethod})
	require.NoError(t, err)
	core.Destinations = policy
	_, err = adaptor.FetchTask(server.URL, "fixture", task, "")
	require.Error(t, err)
	assert.Equal(t, int32(1), redirectCalls.Load(), "POST approval cannot authorize GET rewritten by303")
	policy, err = NewDestinationPolicy([]core.DestinationAdmission{postStart, secondTarget})
	require.NoError(t, err)
	core.Destinations = policy
	response, err = adaptor.FetchTask(server.URL, "fixture", task, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(2), redirectCalls.Load())
}

func TestFrozenTaskRuntimeRestoration(t *testing.T) {
	previousRegistry := jsplugin.DefaultRegistry
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	t.Cleanup(func() { jsplugin.DefaultRegistry = previousRegistry })
	var calls atomic.Int32
	var tier, revision, override string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		override = r.Header.Get("X-Frozen-Override")
		tier = r.Header.Get("X-Frozen-Tier")
		revision = r.Header.Get("X-Revision")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"IN_PROGRESS"}`))
	}))
	defer server.Close()
	service.InitHttpClient()
	const sourceA = `export const meta={apiVersion:1,key:"cmh-frozen-runtime",name:"Frozen runtime",version:"1.0.0",author:{name:"Test"},models:["local-task"],fetchMode:"per_task",channelConfig:{type:"object",properties:{tier:{type:"string",title:{en:"Tier",zh:"档位"},default:"A"}}}};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit",body:{fixture:true}};}
export function parseSubmitResponse(){return {taskId:"local"};}
export function buildQueryRequest(ctx){if(ctx.config.tier!=="A")throw new Error("changed runtime");return {url:ctx.baseUrl+"/query",headers:{"X-Revision":"A"}};}
export function signRequest(ctx,request){if(ctx.config.tier!=="A")throw new Error("changed signer config");return {headers:{"X-Frozen-Tier":ctx.config.tier}};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}`
	plugin, err := registry.RegisterFactory(sourceA, jsplugin.Options{})
	require.NoError(t, err)
	copied := plugin.SourceBytes()
	require.Equal(t, []byte(sourceA), copied)
	copied[0] ^= 0xff
	assert.Equal(t, []byte(sourceA), plugin.SourceBytes())
	info := &relaycommon.RelayInfo{OriginModelName: "local-task", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 17, ChannelType: constant.ChannelTypeTaskPlugin, ChannelBaseUrl: server.URL, ApiKey: "not-to-be-archived", UpstreamModelName: "local-task", HeadersOverride: map[string]any{"X-Frozen-Override": "A"}, ChannelSetting: kitdto.ChannelSettings{TaskPluginKey: plugin.Meta.Key}, ChannelOtherSettings: kitdto.ChannelOtherSettings{PluginConfig: common.RawMessage(`{"tier":"A"}`)}}}
	original := taskjsplugin.New(plugin)
	original.Init(info)
	artifact, err := original.CaptureContinuationArtifact(info)
	require.NoError(t, err)
	assert.Equal(t, []byte(sourceA), artifact.Source)
	assert.NotContains(t, string(artifact.RuntimeJSON), "not-to-be-archived")
	info.ChannelOtherSettings.PluginConfig = common.RawMessage(`{"tier":"B"}`)
	info.ChannelBaseUrl = "https://changed.invalid"
	info.HeadersOverride["X-Frozen-Override"] = "B"
	sourceB := strings.ReplaceAll(sourceA, `"X-Revision":"A"`, `"X-Revision":"B"`)
	replacement, err := registry.RegisterFactory(sourceB, jsplugin.Options{})
	require.NoError(t, err)
	restored, err := taskjsplugin.RestoreContinuationArtifact(artifact)
	require.NoError(t, err)
	current, ok := registry.Get(plugin.Meta.Key)
	require.True(t, ok)
	assert.Same(t, replacement, current, "restoring old source must never publish it")
	restored.Init(info)
	task := &model.Task{TaskID: "persisted-task", ChannelId: 17, Properties: model.Properties{OriginModelName: "local-task"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "local-upstream"}}
	response, err := restored.FetchTask(server.URL, "fixture-static-key", task, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, "A", tier)
	assert.Equal(t, "A", revision)
	assert.Equal(t, "A", override)
	_, err = restored.FetchTask(server.URL, "fixture-static-key", task, "http://proxy-substitution.invalid")
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
	_, err = restored.FetchTask("https://changed.invalid", "fixture-static-key", task, "")
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"local-task"}`))
	require.NotNil(t, restored.ValidateRequestAndSetAction(c, info))
	_, err = restored.BuildRequestBody(c, info)
	require.Error(t, err)
	_, err = restored.DoRequest(c, info, strings.NewReader(`{}`))
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "frozen continuation cannot submit new work")
	previousDestinations := core.Destinations
	denyTargets, err := NewDestinationPolicy(nil)
	require.NoError(t, err)
	core.Destinations = denyTargets
	_, err = restored.FetchTask(server.URL, "fixture-static-key", task, "")
	require.Error(t, err)
	core.Destinations = previousDestinations
	assert.Equal(t, int32(1), calls.Load(), "restore cannot bypass active target checks")
	broken := artifact
	broken.Source = []byte(sourceB)
	_, err = taskjsplugin.RestoreContinuationArtifact(broken)
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	broken = artifact
	broken.RuntimeJSON = []byte(strings.Replace(string(artifact.RuntimeJSON), `"Version":1`, `"Version":2`, 1))
	_, err = taskjsplugin.RestoreContinuationArtifact(broken)
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	broken = artifact
	broken.PluginKey = "other-plugin"
	_, err = taskjsplugin.RestoreContinuationArtifact(broken)
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
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

package domestic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
	"github.com/QuantumNous/new-api/controller"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/ext/continuation"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/providerplugin"
	taskjsplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/request_plugin_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type admissionBillingProbe struct{ reserve, settle, refund int }

func (p *admissionBillingProbe) Settle(int) error              { p.settle++; return nil }
func (p *admissionBillingProbe) Refund(*gin.Context)           { p.refund++ }
func (p *admissionBillingProbe) NeedsRefund() bool             { return false }
func (p *admissionBillingProbe) GetPreConsumedQuota() int      { return 0 }
func (p *admissionBillingProbe) Reserve(int) error             { p.reserve++; return nil }
func (p *admissionBillingProbe) ReserveBeforeSubmit(int) error { p.reserve++; return nil }

func TestHostAdmissionMappingContract(t *testing.T) {
	cases := []struct {
		name         string
		mapping      map[string]string
		public, want string
		cycle        bool
	}{
		{"unmapped", nil, "public", "public", false},
		{"chain", map[string]string{"public": "step", "step": "target"}, "public", "target", false},
		{"self", map[string]string{"public": "public"}, "public", "public", false},
		{"chain self", map[string]string{"public": "step", "step": "step"}, "public", "step", false},
		{"cycle", map[string]string{"public": "step", "step": "public"}, "public", "", true},
		{"modifier fallback", map[string]string{"public": "target"}, "public@effort:max", "target", false},
		{"empty target", map[string]string{"public": ""}, "public", "public", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := model.ResolveChannelModel(tc.public, tc.mapping)
			if tc.cycle {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	for _, tc := range []struct {
		name, raw string
		self      bool
	}{
		{"self returns before request mutation", `{"public":"public"}`, true},
		{"chain self updates request", `{"public":"step","step":"step"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_mapping", tc.raw)
			info := &relaycommon.RelayInfo{OriginModelName: "public", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "public"}}
			req := &kitdto.GeneralOpenAIRequest{Model: "original-request"}
			require.NoError(t, helper.ModelMappedHelper(c, info, req))
			if tc.self {
				assert.False(t, info.IsModelMapped)
				assert.Equal(t, "original-request", req.Model)
			} else {
				assert.True(t, info.IsModelMapped)
				assert.Equal(t, "step", req.Model)
			}
		})
	}
	for _, tc := range []struct {
		name, raw, origin, upstream, requestModel string
		initialMapped, wantMapped                 bool
		wantError                                 bool
	}{
		{"stale mapped flag with no effective mapping", `{"other":"target"}`, "public", "public", "public", true, true, false},
		{"base self clears stale flag without rewriting request", `{"public":"public@effort:max"}`, "public@effort:max", "before", "original-request", true, false, false},
		{"traversed cycle preserves mapped flag", `{"public":"step","step":"public"}`, "public", "before", "original-request", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_mapping", tc.raw)
			info := &relaycommon.RelayInfo{OriginModelName: tc.origin, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.upstream, IsModelMapped: tc.initialMapped}}
			req := &kitdto.GeneralOpenAIRequest{Model: "original-request"}
			err := helper.ModelMappedHelper(c, info, req)
			if tc.wantError {
				require.EqualError(t, err, "model_mapping_contains_cycle")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantMapped, info.IsModelMapped)
			assert.Equal(t, tc.requestModel, req.Model)
		})
	}
}

func runHostAdmissionRequestBoundary(t *testing.T, channels []model.Channel, gate *Gate, noop core.ModelGate) {
	t.Helper()
	newContext := func(id int, name string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+name+`","messages":[]}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("original_model", name)
		common.SetContextKey(c, constant.ContextKeyChannelId, id)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
		common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channels[1].GetModelMapping())
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		return c
	}
	allowedInfo := &relaycommon.RelayInfo{OriginModelName: "public-chat", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channels[1].Id, ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "stale-not-authoritative"}}
	allowedContext := newContext(channels[1].Id, "public-chat")
	validSetup := newContext(channels[1].Id, "public-chat")
	require.Nil(t, middleware.SetupContextForSelectedChannel(validSetup, &channels[1], "public-chat"))
	assert.Equal(t, channels[1].Key, common.GetContextKeyString(validSetup, constant.ContextKeyChannelKey))
	require.Nil(t, relay.CheckRequestAdmission(allowedContext, allowedInfo))
	assert.Equal(t, "stale-not-authoritative", allowedInfo.UpstreamModelName, "identity check must not mutate relay state")
	staleMappingContext := newContext(channels[1].Id, "public-chat")
	common.SetContextKey(staleMappingContext, constant.ContextKeyChannelModelMapping, channels[0].GetModelMapping())
	require.NotNil(t, relay.CheckRequestAdmission(staleMappingContext, allowedInfo), "fresh DB repair cannot authorize an older denied context mapping")
	req := &kitdto.GeneralOpenAIRequest{Model: "original-request-must-not-change"}
	probe := &admissionBillingProbe{}
	deniedInfo := &relaycommon.RelayInfo{OriginModelName: "public-chat", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channels[0].Id, ChannelType: constant.ChannelTypeOpenAI}, Request: req, Billing: probe}
	apiErr := relay.PrepareRequestBilling(newContext(channels[0].Id, "public-chat"), deniedInfo)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Zero(t, probe.reserve)
	assert.Zero(t, probe.settle)
	assert.Zero(t, probe.refund)
	assert.Equal(t, "original-request-must-not-change", req.Model)
	assert.Zero(t, deniedInfo.FinalPreConsumedQuota)
	assert.Zero(t, deniedInfo.GetEstimatePromptTokens(), "denied request must stop before token estimation")
	assert.NotContains(t, apiErr.Error(), "unapproved-target")
	// The ordinary initial path has nil ChannelMeta until the handler. Its
	// admission-only snapshot still rejects before estimation or free-model skip.
	initial := &relaycommon.RelayInfo{OriginModelName: "public-chat", Request: req, Billing: probe}
	require.NotNil(t, relay.PrepareRequestBilling(newContext(channels[0].Id, "public-chat"), initial))
	assert.Nil(t, initial.ChannelMeta)
	assert.Zero(t, probe.reserve)

	// Removing a model from an otherwise enabled cached selection must revoke
	// this attempt even when a mapping entry for that old name still exists.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channels[1].Id).Update("models", "gpt-4o-mini").Error)
	require.NotNil(t, middleware.SetupContextForSelectedChannel(newContext(channels[1].Id, "public-chat"), &channels[1], "public-chat"))
	require.NotNil(t, relay.CheckRequestAdmission(allowedContext, allowedInfo))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channels[1].Id).Update("models", channels[1].Models).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channels[1].Id).Update("group", "other-group").Error)
	require.NotNil(t, middleware.SetupContextForSelectedChannel(newContext(channels[1].Id, "public-chat"), &channels[1], "public-chat"))
	require.NotNil(t, relay.CheckRequestAdmission(allowedContext, allowedInfo))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channels[1].Id).Update("group", "default").Error)
	// A cached chosen channel may become disabled before credential selection.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channels[1].Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
	setupContext := newContext(channels[1].Id, "public-chat")
	setupErr := middleware.SetupContextForSelectedChannel(setupContext, &channels[1], "public-chat")
	require.NotNil(t, setupErr)
	_, credentialPublished := common.GetContextKey(setupContext, constant.ContextKeyChannelKey)
	assert.False(t, credentialPublished, "rejected setup cannot publish a channel credential")
	require.NotNil(t, relay.CheckRequestAdmission(allowedContext, allowedInfo))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channels[1].Id).Update("status", common.ChannelStatusEnabled).Error)

	distributor := gin.New()
	reached := 0
	distributor.POST("/v1/chat/completions", middleware.Distribute(), func(c *gin.Context) { reached++; c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"not-published","messages":[]}`))
	r.Header.Set("Content-Type", "application/json")
	distributor.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Zero(t, reached, "public denial stops before channel selection/downstream dispatch")
	assert.NotContains(t, w.Body.String(), "fixture")
	readTask := model.Task{ID: 94001, TaskID: "task_cmh_admission_read", UserId: 1, Properties: model.Properties{OriginModelName: "not-published"}}
	require.NoError(t, model.DB.Create(&readTask).Error)
	defer func() { require.NoError(t, model.DB.Where("id = ?", readTask.ID).Delete(&model.Task{}).Error) }()
	for _, limited := range []bool{false, true} {
		reads := gin.New()
		reads.GET("/v1/videos/:task_id", func(c *gin.Context) {
			c.Set("id", 1)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, limited)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"not-published": true})
			c.Next()
		}, middleware.Distribute(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		rw := httptest.NewRecorder()
		reads.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/v1/videos/task_cmh_admission_read", nil))
		assert.Equal(t, http.StatusNoContent, rw.Code, "existing task read is not a new submission; model-limited=%v", limited)
	}
	core.Models = noop
	assert.Nil(t, relay.CheckRequestAdmission(nil, nil), "default boundary must not create new context or DB dependencies")
	core.Models = gate
}

func runHostBoundProviderAdmission(t *testing.T, previousGate *Gate, noop core.ModelGate) {
	t.Helper()
	previousRegistry := jsplugin.DefaultRegistry
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	defer func() { jsplugin.DefaultRegistry, core.Models = previousRegistry, previousGate }()
	var exchanges, dispatches, refreshes atomic.Int32
	var failExchange atomic.Bool
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/refresh" {
			refreshes.Add(1)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		if r.URL.Path == "/token" {
			exchanges.Add(1)
			if failExchange.Load() {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"token":"fixture-token"}`))
			return
		}
		dispatches.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstreamServer.Close()
	service.InitHttpClient()
	const source = `export const meta={apiVersion:1,key:"cmh-host-bound",name:"Host bound",version:"1.0.0",author:{name:"Test"},relay:[{incoming_path:"/v1/chat/completions",upstream_path:"/generate",converter:"none"}],channelConfig:{type:"object",properties:{tier:{type:"string",title:{en:"Tier",zh:"档位"},default:"standard"}}}};
export function exchangeKey(ctx) { const r=utils.fetch({url:ctx.baseUrl+"/token",method:"POST",body:JSON.stringify({tier:ctx.config.tier})}); if(r.status!==200) throw new Error("exchange failed"); return {token:r.body.token}; }
export function refresh(ctx) { utils.fetch({url:ctx.baseUrl+"/refresh",method:"POST"}); return {credential:{fixture:true},expiresAt:utils.unixNow()+3600}; }`
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	binding, mapping := `{"task_plugin_key":"cmh-host-bound"}`, `{"public-chat":"approved-target"}`
	base := upstreamServer.URL
	channel := model.Channel{Id: 95002, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Key: "fixture", Models: "public-chat", Group: "default", BaseURL: &base, Setting: &binding, ModelMapping: &mapping, OtherSettings: `{"plugin_config":{"tier":"standard"}}`}
	core.Models = noop
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(model.DB))
	defer func() {
		core.Models = noop
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("id = ?", channel.Id).Delete(&model.Channel{}).Error)
	}()
	identity, err := model.PluginExecutorIdentity(&channel, plugin, registry.Generation().Number, "provider", "/v1/chat/completions")
	require.NoError(t, err)
	public, err := New([]string{"public-chat"}, []string{"61"})
	require.NoError(t, err)
	upstream, err := New([]string{"approved-target"}, nil)
	require.NoError(t, err)
	gate, err := NewGate(public, upstream, []core.ExecutorIdentity{identity})
	require.NoError(t, err)
	core.Models = gate
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	service.GetChannelConstraints(c).AddFilter(hostdto.ChannelFilter{Kind: hostdto.FilterRequestPath, RequestPath: c.Request.URL.Path})
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "public-chat"))
	info := &relaycommon.RelayInfo{OriginModelName: "public-chat", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelType: channel.Type}}
	require.Nil(t, relay.CheckRequestAdmission(c, info))
	selectedParams, _ := common.GetContextKey(c, constant.ContextKeyChannelParamOverride)
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"temperature": 0})
	require.NotNil(t, relay.CheckRequestAdmission(c, info), "runtime-added affinity overrides currently require a separately supported execution envelope")
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, selectedParams)
	c.Set("request_plugin_plan", struct{}{})
	require.NotNil(t, relay.CheckRequestAdmission(c, info), "retained plan remains unknown even after current request-plugin settings are empty")
	delete(c.Keys, "request_plugin_plan")
	actual := &relaycommon.RelayInfo{UsingGroup: "default", OriginModelName: "public-chat", RequestURLPath: c.Request.URL.Path, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: kittypes.RelayFormatOpenAI}
	actual.InitChannelMeta(c)
	actual.UpstreamModelName = "approved-target"
	changedActual := *actual
	changedMeta := *actual.ChannelMeta
	changedActual.ChannelMeta = &changedMeta
	changedActual.ChannelOtherSettings.PluginConfig = common.RawMessage(`{"tier":"unapproved"}`)
	deniedAdaptor := &providerplugin.Adaptor{}
	deniedAdaptor.Init(&changedActual)
	_, runtimeErr := deniedAdaptor.DoRequest(c, &changedActual, strings.NewReader(`{"model":"approved-target"}`))
	require.Error(t, runtimeErr)
	assert.Zero(t, exchanges.Load(), "actual unapproved hook config must fail before key-exchange fetch")
	assert.Zero(t, dispatches.Load())
	adaptor := &providerplugin.Adaptor{}
	adaptor.Init(actual)
	response, runtimeErr := adaptor.DoRequest(c, actual, strings.NewReader(`{"model":"approved-target"}`))
	require.NoError(t, runtimeErr)
	upstreamResponse, ok := response.(*http.Response)
	require.True(t, ok)
	require.NoError(t, upstreamResponse.Body.Close())
	assert.Equal(t, int32(1), exchanges.Load())
	assert.Equal(t, int32(1), dispatches.Load())
	failedInfo := *actual
	failedMeta := *actual.ChannelMeta
	failedInfo.ChannelMeta = &failedMeta
	failedInfo.ApiKey = "fixture-exchange-retry"
	failedAdaptor := &providerplugin.Adaptor{}
	failedAdaptor.Init(&failedInfo)
	failExchange.Store(true)
	_, runtimeErr = failedAdaptor.DoRequest(c, &failedInfo, strings.NewReader(`{"model":"approved-target"}`))
	require.Error(t, runtimeErr)
	assert.Equal(t, int32(2), exchanges.Load())
	assert.Equal(t, int32(1), dispatches.Load())
	failExchange.Store(false)
	response, runtimeErr = failedAdaptor.DoRequest(c, &failedInfo, strings.NewReader(`{"model":"approved-target"}`))
	require.NoError(t, runtimeErr)
	upstreamResponse, ok = response.(*http.Response)
	require.True(t, ok)
	require.NoError(t, upstreamResponse.Body.Close())
	assert.Equal(t, int32(3), exchanges.Load(), "retry must prepare the key again after failed exchange")
	assert.Equal(t, int32(2), dispatches.Load())
	actual.UpstreamModelName = "unapproved-target"
	_, runtimeErr = adaptor.DoRequest(c, actual, strings.NewReader(`{"model":"unapproved-target"}`))
	require.Error(t, runtimeErr, "prepared adaptor cannot bypass a changed logical target")
	assert.Equal(t, int32(2), dispatches.Load())
	actual.UpstreamModelName = "approved-target"
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("group", "removed").Error)
	_, runtimeErr = adaptor.DoRequest(c, actual, strings.NewReader(`{"model":"approved-target"}`))
	require.Error(t, runtimeErr, "removed using group must reject at runtime before dispatch")
	assert.Equal(t, int32(2), dispatches.Load())
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("group", "default").Error)
	planContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	planContext.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(planContext, constant.ContextKeyUsingGroup, "default")
	planContext.Set("request_plugin_plan", struct{}{})
	require.NotNil(t, middleware.SetupContextForSelectedChannel(planContext, &channel, "public-chat"))
	_, publishedKey := planContext.Get(string(constant.ContextKeyChannelKey))
	assert.False(t, publishedKey, "retained unknown plan must reject before selected credential publication")
	// A new registry generation with the same key/version but changed source
	// must not substitute for the exact provider already pinned on this request.
	_, err = registry.Register(source+"\nexport const revision=2;", jsplugin.Options{})
	require.NoError(t, err)
	require.Nil(t, relay.CheckRequestAdmission(c, info), "retained approved pinned source does not silently switch to new registry artifact")
	credentialInfo := *actual
	credentialMeta := *actual.ChannelMeta
	credentialInfo.ChannelMeta = &credentialMeta
	credentialInfo.ApiKey = `{"type":"plugin_credential","plugin":"cmh-host-bound","data":{"fixture":true},"expires_at":1}`
	credentialAdaptor := &providerplugin.Adaptor{}
	credentialAdaptor.Init(&credentialInfo)
	_, runtimeErr = credentialAdaptor.DoRequest(c, &credentialInfo, strings.NewReader(`{"model":"approved-target"}`))
	require.Error(t, runtimeErr, "credential service would resolve current replacement; it must not execute after approving old pinned source")
	assert.Zero(t, refreshes.Load())
	assert.Equal(t, int32(2), dispatches.Load())
	changed := `{"plugin_config":{"tier":"premium"}}`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("settings", changed).Error)
	require.NotNil(t, relay.CheckRequestAdmission(c, info), "fresh config cannot authorize a different frozen hook config")
	_, runtimeErr = adaptor.DoRequest(c, actual, strings.NewReader(`{"model":"approved-target"}`))
	require.Error(t, runtimeErr, "prepared provider must revalidate fresh configuration before more work")
	assert.Equal(t, int32(3), exchanges.Load())
	assert.Equal(t, int32(2), dispatches.Load())
	changedChannel := channel
	changedChannel.OtherSettings = changed
	freshIdentity, err := model.PluginExecutorIdentity(&changedChannel, plugin, uint64(identity.Generation), "provider", "/v1/chat/completions")
	require.NoError(t, err)
	bothGate, err := NewGate(public, upstream, []core.ExecutorIdentity{identity, freshIdentity})
	require.NoError(t, err)
	core.Models = bothGate
	require.NotNil(t, relay.CheckRequestAdmission(c, info), "even independently approved fresh config cannot replace frozen selected config")
}

func runHostProviderChainAdmission(t *testing.T, previousGate *Gate, noop core.ModelGate) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 5
	defer func() { constant.StreamingTimeout = oldTimeout }()
	previousRegistry := jsplugin.DefaultRegistry
	previousDestinations := core.Destinations
	defer func() { core.Destinations = previousDestinations }()
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	defer func() { jsplugin.DefaultRegistry, core.Models = previousRegistry, previousGate }()
	var calls, handshakes, messages atomic.Int32
	var mode atomic.Int32
	var channel model.Channel
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			handshakes.Add(1)
			defer ws.Close()
			if _, _, err = ws.ReadMessage(); err != nil {
				return
			}
			messages.Add(1)
			if mode.Load() == 3 {
				if err = model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
					t.Error(err)
				}
			}
			_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"ok":true}`))
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if mode.Load() == 1 && r.URL.Path == "/first" {
			if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
				t.Error(err)
			}
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	service.InitHttpClient()
	const source = `export const meta={apiVersion:1,key:"cmh-chain-admission",name:"Chain admission",version:"1.0.0",author:{name:"Test"},relay:[{incoming_path:"/v1/chat/completions",upstream_path:"/first",converter:"plugin"}]};
export function convertRequest(ctx,req){if(req.body.ws)return {websocket:{path:"/socket",messages:["start"]},state:{stream:req.stream}};return {path:"/first",body:req.body,state:{step:1}};}
export function convertResponse(ctx,res){if(ctx.state.step===1)return {next:{path:"/second",method:"GET"},state:{step:2}};return {body:{ok:true}};}
export function convertStreamEvent(ctx,event){if(ctx.state.stream)return {events:[{data:{ok:true}}],close:true};return {body:{ok:true},close:true};}`
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	base, binding := upstream.URL, `{"task_plugin_key":"cmh-chain-admission"}`
	channel = model.Channel{Id: 95004, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "public-chat", Group: "default", Key: "fixture", BaseURL: &base, Setting: &binding}
	core.Models = noop
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(model.DB))
	defer func() {
		core.Models = noop
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("id = ?", channel.Id).Delete(&model.Channel{}).Error)
	}()
	identity, err := model.PluginExecutorIdentity(&channel, plugin, registry.Generation().Number, "provider", "/v1/chat/completions")
	require.NoError(t, err)
	public, err := New([]string{"public-chat"}, []string{"61"})
	require.NoError(t, err)
	target, err := New([]string{"public-chat"}, nil)
	require.NoError(t, err)
	gate, err := NewGate(public, target, []core.ExecutorIdentity{identity})
	require.NoError(t, err)
	core.Models = gate
	for _, scenario := range []struct {
		name                               string
		mode                               int32
		ws                                 bool
		denied                             bool
		wantCalls, wantDials, wantMessages int32
	}{{"chain allowed", 0, false, false, 2, 0, 0}, {"chain revoked after first", 1, false, true, 1, 0, 0}, {"websocket allowed", 2, true, false, 0, 1, 1}, {"websocket revoked before conversion", 3, true, true, 0, 1, 1}, {"stream pending revoked", 4, true, false, 0, 1, 1}, {"websocket destination denied", 5, true, true, 0, 0, 0}, {"websocket destination approved", 6, true, false, 0, 1, 1}} {
		t.Run(scenario.name, func(t *testing.T) {
			core.Destinations = previousDestinations
			if scenario.mode == 5 {
				deniedTarget, err := NewDestinationPolicy(nil)
				require.NoError(t, err)
				core.Destinations = deniedTarget
			}
			if scenario.mode == 6 {
				digest, err := core.DestinationAddress("ws" + strings.TrimPrefix(upstream.URL, "http") + "/socket")
				require.NoError(t, err)
				exactTarget, err := NewDestinationPolicy([]core.DestinationAdmission{{Kind: "relay_websocket", Method: "GET", AddressSHA256: digest, HostSHA256: core.DestinationHost(""), Model: core.ModelIdentity{Public: "public-chat", Upstream: "public-chat"}, Channel: core.ChannelIdentity{Known: true, ID: channel.Id, Type: channel.Type}}})
				require.NoError(t, err)
				core.Destinations = exactTarget
			}
			mode.Store(scenario.mode)
			calls.Store(0)
			handshakes.Store(0)
			messages.Store(0)
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusEnabled).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "public-chat"))
			info := &relaycommon.RelayInfo{UsingGroup: "default", OriginModelName: "public-chat", RequestURLPath: c.Request.URL.Path, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: kittypes.RelayFormatOpenAI}
			info.InitChannelMeta(c)
			if scenario.mode == 4 {
				info.IsStream = true
			}
			adaptor := &providerplugin.Adaptor{}
			adaptor.Init(info)
			payload := `{"model":"public-chat"}`
			if scenario.ws {
				payload = `{"model":"public-chat","ws":true}`
			}
			var response any
			var callErr error
			require.NotPanics(t, func() { response, callErr = adaptor.DoRequest(c, info, strings.NewReader(payload)) }, "denied WebSocket conversion must close cleanly without nil-timer panic")
			if scenario.denied {
				require.Error(t, callErr)
				wrapped := kittypes.NewOpenAIError(callErr, kittypes.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
				assert.Equal(t, http.StatusForbidden, wrapped.StatusCode)
				assert.False(t, service.ShouldRetryRelayError(c, wrapped, 2), "revocation after partial chain work must not become retryable generic500")
			} else {
				require.NoError(t, callErr)
				httpResponse, ok := response.(*http.Response)
				require.True(t, ok)
				if scenario.mode == 4 {
					require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
					raw, readErr := io.ReadAll(httpResponse.Body)
					require.Error(t, readErr)
					assert.Empty(t, raw, "revoked pending converted events must not reach the caller")
				} else {
					raw, readErr := io.ReadAll(httpResponse.Body)
					require.NoError(t, readErr)
					assert.JSONEq(t, `{"ok":true}`, string(raw))
				}
				closeErr := httpResponse.Body.Close()
				if scenario.mode == 4 && closeErr != nil {
					assert.ErrorIs(t, closeErr, net.ErrClosed)
				} else {
					require.NoError(t, closeErr)
				}
			}
			assert.Equal(t, scenario.wantCalls, calls.Load())
			assert.Equal(t, scenario.wantDials, handshakes.Load())
			assert.Equal(t, scenario.wantMessages, messages.Load())
		})
	}
}

type frozenPollHookVeto struct{ export string }

func (gate frozenPollHookVeto) CheckHook(_ context.Context, identity core.PluginHookIdentity) error {
	if gate.export == "" || identity.Export == gate.export {
		return errors.New("fixture source approval revoked")
	}
	return nil
}

type failingContinuationStore struct{ core.ContinuationStore }

func (f *failingContinuationStore) Save(context.Context, core.ContinuationBinding, core.ContinuationArtifact) (core.ContinuationRef, error) {
	return core.ContinuationRef{}, core.ErrContinuationUnavailable
}

type countedContinuationStore struct {
	core.ContinuationStore
	loads int
}

func (s *countedContinuationStore) Load(ctx context.Context, b core.ContinuationBinding, r core.ContinuationRef, p core.ContinuationPurpose) (core.ContinuationArtifact, error) {
	s.loads++
	return s.ContinuationStore.Load(ctx, b, r, p)
}

type cancellingContinuationStore struct {
	core.ContinuationStore
	cancel context.CancelFunc
}

func (s *cancellingContinuationStore) Load(ctx context.Context, b core.ContinuationBinding, r core.ContinuationRef, p core.ContinuationPurpose) (core.ContinuationArtifact, error) {
	a, err := s.ContinuationStore.Load(ctx, b, r, p)
	s.cancel()
	return a, err
}

func runHostTaskDriverAdmission(t *testing.T, previousGate *Gate, noop core.ModelGate) {
	t.Helper()
	previousDestinations := core.Destinations
	previousContinuations := core.TaskContinuations
	defer func() { core.TaskContinuations = previousContinuations }()
	require.NoError(t, migrations.MigrateContinuation(model.DB))
	defer func() { require.NoError(t, model.DB.Migrator().DropTable(&migrations.ContinuationRecord{})) }()
	archive, err := continuation.New(model.DB, "fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{0x51}, 32)})
	require.NoError(t, err)
	defer func() { core.Destinations = previousDestinations }()
	previousRegistry := jsplugin.DefaultRegistry
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	defer func() { jsplugin.DefaultRegistry, core.Models = previousRegistry, previousGate }()
	var lookups, submissions, mediaCalls, queryCalls atomic.Int32
	var mediaRevision, mediaTier, mediaAuthorization string
	var signedTier string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/lookup" {
			lookups.Add(1)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		if r.URL.Path == "/query" {
			queryCalls.Add(1)
			_, _ = w.Write([]byte(`{"status":"SUCCESS"}`))
			return
		}
		if r.URL.Path == "/media" {
			mediaCalls.Add(1)
			mediaRevision = r.Header.Get("X-Revision")
			mediaTier = r.Header.Get("X-Tier")
			mediaAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Range", "bytes 0-3/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("data"))
			return
		}
		signedTier = r.Header.Get("X-Tier")
		submissions.Add(1)
		_, _ = w.Write([]byte(`{"id":"local-task"}`))
	}))
	defer upstream.Close()
	service.InitHttpClient()
	const source = `export const meta={apiVersion:1,key:"cmh-task-admission",name:"Task admission",version:"1.0.0",author:{name:"Test"},models:["public-task"],fetchMode:"per_task",channelConfig:{type:"object",properties:{tier:{type:"string",title:{en:"Tier",zh:"档位"},default:"approved"}}}};
export function buildSubmitRequest(ctx){utils.fetch({url:ctx.baseUrl+"/lookup"});if(ctx.config.tier!=="approved")throw new Error("wrong actual config");return {url:ctx.baseUrl+"/submit",method:"POST",rewriteModel:ctx.requestBody.rewrite||ctx.upstreamModel,body:{model:ctx.upstreamModel}};}
export function parseSubmitResponse(){return {taskId:"local-task"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}
export function signRequest(ctx){if(ctx.config.tier!=="approved")throw new Error("wrong signer config");return {headers:{"X-Tier":ctx.config.tier}};}
export function parseTaskResult(ctx){if(ctx.state&&ctx.state.ordinaryFailure)throw new Error("plugin execution rejected");return {status:"IN_PROGRESS"};}
export function extractUsageOnComplete(){return {};}
export function listArtifacts(){return [{key:"video-main",type:"video"}];}
export function buildContentRequest(ctx){return {url:ctx.baseUrl+"/media",method:"GET",headers:{"X-Revision":"A","Authorization":"Bearer "+ctx.apiKey}};}`
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	base := upstream.URL
	binding := `{"task_plugin_key":"cmh-task-admission"}`
	stored := model.Channel{Id: 95003, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "public-task", Group: "default", Key: "fixture", BaseURL: &base, Setting: &binding, OtherSettings: `{"plugin_config":{"tier":"approved"}}`}
	core.Models = noop
	require.NoError(t, model.DB.Create(&stored).Error)
	require.NoError(t, stored.AddAbilities(model.DB))
	defer func() {
		core.Models = noop
		require.NoError(t, model.DB.Where("channel_id = ?", stored.Id).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("id = ?", stored.Id).Delete(&model.Channel{}).Error)
		model.InitChannelCache()
	}()
	identity, err := model.PluginExecutorIdentity(&stored, plugin, registry.Generation().Number, "task", "/v1/tasks/cmh-task-admission")
	require.NoError(t, err)
	public, err := New([]string{"public-task"}, []string{"61"})
	require.NoError(t, err)
	upstreamPolicy, err := New([]string{"public-task"}, nil)
	require.NoError(t, err)
	gate, err := NewGate(public, upstreamPolicy, []core.ExecutorIdentity{identity})
	require.NoError(t, err)
	core.Models = gate
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/tasks/cmh-task-admission", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set("expected_task_plugin_key", plugin.Meta.Key)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Plugin: plugin, Generation: registry.Generation()})
	service.GetChannelConstraints(c).AddFilter(hostdto.ChannelFilter{Kind: hostdto.FilterTaskPluginIdentity, TaskPluginKey: plugin.Meta.Key, RequestPath: c.Request.URL.Path})
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, &stored, "public-task"))
	c.Set("task_request", map[string]any{"prompt": "local"})
	c.Set("task_action", "generate")
	probe := &admissionBillingProbe{}
	info := &relaycommon.RelayInfo{OriginModelName: "public-task", UsingGroup: "default", Billing: probe, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	info.InitChannelMeta(c)
	info.UpstreamModelName = "public-task"
	info.UserId = 71
	info.PublicTaskID = "synthetic-pre-submit-archive"
	core.TaskContinuations = archive
	c.Set(core.ContextKeyTaskContinuation, core.ContinuationRef{ID: "stale", Digest: "stale"})
	bad := *info
	badMeta := *info.ChannelMeta
	bad.ChannelMeta = &badMeta
	bad.ChannelOtherSettings.PluginConfig = common.RawMessage(`{"tier":"unapproved"}`)
	badAdaptor := taskjsplugin.New(plugin)
	badAdaptor.Init(&bad)
	deniedTask := badAdaptor.ValidateRequestAndSetAction(c, &bad)
	require.NotNil(t, deniedTask)
	assert.True(t, deniedTask.NoRetry)
	assert.Equal(t, http.StatusForbidden, deniedTask.StatusCode)
	assert.Zero(t, lookups.Load())
	assert.Zero(t, submissions.Load())
	assert.Zero(t, probe.reserve)
	stale, _ := c.Get(core.ContextKeyTaskContinuation)
	assert.Equal(t, core.ContinuationRef{}, stale)
	var count int64
	require.NoError(t, model.DB.Model(&migrations.ContinuationRecord{}).Count(&count).Error)
	assert.Zero(t, count)
	// Deliberately stale channel cache must not replace the checked runtime config.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Update("settings", `{"plugin_config":{"tier":"stale-cache"}}`).Error)
	model.InitChannelCache()
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Update("settings", stored.OtherSettings).Error)
	capability, err := service.PluginFetchExecutor(plugin, base, "", info.ChannelSetting)
	require.NoError(t, err)
	lookupHash, err := core.DestinationAddress(base + "/lookup")
	require.NoError(t, err)
	baseURL, err := url.Parse(base)
	require.NoError(t, err)
	lookupTarget := core.DestinationAdmission{Kind: "plugin_fetch", Method: "GET", AddressSHA256: lookupHash, HostSHA256: core.DestinationHost(baseURL.Host), Executor: capability, Hook: "buildSubmitRequest"}
	fetchOnly, err := NewDestinationPolicy([]core.DestinationAdmission{lookupTarget})
	require.NoError(t, err)
	core.Destinations = fetchOnly
	core.Models = noop
	noGateAdaptor := taskjsplugin.New(plugin)
	noGateAdaptor.Init(info)
	require.NotNil(t, noGateAdaptor.ValidateRequestAndSetAction(c, info))
	assert.Zero(t, lookups.Load())
	assert.Zero(t, submissions.Load())
	core.Models = gate
	missingTaskState := *info
	missingTaskState.TaskRelayInfo = nil
	missingAdaptor := taskjsplugin.New(plugin)
	missingAdaptor.Init(&missingTaskState)
	require.NotPanics(t, func() { require.NotNil(t, missingAdaptor.ValidateRequestAndSetAction(c, &missingTaskState)) })
	assert.Zero(t, lookups.Load())
	core.TaskContinuations = &failingContinuationStore{ContinuationStore: archive}
	failedArchiveAdaptor := taskjsplugin.New(plugin)
	failedArchiveAdaptor.Init(info)
	require.NotNil(t, failedArchiveAdaptor.ValidateRequestAndSetAction(c, info))
	assert.Zero(t, lookups.Load())
	assert.Zero(t, submissions.Load())
	failedRef, _ := c.Get(core.ContextKeyTaskContinuation)
	assert.Equal(t, core.ContinuationRef{}, failedRef)
	core.TaskContinuations = archive
	adaptor := taskjsplugin.New(plugin)
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, int32(1), lookups.Load())
	snapshot := service.TaskExecutionSnapshotFromContext(c)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Continuation)
	bindingArchive := core.ContinuationBinding{TaskID: info.PublicTaskID, UserID: info.UserId, ChannelID: info.ChannelId}
	archived, err := archive.Load(t.Context(), bindingArchive, *snapshot.Continuation, core.ContinuationPoll)
	require.NoError(t, err)
	assert.Equal(t, []byte(source), archived.Source)
	persisted := &model.Task{Status: model.TaskStatusSubmitted, SubmitTime: time.Now().Unix(), Quota: 13, Platform: constant.TaskPlatform(plugin.Meta.Key), Properties: model.Properties{OriginModelName: info.OriginModelName, UpstreamModelName: info.UpstreamModelName}, TaskID: info.PublicTaskID, UserId: info.UserId, ChannelId: info.ChannelId, PrivateData: model.TaskPrivateData{Execution: snapshot}}
	require.NoError(t, model.DB.Create(persisted).Error)
	defer func() { require.NoError(t, model.DB.Where("id = ?", persisted.ID).Delete(&model.Task{}).Error) }()
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, persisted.ID).Error)
	require.NotNil(t, reloaded.PrivateData.Execution)
	assert.Equal(t, snapshot.Continuation, reloaded.PrivateData.Execution.Continuation)
	queryHash, err := core.DestinationAddress(base + "/query")
	require.NoError(t, err)
	queryTarget := core.DestinationAdmission{Kind: "task_query", Method: "GET", AddressSHA256: queryHash, HostSHA256: core.DestinationHost(baseURL.Host), Executor: capability, Hook: "buildQueryRequest"}
	queryPolicy, err := NewDestinationPolicy([]core.DestinationAdmission{queryTarget})
	require.NoError(t, err)
	core.Destinations = queryPolicy
	reloaded.PrivateData.UpstreamTaskID = "archived-upstream"
	cancelCtx, cancelResolve := context.WithCancel(t.Context())
	core.TaskContinuations = &cancellingContinuationStore{ContinuationStore: archive, cancel: cancelResolve}
	_, err = relay.GetTaskAdaptorForTask(cancelCtx, &reloaded, core.ContinuationPoll)
	require.ErrorIs(t, err, context.Canceled)
	core.TaskContinuations = archive
	resolved, err := relay.GetTaskAdaptorForTask(t.Context(), &reloaded, core.ContinuationPoll)
	require.NoError(t, err)
	queryResponse, err := resolved.FetchTask(base, "fixture", &reloaded, "")
	require.NoError(t, err)
	require.NoError(t, queryResponse.Body.Close())
	for _, alter := range []func(*model.Task){func(task *model.Task) { task.TaskID = "different-task" }, func(task *model.Task) { task.UserId++ }, func(task *model.Task) { task.ChannelId++ }, func(task *model.Task) { task.Properties.OriginModelName = "different-model" }} {
		other := reloaded
		alter(&other)
		_, err := relay.GetTaskAdaptorForTask(t.Context(), &other, core.ContinuationPoll)
		require.ErrorIs(t, err, core.ErrContinuationUnavailable)
		_, err = resolved.FetchTask(base, "fixture", &other, "")
		require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	}
	if artifacts, ok := resolved.(relaychannel.TaskArtifactProvider); ok {
		_, err = artifacts.ListArtifacts(&reloaded)
		require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	}
	upstreamChanged := reloaded
	upstreamChanged.PrivateData.UpstreamTaskID = "other-upstream-job"
	_, err = resolved.FetchTask(base, "fixture", &upstreamChanged, "")
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	accountChanged := reloaded
	accountChanged.PrivateData.CredentialAccount = "other-account"
	_, err = resolved.FetchTask(base, "fixture", &accountChanged, "")
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	_, err = relay.GetTaskAdaptorForTask(t.Context(), &reloaded, core.ContinuationPurpose("submit"))
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	contentOnly, err := relay.GetTaskAdaptorForTask(t.Context(), &reloaded, core.ContinuationContent)
	require.NoError(t, err)
	_, err = contentOnly.FetchTask(base, "fixture", &reloaded, "")
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	core.TaskContinuations = nil
	_, err = relay.GetTaskAdaptorForTask(t.Context(), &reloaded, core.ContinuationPoll)
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	core.TaskContinuations = archive
	require.NoError(t, model.DB.Save(&reloaded).Error)
	previousFactory, previousContinuationFactory := service.GetTaskAdaptorFunc, service.GetTaskContinuationFunc
	defer func() {
		service.GetTaskAdaptorFunc = previousFactory
		service.GetTaskContinuationFunc = previousContinuationFactory
	}()
	service.GetTaskContinuationFunc = func(ctx context.Context, task *model.Task) (service.TaskPollingAdaptor, *model.Channel, error) {
		adaptor, err := relay.GetTaskAdaptorForTask(ctx, task, core.ContinuationPoll)
		if err != nil {
			return nil, nil, err
		}
		frozen, err := adaptor.(*taskjsplugin.TaskAdaptor).ContinuationChannel()
		if err != nil {
			return nil, nil, err
		}
		frozen.Key = stored.Key
		return adaptor, frozen, nil
	}
	var currentFactoryCalls int
	service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { currentFactoryCalls++; return nil }
	service.DispatchPlatformUpdate(t.Context(), reloaded.Platform, map[int][]string{stored.Id: {reloaded.GetUpstreamTaskID()}}, map[string]*model.Task{reloaded.GetUpstreamTaskID(): &reloaded})
	var polled model.Task
	require.NoError(t, model.DB.First(&polled, reloaded.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), polled.Status)
	assert.Equal(t, 13, polled.Quota)
	assert.Zero(t, currentFactoryCalls, "archived task must never select current platform factory")
	previousHooks := core.PluginHooks
	defer func() { core.PluginHooks = previousHooks }()
	for _, scenario := range []struct {
		name         string
		hooks        core.PluginHookGate
		state        common.RawMessage
		wantQueries  int32
		wantFailures int
	}{
		{name: "frozen_query_source_veto", hooks: frozenPollHookVeto{}, wantFailures: 2},
		{name: "frozen_parse_source_veto", hooks: frozenPollHookVeto{export: "parseTaskResult"}, wantQueries: 1, wantFailures: 2},
		{name: "frozen_sign_source_veto", hooks: frozenPollHookVeto{export: "signRequest"}, wantFailures: 2},
		{name: "frozen_usage_source_veto", hooks: frozenPollHookVeto{export: "extractUsageOnComplete"}, wantQueries: 1, wantFailures: 2},
		{name: "frozen_javascript_same_text_throw_is_ordinary", hooks: previousHooks, state: common.RawMessage(`{"ordinaryFailure":true}`), wantQueries: 1, wantFailures: 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			candidate := polled
			candidate.Status = model.TaskStatusInProgress
			candidate.PrivateData.PollFailures = 2
			candidate.PrivateData.PluginState = scenario.state
			candidate.PrivateData.BillingSource = "wallet"
			candidate.PrivateData.SubscriptionId = 917
			candidate.PrivateData.TokenId = 918
			candidate.PrivateData.NodeName = "frozen-poll-fixture"
			require.NoError(t, model.DB.Save(&candidate).Error)
			beforePrivate, err := common.Marshal(candidate.PrivateData)
			require.NoError(t, err)
			beforeQuery := queryCalls.Load()
			beforeData := append([]byte(nil), candidate.Data...)
			core.PluginHooks = scenario.hooks
			service.DispatchPlatformUpdate(t.Context(), candidate.Platform, map[int][]string{stored.Id: {candidate.GetUpstreamTaskID()}}, map[string]*model.Task{candidate.GetUpstreamTaskID(): &candidate})
			var after model.Task
			require.NoError(t, model.DB.First(&after, candidate.ID).Error)
			assert.Equal(t, scenario.wantQueries, queryCalls.Load()-beforeQuery)
			assert.Equal(t, scenario.wantFailures, after.PrivateData.PollFailures)
			assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), after.Status)
			assert.Equal(t, candidate.Quota, after.Quota)
			assert.Equal(t, candidate.FinishTime, after.FinishTime)
			assert.Equal(t, candidate.FailReason, after.FailReason)
			assert.Equal(t, beforeData, []byte(after.Data))
			// Compare the complete private funding/provenance snapshot while
			// allowing only the ordinary-error control's failure-count change.
			after.PrivateData.PollFailures = 2
			afterPrivate, err := common.Marshal(after.PrivateData)
			require.NoError(t, err)
			assert.JSONEq(t, string(beforePrivate), string(afterPrivate))
			assert.Zero(t, probe.reserve)
			assert.Zero(t, probe.settle)
			assert.Zero(t, probe.refund)
			assert.Zero(t, currentFactoryCalls)
		})
	}
	core.PluginHooks = previousHooks
	require.NoError(t, model.DB.Save(&polled).Error)
	beforeFailure := polled.PrivateData.PollFailures
	core.TaskContinuations = nil
	service.DispatchPlatformUpdate(t.Context(), polled.Platform, map[int][]string{stored.Id: {polled.GetUpstreamTaskID()}}, map[string]*model.Task{polled.GetUpstreamTaskID(): &polled})
	var unavailable model.Task
	require.NoError(t, model.DB.First(&unavailable, polled.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), unavailable.Status)
	assert.Equal(t, beforeFailure, unavailable.PrivateData.PollFailures)
	assert.Equal(t, 13, unavailable.Quota)
	core.TaskContinuations = archive
	accountTask := reloaded
	accountTask.PrivateData.CredentialAccount = "submitted-account"
	accountAdaptor, err := relay.GetTaskAdaptorForTask(t.Context(), &accountTask, core.ContinuationPoll)
	require.NoError(t, err)
	_, err = accountAdaptor.FetchTask(base, "fixture-static-key", &accountTask, "")
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	batchSource := strings.Replace(source, `fetchMode:"per_task"`, `fetchMode:"batch"`, 1)
	batchSource = strings.Replace(batchSource, `export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}`, `export function buildBatchQueryRequest(ctx,tasks){return {url:ctx.baseUrl+"/query",method:"GET"};}
export function parseBatchResult(ctx,body){return ctx.tasks.map(t=>({taskId:t.taskId,status:"IN_PROGRESS"}));}`, 1)
	batchPlugin, err := jsplugin.CompilePlugin(batchSource, jsplugin.Options{})
	require.NoError(t, err)
	batchAdaptor := taskjsplugin.New(batchPlugin)
	batchAdaptor.Init(info)
	batchArtifact, err := batchAdaptor.CaptureContinuationArtifact(info)
	require.NoError(t, err)
	batchTask := reloaded
	batchTask.ID = 0
	batchTask.TaskID = "synthetic-archived-batch"
	batchTask.Status = model.TaskStatusSubmitted
	batchTask.PrivateData.Execution = &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: batchPlugin.Meta.Key}}
	batchRef, err := archive.Save(t.Context(), core.ContinuationBinding{TaskID: batchTask.TaskID, UserID: batchTask.UserId, ChannelID: batchTask.ChannelId}, batchArtifact)
	require.NoError(t, err)
	batchTask.PrivateData.Execution.Continuation = &batchRef
	require.NoError(t, model.DB.Create(&batchTask).Error)
	defer func() { require.NoError(t, model.DB.Where("id = ?", batchTask.ID).Delete(&model.Task{}).Error) }()
	batchCapability, err := service.PluginFetchExecutor(batchPlugin, base, "", info.ChannelSetting)
	require.NoError(t, err)
	batchTarget := queryTarget
	batchTarget.Executor = batchCapability
	batchTarget.Hook = "buildBatchQueryRequest"
	batchPolicy, err := NewDestinationPolicy([]core.DestinationAdmission{batchTarget})
	require.NoError(t, err)
	core.Destinations = batchPolicy
	service.DispatchPlatformUpdate(t.Context(), batchTask.Platform, map[int][]string{stored.Id: {batchTask.GetUpstreamTaskID()}}, map[string]*model.Task{batchTask.GetUpstreamTaskID(): &batchTask})
	var batchPolled model.Task
	require.NoError(t, model.DB.First(&batchPolled, batchTask.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), batchPolled.Status)
	assert.Zero(t, batchPolled.PrivateData.PollFailures)
	assert.Equal(t, 13, batchPolled.Quota)
	for _, export := range []string{"buildBatchQueryRequest", "parseBatchResult", "signRequest", "extractUsageOnComplete"} {
		t.Run("frozen_batch_veto_"+export, func(t *testing.T) {
			candidate := batchPolled
			candidate.PrivateData.PollFailures = 2
			require.NoError(t, model.DB.Save(&candidate).Error)
			before := queryCalls.Load()
			core.PluginHooks = frozenPollHookVeto{export: export}
			service.DispatchPlatformUpdate(t.Context(), candidate.Platform, map[int][]string{stored.Id: {candidate.GetUpstreamTaskID()}}, map[string]*model.Task{candidate.GetUpstreamTaskID(): &candidate})
			var after model.Task
			require.NoError(t, model.DB.First(&after, candidate.ID).Error)
			assert.Equal(t, 2, after.PrivateData.PollFailures)
			assert.Equal(t, candidate.Status, after.Status)
			assert.Equal(t, candidate.Quota, after.Quota)
			want := int32(1)
			if export == "buildBatchQueryRequest" || export == "signRequest" {
				want = 0
			}
			assert.Equal(t, want, queryCalls.Load()-before)
			assert.Zero(t, probe.refund)
			assert.Zero(t, probe.settle)
		})
	}
	core.PluginHooks = previousHooks
	require.NoError(t, model.DB.Save(&batchPolled).Error)
	var wrongTargetCalls atomic.Int32
	wrongTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrongTargetCalls.Add(1)
		_, _ = w.Write([]byte("wrong-target"))
	}))
	defer wrongTarget.Close()
	sourceB := strings.Replace(source, `key:"video-main",type:"video"`, `key:"different-current-artifact",type:"video"`, 1)
	_, err = registry.RegisterFactory(sourceB, jsplugin.Options{})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Updates(map[string]any{"base_url": wrongTarget.URL, "settings": `{"plugin_config":{"tier":"current-B"}}`, "key": "fixture-live-key"}).Error)
	model.InitChannelCache()
	reloaded.Status = model.TaskStatusSuccess
	reloaded.PrivateData.ResultURL = wrongTarget.URL + "/fallback"
	require.NoError(t, model.DB.Save(&reloaded).Error)
	oldFetch := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{"1-65535"}
	defer func() { *system_setting.GetFetchSetting() = oldFetch }()
	mediaHash, err := core.DestinationAddress(base + "/media")
	require.NoError(t, err)
	mediaTarget := core.DestinationAdmission{Kind: "task_content", Method: "GET", AddressSHA256: mediaHash, HostSHA256: core.DestinationHost(baseURL.Host), Model: core.ModelIdentity{Public: info.OriginModelName, Upstream: info.UpstreamModelName}, Channel: core.ChannelIdentity{Known: true, ID: stored.Id, Type: stored.Type}}
	mediaPolicy, err := NewDestinationPolicy([]core.DestinationAdmission{mediaTarget})
	require.NoError(t, err)
	core.Destinations = mediaPolicy
	contentCall := func(owner int, video bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", owner)
		ctx.Set("role", common.RoleCommonUser)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+reloaded.TaskID+"/artifacts/video-main/content", nil)
		ctx.Request.Header.Set("Range", "bytes=0-3")
		if video {
			ctx.Params = gin.Params{{Key: "task_id", Value: reloaded.TaskID}}
			controller.VideoProxy(ctx)
		} else {
			ctx.Params = gin.Params{{Key: "key", Value: reloaded.TaskID}, {Key: "artifact_key", Value: "video-main"}}
			controller.TaskArtifactContent(ctx)
		}
		return recorder
	}
	contentResponse := contentCall(info.UserId, false)
	assert.Equal(t, http.StatusPartialContent, contentResponse.Code)
	assert.Equal(t, "data", contentResponse.Body.String())
	assert.Equal(t, "A", mediaRevision)
	assert.Equal(t, "approved", mediaTier)
	assert.Equal(t, "Bearer fixture-live-key", mediaAuthorization)
	assert.Zero(t, wrongTargetCalls.Load())
	beforeMedia := mediaCalls.Load()
	foreign := contentCall(info.UserId+1, false)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	assert.Equal(t, beforeMedia, mediaCalls.Load())
	privateKeyChanged := reloaded
	privateKeyChanged.PrivateData.Key = "substitute-private-key"
	boundContent, err := relay.GetTaskAdaptorForTask(t.Context(), &reloaded, core.ContinuationContent)
	require.NoError(t, err)
	attach := boundContent.(*taskjsplugin.TaskAdaptor)
	require.ErrorIs(t, attach.AttachContinuationCredentials(t.Context(), &privateKeyChanged, &stored), core.ErrContinuationUnavailable)
	cancelledResponse := httptest.NewRecorder()
	cancelledContext, _ := gin.CreateTestContext(cancelledResponse)
	cancelledContext.Set("id", info.UserId)
	cancelledContext.Set("role", common.RoleCommonUser)
	cancelledContext.Params = gin.Params{{Key: "key", Value: reloaded.TaskID}, {Key: "artifact_key", Value: "video-main"}}
	cancelled, cancelNow := context.WithCancel(t.Context())
	cancelNow()
	cancelledContext.Request = httptest.NewRequest(http.MethodGet, "/artifact", nil).WithContext(cancelled)
	beforeCancelled := mediaCalls.Load()
	controller.TaskArtifactContent(cancelledContext)
	assert.Equal(t, http.StatusServiceUnavailable, cancelledResponse.Code)
	assert.Equal(t, beforeCancelled, mediaCalls.Load())
	core.TaskContinuations = nil
	missing := contentCall(info.UserId, false)
	assert.Equal(t, http.StatusServiceUnavailable, missing.Code)
	missingVideo := contentCall(info.UserId, true)
	assert.Equal(t, http.StatusServiceUnavailable, missingVideo.Code)
	assert.Zero(t, wrongTargetCalls.Load(), "archived VideoProxy cannot fallback to stored raw ResultURL")
	core.TaskContinuations = archive
	_, err = registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Updates(map[string]any{"base_url": base, "settings": stored.OtherSettings, "key": stored.Key}).Error)
	model.InitChannelCache()
	core.Destinations = fetchOnly
	encodedTask, err := common.Marshal(reloaded.PrivateData)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedTask), "export function")
	assert.NotContains(t, string(encodedTask), `\"RuntimeJSON\"`)
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	_, err = adaptor.DoRequest(c, info, body)
	require.Error(t, err)
	assert.Zero(t, submissions.Load(), "task submit must hit final relay target gate")
	submitHash, err := core.DestinationAddress(base + "/submit")
	require.NoError(t, err)
	submitTarget := core.DestinationAdmission{Kind: "relay_http", Method: "POST", AddressSHA256: submitHash, HostSHA256: core.DestinationHost(baseURL.Host), Model: core.ModelIdentity{Public: "public-task", Upstream: "public-task"}, Channel: core.ChannelIdentity{Known: true, ID: stored.Id, Type: stored.Type}}
	bothTargets, err := NewDestinationPolicy([]core.DestinationAdmission{lookupTarget, submitTarget})
	require.NoError(t, err)
	core.Destinations = bothTargets
	body, err = adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	response, err := adaptor.DoRequest(c, info, body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int32(1), submissions.Load())
	assert.Equal(t, "approved", signedTier, "actual signRequest must not read deliberately stale cache config")
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
	_, err = adaptor.BuildRequestBody(c, info)
	require.Error(t, err, "cached descriptor body reuse must revalidate current status")
	_, err = adaptor.DoRequest(c, info, strings.NewReader(`{"model":"public-task"}`))
	require.Error(t, err, "cached final task dispatch must not bypass fresh admission")
	assert.Equal(t, int32(1), submissions.Load())
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Update("status", common.ChannelStatusEnabled).Error)
	cachedSettings := info.ChannelOtherSettings
	updatedStored := stored
	updatedStored.OtherSettings = `{"plugin_config":{"tier":"updated-approved"}}`
	updatedIdentity, err := model.PluginExecutorIdentity(&updatedStored, plugin, uint64(identity.Generation), "task", c.Request.URL.Path)
	require.NoError(t, err)
	bothConfigs, err := NewGate(public, upstreamPolicy, []core.ExecutorIdentity{identity, updatedIdentity})
	require.NoError(t, err)
	core.Models = bothConfigs
	info.ChannelOtherSettings.PluginConfig = common.RawMessage(`{"tier":"updated-approved"}`)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Update("settings", updatedStored.OtherSettings).Error)
	_, err = adaptor.BuildRequestBody(c, info)
	require.Error(t, err, "old cached descriptor cannot inherit a separately approved new building config")
	_, err = adaptor.DoRequest(c, info, strings.NewReader(`{"model":"public-task"}`))
	require.Error(t, err)
	assert.Equal(t, int32(1), submissions.Load())
	info.ChannelOtherSettings = cachedSettings
	core.Models = gate
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", stored.Id).Update("settings", stored.OtherSettings).Error)
	c.Set("task_request", map[string]any{"prompt": "local", "rewrite": "unapproved-task"})
	changed := *info
	changedMeta := *info.ChannelMeta
	changed.ChannelMeta = &changedMeta
	changedAdaptor := taskjsplugin.New(plugin)
	changedAdaptor.Init(&changed)
	require.NotNil(t, changedAdaptor.ValidateRequestAndSetAction(c, &changed), "descriptor rewrite must be rejected before reserve or submission")
	assert.Equal(t, int32(2), lookups.Load(), "approved build hook may do read-only lookup before returning a denied descriptor")
	assert.Equal(t, int32(1), submissions.Load())
	assert.Zero(t, probe.reserve)
	assert.Zero(t, probe.settle)
	assert.Zero(t, probe.refund)
	runFrozenTaskReport(t, archive, stored.Id, base, info.UserId)
}

func runFrozenTaskReport(t *testing.T, archive core.ContinuationStore, channelID int, _ string, userID int) {
	t.Helper()
	oldInstances, err := common.Marshal(request_plugin_setting.GetInstances())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(oldInstances)}))
	}()
	oldTargets := core.Destinations
	defer func() { core.Destinations = oldTargets }()
	var reportsReceived atomic.Int32
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reportsReceived.Add(1)
		received <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	const source = `export const meta={apiVersion:1,key:"cmh-frozen-report",name:"Frozen report",version:"1.0.0",author:{name:"Test"},configSchema:{type:"object",properties:{url:{type:"string",title:{en:"Service URL",zh:"服务地址"},format:"url"},marker:{type:"string",title:{en:"Marker",zh:"标记"}}}}};
export function report(ctx,record){utils.fetch({url:ctx.config.url,method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({source:"A",marker:ctx.config.marker})});}`
	plugin, err := jsplugin.DefaultRegistry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	instances := []request_plugin_setting.Instance{{ID: "frozen-report", Name: "Frozen", Plugin: plugin.Meta.Key, Enabled: true, Global: true, Config: common.RawMessage(fmt.Sprintf(`{"url":%q,"marker":"original-A"}`, server.URL))}}
	encoded, err := common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(encoded)}))
	core.TaskContinuations = archive
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/tasks/report", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	_, _, _, apiErr := requestplugin.SubmitIngress(ctx, kittypes.RelayFormatTask, "public-task")
	require.Nil(t, apiErr)
	info := &relaycommon.RelayInfo{UserId: userID, OriginModelName: "public-task", UsingGroup: "default", TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "synthetic-report-frozen"}, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID, ChannelType: constant.ChannelTypeTaskPlugin, UpstreamModelName: "public-task"}}
	core.TaskContinuations = &failingContinuationStore{ContinuationStore: archive}
	failed := requestplugin.TaskReport(ctx, info)
	require.NotNil(t, failed)
	require.True(t, failed.Frozen)
	assert.Empty(t, failed.Continuations)
	core.TaskContinuations = archive
	stored := requestplugin.TaskReport(ctx, info)
	require.NotNil(t, stored)
	require.True(t, stored.Frozen)
	require.Len(t, stored.Continuations, 1)
	task := &model.Task{TaskID: info.PublicTaskID, UserId: userID, ChannelId: channelID, Status: model.TaskStatusSuccess, Group: "default", Properties: model.Properties{OriginModelName: "public-task"}, PrivateData: model.TaskPrivateData{RequestPlugins: stored}}
	require.NoError(t, model.DB.Create(task).Error)
	defer func() { require.NoError(t, model.DB.Where("id = ?", task.ID).Delete(&model.Task{}).Error) }()
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	require.True(t, reloaded.PrivateData.RequestPlugins.Frozen)
	assert.Equal(t, stored.Continuations, reloaded.PrivateData.RequestPlugins.Continuations)
	_, err = jsplugin.DefaultRegistry.RegisterFactory(strings.Replace(source, `source:"A"`, `source:"B"`, 1), jsplugin.Options{})
	require.NoError(t, err)
	instances[0].Config = common.RawMessage(fmt.Sprintf(`{"url":%q,"marker":"current-B"}`, server.URL))
	encoded, err = common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(encoded)}))
	originalConfig := common.RawMessage(fmt.Sprintf(`{"url":%q,"marker":"original-A"}`, server.URL))
	fetchIdentity, err := service.RequestPluginFetchExecutor(plugin, originalConfig)
	require.NoError(t, err)
	target, err := core.DestinationAddress(server.URL)
	require.NoError(t, err)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	policy, err := NewDestinationPolicy([]core.DestinationAdmission{{Kind: "plugin_fetch", Method: "POST", AddressSHA256: target, HostSHA256: core.DestinationHost(parsed.Host), Executor: fetchIdentity, Hook: "report"}})
	require.NoError(t, err)
	core.Destinations = policy
	counted := &countedContinuationStore{ContinuationStore: archive}
	core.TaskContinuations = counted
	partial := reloaded
	partialStored := *reloaded.PrivateData.RequestPlugins
	partialStored.Instances = append(append([]string{}, partialStored.Instances...), "missing-second")
	partial.PrivateData.RequestPlugins = &partialStored
	requestplugin.ReportTask(&partial)
	assert.Zero(t, counted.loads, "partial frozen target set must reject before loading or queueing first target")
	core.TaskContinuations = archive
	instances[0].Enabled = false
	disabledSettings, err := common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(disabledSettings)}))
	countedDisabled := &countedContinuationStore{ContinuationStore: archive}
	core.TaskContinuations = countedDisabled
	requestplugin.ReportTask(&reloaded)
	assert.Zero(t, countedDisabled.loads)
	core.TaskContinuations = archive
	instances[0].Enabled = true
	enabledSettings, err := common.Marshal(instances)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(enabledSettings)}))
	oversized := reloaded
	oversizedStored := *reloaded.PrivateData.RequestPlugins
	oversizedStored.Instances = make([]string, 17)
	oversizedStored.Continuations = make([]model.TaskReportContinuation, 17)
	oversized.PrivateData.RequestPlugins = &oversizedStored
	countedLarge := &countedContinuationStore{ContinuationStore: archive}
	core.TaskContinuations = countedLarge
	requestplugin.ReportTask(&oversized)
	assert.Zero(t, countedLarge.loads)
	core.TaskContinuations = archive
	requestplugin.ReportTask(&reloaded)
	select {
	case payload := <-received:
		assert.Contains(t, payload, `"source":"A"`)
		assert.Contains(t, payload, "original-A")
		assert.NotContains(t, payload, "current-B")
	case <-time.After(5 * time.Second):
		t.Fatal("original frozen report was not delivered")
	}
	before := reportsReceived.Load()
	core.TaskContinuations = nil
	requestplugin.ReportTask(&reloaded)
	assert.Equal(t, before, reportsReceived.Load())
	core.TaskContinuations = archive
	reloaded.PrivateData.RequestPlugins.Continuations = nil
	requestplugin.ReportTask(&reloaded)
	assert.Equal(t, before, reportsReceived.Load(), "Frozen empty cannot fallback to current report settings")
}

func runHostAdmissionSockets(t *testing.T, channel model.Channel, gate *Gate, noop core.ModelGate) {
	t.Helper()
	defer func() { core.Models = gate }()
	oldPrices := ratio_setting.ModelPrice2JSONString()
	oldCountToken := constant.CountToken
	constant.CountToken = false
	oldFreePreconsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	defer func() { operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreePreconsume }()
	defer func() { constant.CountToken = oldCountToken }()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public-chat":0}`))
	defer func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices)) }()
	var upstreamCreates, upstreamDials atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		upstreamDials.Add(1)
		defer ws.Close()
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var create struct{ Type, Model string }
			if common.Unmarshal(message, &create) != nil {
				return
			}
			index := upstreamCreates.Add(1)
			terminal := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_cmh_local_%d","status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`, index)
			if ws.WriteMessage(websocket.TextMessage, []byte(terminal)) != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	oldSetting, oldBase := channel.Setting, channel.BaseURL
	setting := `{"responses_websocket_enabled":true}`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"setting": setting, "base_url": upstream.URL}).Error)
	defer func() {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"setting": oldSetting, "base_url": oldBase, "status": common.ChannelStatusEnabled}).Error)
		model.InitChannelCache()
	}()
	model.InitChannelCache()
	done := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer client.Close()
		defer close(done)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		runner := func(request *http.Request, requestID string, handle func(*gin.Context) *kittypes.NewAPIError) *kittypes.NewAPIError {
			turn, _ := gin.CreateTestContext(httptest.NewRecorder())
			turn.Request = request
			common.SetContextKey(turn, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(turn, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(turn, constant.ContextKeyTokenGroup, "default")
			common.SetContextKey(turn, constant.ContextKeyUserId, 1)
			common.SetContextKey(turn, common.RequestIdKey, requestID)
			service.GetChannelConstraints(turn).AddPin(hostdto.ChannelPin{ChannelId: channel.Id, Source: hostdto.PinSourceToken, Rank: hostdto.PinRankToken, RetryMode: hostdto.PinRetrySingleAttempt})
			return handle(turn)
		}
		_ = relay.ResponsesWebSocketHelper(c, client, runner)
	}))
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http"), nil)
	require.NoError(t, err)
	defer func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("local WebSocket session did not stop")
		}
		gateway.Close()
	}()
	create := func() map[string]any {
		require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"public-chat","input":"local fixture"}`)))
		require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, wire, err := client.ReadMessage()
		require.NoError(t, err)
		var event map[string]any
		require.NoError(t, common.Unmarshal(wire, &event))
		return event
	}
	event := create()
	require.Equal(t, "response.completed", event["type"], "local fixture response: %v", event)
	assert.Equal(t, int32(1), upstreamCreates.Load())
	brokenSetting := `{"responses_websocket_enabled":`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("setting", brokenSetting).Error)
	event = create()
	assert.Equal(t, "error", event["type"])
	storedMalformed, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, storedMalformed.Setting)
	assert.Equal(t, brokenSetting, *storedMalformed.Setting, "denied restore is read-only")
	assert.Equal(t, int32(1), upstreamCreates.Load())
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("setting", setting).Error)
	brokenOther := `{"advanced_custom":`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("settings", brokenOther).Error)
	event = create()
	assert.Equal(t, "error", event["type"])
	storedMalformed, err = model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, brokenOther, storedMalformed.OtherSettings)
	assert.Equal(t, int32(1), upstreamCreates.Load())
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("settings", channel.OtherSettings).Error)
	originalMapping := channel.GetModelMapping()
	otherUpstream, err := New([]string{"approved-target,another-approved-target"}, nil)
	require.NoError(t, err)
	publicPolicy, err := New([]string{"public-chat,gpt-4o-mini,denied-only"}, []string{"1,14,61"})
	require.NoError(t, err)
	broaderGate, err := NewGate(publicPolicy, otherUpstream, nil)
	require.NoError(t, err)
	core.Models = broaderGate
	changedMapping := `{"public-chat":"another-approved-target"}`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", changedMapping).Error)
	event = create()
	assert.Equal(t, "error", event["type"])
	assert.Equal(t, int32(1), upstreamCreates.Load(), "reused socket cannot change its frozen target when DB maps to a different allowed model")
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", originalMapping).Error)
	core.Models = gate
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
	event = create()
	assert.Equal(t, "error", event["type"])
	assert.Equal(t, int32(1), upstreamCreates.Load(), "revoked second create cannot reach upstream")
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusEnabled).Error)
	event = create()
	assert.Equal(t, "response.completed", event["type"], "a denied create does not poison the reusable session")
	assert.Equal(t, int32(2), upstreamCreates.Load())
	// The realtime helper must inspect model fields omitted from typed DTOs,
	// without rewriting or accounting for rejected events.
	fresh, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=public-chat", nil)
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "public-chat")
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, fresh.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	info := &relaycommon.RelayInfo{OriginModelName: "public-chat", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelType: channel.Type, UpstreamModelName: "approved-target"}}
	require.Nil(t, openai.CheckRealtimeAdmission(c, info, []byte(`{"type":"response.create"}`)))
	require.NotNil(t, openai.CheckRealtimeAdmission(c, info, []byte(`{"type":"session.update","session":{"model":"unapproved-target"}}`)))
	require.NotNil(t, openai.CheckRealtimeAdmission(c, info, []byte(`{"type":"response.create","response":{"model":"unapproved-target"}}`)))
	require.NotNil(t, openai.CheckRealtimeAdmission(c, info, []byte(`{"type":"session.update","session":{"input_audio_transcription":{"model":"unapproved-transcriber"}}}`)))
	require.NotNil(t, openai.CheckRealtimeAdmission(c, info, []byte(`{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":"unapproved-transcriber"}}}}}`)))
	require.Nil(t, openai.CheckRealtimeAdmission(c, info, []byte(`{"type":"session.update","session":{"instructions":"ordinary update"}}`)))
	// WssHelper initializes the actual handshake identity from context before
	// dialing. This mapped public alias must be rejected before a new handshake,
	// rather than dial the original alias and only reject in the event handler.
	beforeDials := upstreamDials.Load()
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, fresh, "public-chat"))
	handshakeInfo := relaycommon.GenRelayInfoWs(c, nil)
	require.NotNil(t, relay.WssHelper(c, handshakeInfo))
	assert.Equal(t, beforeDials, upstreamDials.Load())
	core.Models = noop
	require.Nil(t, openai.CheckRealtimeAdmission(nil, nil, nil))
	core.Models = gate
}

func runHostAdmissionWrites(t *testing.T, gate *Gate, noop core.ModelGate) {
	t.Helper()
	approved, denied := `{"public-chat":"approved-target","gpt-4o-mini":"approved-target"}`, `{"public-chat":"unapproved-target"}`
	tag := "admission-write-fixture"
	makeChannel := func(id int, mapping *string) model.Channel {
		return model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "admission-write", Key: "fixture", Models: "public-chat", Group: "default", ModelMapping: mapping, Tag: &tag}
	}
	ids := []int{92001, 92002, 92003, 92004, 92005, 92006}
	defer func() {
		core.Models = noop
		require.NoError(t, model.DB.Where("channel_id IN ?", ids).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("id IN ?", ids).Delete(&model.Channel{}).Error)
		core.Models = gate
	}()
	good := makeChannel(92001, &approved)
	bad := makeChannel(92002, &denied)
	batch := []model.Channel{good, bad}
	require.Error(t, model.BatchInsertChannels(batch))
	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id IN ?", ids).Count(&count).Error)
	assert.Zero(t, count, "later denied batch row must leave no first channel")
	require.NoError(t, good.Insert())
	require.Error(t, bad.Insert())
	callbackName := "cmh_admission_fail_ability_create"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("fixture ability write failed"))
		}
	}))
	failedInsert := makeChannel(92004, &approved)
	insertErr := failedInsert.Insert()
	require.NoError(t, model.DB.Callback().Create().Remove(callbackName))
	require.Error(t, insertErr)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 92004).Count(&count).Error)
	assert.Zero(t, count, "failed ability creation must roll back its channel")
	patch := model.Channel{Id: good.Id, ModelMapping: &denied}
	require.Error(t, patch.Update())
	written, err := model.UpdateChannelOtherInfo(good.Id, func(ch *model.Channel) (map[string]any, error) {
		return map[string]any{"model_mapping": denied}, nil
	})
	require.Error(t, err)
	assert.False(t, written)
	written, err = model.UpdateChannelOtherInfo(good.Id, func(ch *model.Channel) (map[string]any, error) {
		*ch.ModelMapping = approved
		return map[string]any{"model_mapping": denied}, nil
	})
	require.Error(t, err, "callback in-memory allowed value cannot hide denied returned persistence column")
	assert.False(t, written)
	for _, updates := range []map[string]any{
		{"ModelMapping": denied},
		{"type": "1"},
		{"model_mapping": gorm.Expr("?", approved)},
		{"model_mapping": denied, "ModelMapping": approved},
	} {
		written, err = model.UpdateChannelOtherInfo(good.Id, func(ch *model.Channel) (map[string]any, error) { return updates, nil })
		require.Error(t, err)
		assert.False(t, written)
	}
	router := gin.New()
	router.POST("/add", func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleRootUser); c.Next() }, controller.AddChannel)
	addBody, err := common.Marshal(map[string]any{"mode": "batch", "channel": map[string]any{"type": constant.ChannelTypeOpenAI, "name": "admission-add-denied", "models": "not-published", "group": "default", "key": "first-fixture\nsecond-fixture"}})
	require.NoError(t, err)
	addResponse := httptest.NewRecorder()
	addRequest := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(string(addBody)))
	addRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(addResponse, addRequest)
	var addResult struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(addResponse.Body.Bytes(), &addResult))
	assert.False(t, addResult.Success)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("name = ?", "admission-add-denied").Count(&count).Error)
	assert.Zero(t, count)
	router.PUT("/channel", func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleRootUser); c.Next() }, controller.UpdateChannel)
	payload, err := common.Marshal(map[string]any{"id": good.Id, "model_mapping": denied, "key": "must-not-persist"})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/channel", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.False(t, result.Success)
	stored, err := model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, approved, stored.GetModelMapping(), "partial patch uses merged stored type/status/models")
	assert.Equal(t, "fixture", stored.Key, "controller admission must reject before the first key write")
	atomicCallback := "cmh_admission_fail_edit_abilities"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(atomicCallback, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("fixture edit ability failure"))
		}
	}))
	payload, err = common.Marshal(map[string]any{"id": good.Id, "model_mapping": approved, "name": "must-roll-back", "key": "must-roll-back-key"})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/channel", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.NoError(t, model.DB.Callback().Create().Remove(atomicCallback))
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.False(t, result.Success)
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "fixture", stored.Key, "late ability failure must roll back the controller key replacement")
	assert.NotEqual(t, "must-roll-back", stored.Name)
	core.Models = noop
	repair := makeChannel(92004, &denied)
	require.NoError(t, repair.Insert())
	core.Models = gate
	payload, err = common.Marshal(map[string]any{"id": repair.Id, "model_mapping": approved, "key": "repaired-key"})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/channel", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.True(t, result.Success, "repairing denied mapping and rotating key is one valid admitted edit")
	stored, err = model.GetChannelById(repair.Id, true)
	require.NoError(t, err)
	assert.Equal(t, approved, stored.GetModelMapping())
	assert.Equal(t, "repaired-key", stored.Key)
	core.Models = noop
	multi := makeChannel(92004, &approved)
	multi.Id = 92006
	multi.Key = "multi-fixture-a\nmulti-fixture-b"
	multi.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}
	require.NoError(t, multi.Insert())
	core.Models = gate
	multiPatch := model.Channel{Id: multi.Id}
	_, _, err = multiPatch.UpdateWithAdmissionKey(func(old string) (string, error) { return old + "\nmulti-fixture-c", nil })
	require.NoError(t, err)
	stored, err = model.GetChannelById(multi.Id, true)
	require.NoError(t, err)
	assert.True(t, stored.ChannelInfo.IsMultiKey)
	assert.Equal(t, 3, stored.ChannelInfo.MultiKeySize)
	patch = model.Channel{Id: good.Id, Name: "admission-renamed"}
	require.NoError(t, patch.Update())
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "admission-renamed", stored.Name)
	assert.Equal(t, approved, stored.GetModelMapping())
	revisionsBefore, err := model.GetLatestSyncRevisions()
	require.NoError(t, err)
	written, err = model.UpdateChannelOtherInfo(good.Id, func(ch *model.Channel) (map[string]any, error) {
		return map[string]any{"ModelMapping": approved + " "}, nil
	})
	require.NoError(t, err)
	assert.True(t, written)
	revisionsAfter, err := model.GetLatestSyncRevisions()
	require.NoError(t, err)
	assert.Greater(t, revisionsAfter[model.SyncTopicChannels], revisionsBefore[model.SyncTopicChannels], "mapping-only commit must publish channel synchronization")
	written, err = model.UpdateChannelOtherInfo(good.Id, func(ch *model.Channel) (map[string]any, error) { return map[string]any{"model_mapping": approved}, nil })
	require.NoError(t, err)
	assert.True(t, written)
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	pendingSettings := stored.GetOtherSettings()
	pendingSettings.UpstreamModelUpdateLastDetectedModels = []string{"not-published"}
	stored.SetOtherSettings(pendingSettings)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", good.Id).Update("settings", stored.OtherSettings).Error)
	router.POST("/apply", func(c *gin.Context) { c.Set("id", 1); c.Set("role", common.RoleRootUser); c.Next() }, controller.ApplyChannelUpstreamModelUpdates)
	applyBody, err := common.Marshal(map[string]any{"id": good.Id, "add_models": []string{"not-published"}})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader(string(applyBody)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	result.Success = true
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.False(t, result.Success)
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "public-chat", stored.Models)
	assert.Equal(t, []string{"not-published"}, stored.GetOtherSettings().UpstreamModelUpdateLastDetectedModels)
	pendingSettings = stored.GetOtherSettings()
	pendingSettings.UpstreamModelUpdateLastDetectedModels = []string{"gpt-4o-mini"}
	stored.SetOtherSettings(pendingSettings)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", good.Id).Update("settings", stored.OtherSettings).Error)
	newerSettings := pendingSettings
	newerSettings.UpstreamModelUpdateIgnoredModels = []string{"operator-new-ignore"}
	newerRaw, err := common.Marshal(newerSettings)
	require.NoError(t, err)
	interleaved := false
	var interleaveErr error
	queryCallback := "cmh_admission_interleave_ignore"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if !interleaved && tx.Statement.Table == "channels" {
			interleaved = true
			interleaveErr = model.DB.Model(&model.Channel{}).Where("id = ?", good.Id).Update("settings", string(newerRaw)).Error
		}
	}))
	applyBody, err = common.Marshal(map[string]any{"id": good.Id, "add_models": []string{"gpt-4o-mini"}})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader(string(applyBody)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.NoError(t, model.DB.Callback().Query().Remove(queryCallback))
	require.NoError(t, interleaveErr)
	require.True(t, interleaved)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.False(t, result.Success, "stale upstream selection must retry instead of overwriting newer settings")
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"operator-new-ignore"}, stored.GetOtherSettings().UpstreamModelUpdateIgnoredModels)
	assert.Equal(t, "public-chat", stored.Models)
	applyBody, err = common.Marshal(map[string]any{"id": good.Id, "add_models": []string{"gpt-4o-mini"}})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader(string(applyBody)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.True(t, result.Success, "approved delta uses current locked identity and succeeds")
	var addedAbility model.Ability
	require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", good.Id, "gpt-4o-mini").First(&addedAbility).Error)
	assert.True(t, addedAbility.Enabled)

	// Invalid old rows can remain disabled, but cannot be republished in a tag
	// batch. Seed through the explicit baseline/noop path to model existing data.
	core.Models = noop
	bad.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, bad.Insert())
	core.Models = gate
	require.Error(t, model.EnableChannelByTag(tag))
	stored, err = model.GetChannelById(bad.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	require.Error(t, model.EditChannelByTag(tag, nil, &denied, nil, nil, nil, nil, nil, nil))
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, approved, stored.GetModelMapping(), "tag veto preserves every matching row")
	assert.False(t, model.UpdateChannelStatus(bad.Id, "", common.ChannelStatusEnabled, "fixture enable"))
	_, err = model.UpdateChannelLocked(bad.Id, func(ch *model.Channel) error { ch.Status = common.ChannelStatusEnabled; ch.Key = "changed"; return nil })
	require.Error(t, err)
	stored, err = model.GetChannelById(bad.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "fixture", stored.Key, "credential callback cannot change key when enable is rejected")
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	_, err = model.UpdateScheduledChannelLocked(context.Background(), bad.Id, func(ch *model.Channel) error { ch.Status = common.ChannelStatusEnabled; ch.Key = "changed"; return nil })
	require.Error(t, err)
	stored, err = model.GetChannelById(bad.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "fixture", stored.Key)
	_, err = model.UpdateChannelLocked(bad.Id, func(ch *model.Channel) error {
		*ch.ModelMapping = approved
		ch.Status = common.ChannelStatusEnabled
		ch.Key = "changed"
		return nil
	})
	require.Error(t, err, "unwritten pointer mutations cannot fake the persisted admission identity")
	stored, err = model.GetChannelById(bad.Id, true)
	require.NoError(t, err)
	assert.Equal(t, denied, stored.GetModelMapping())
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	_, err = model.UpdateScheduledChannelLocked(context.Background(), bad.Id, func(ch *model.Channel) error {
		*ch.ModelMapping = approved
		ch.Status = common.ChannelStatusEnabled
		ch.Key = "changed"
		return nil
	})
	require.Error(t, err)
	stored, err = model.GetChannelById(bad.Id, true)
	require.NoError(t, err)
	assert.Equal(t, denied, stored.GetModelMapping())
	assert.Equal(t, "fixture", stored.Key)
	core.Models = noop
	pluginKey := `{"task_plugin_key":"cmh-unapproved"}`
	pluginChannel := makeChannel(92005, &approved)
	pluginChannel.Type, pluginChannel.Setting, pluginChannel.Status = constant.ChannelTypeTaskPlugin, &pluginKey, common.ChannelStatusManuallyDisabled
	require.NoError(t, pluginChannel.Insert())
	core.Models = gate
	err = model.UpdateTaskPluginChannelLocked(pluginChannel.Id, func(ch *model.Channel, _ []*model.Channel) error {
		*ch.Setting = `{}`
		ch.Type = constant.ChannelTypeOpenAI
		ch.Status = common.ChannelStatusEnabled
		ch.Key = "changed"
		return nil
	})
	require.Error(t, err)
	stored, err = model.GetChannelById(pluginChannel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, pluginKey, *stored.Setting)
	assert.Equal(t, "fixture", stored.Key)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	newTag, newGroup := "admission-moved", "admission-group"
	priority, weight := int64(8), uint(3)
	require.NoError(t, model.EditChannelByTag(tag, &newTag, &approved, nil, &newGroup, &priority, &weight, nil, nil))
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, newTag, stored.GetTag())
	assert.Equal(t, newGroup, stored.Group)
	var moved model.Ability
	require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", good.Id, "public-chat").First(&moved).Error)
	assert.Equal(t, newGroup, moved.Group)
	require.NotNil(t, moved.Tag)
	assert.Equal(t, newTag, *moved.Tag)
	require.NotNil(t, moved.Priority)
	assert.Equal(t, priority, *moved.Priority)
	assert.Equal(t, weight, moved.Weight)
	core.Models = noop
	quarantine := makeChannel(92003, &denied)
	quarantine.Key = "fixture-a\nfixture-b"
	quarantine.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}
	require.NoError(t, quarantine.Insert())
	core.Models = gate
	require.True(t, model.UpdateChannelStatus(quarantine.Id, "fixture-a", common.ChannelStatusAutoDisabled, "fixture quarantine"))
	stored, err = model.GetChannelById(quarantine.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status, "remaining key stays enabled but new execution is independently denied")
	written, err = model.UpdateChannelOtherInfo(quarantine.Id, func(ch *model.Channel) (map[string]any, error) {
		delete(ch.ChannelInfo.MultiKeyStatusList, 0)
		return map[string]any{"channel_info": ch.ChannelInfo}, nil
	})
	require.Error(t, err, "restoring a disabled key cannot receive a restrictive-write exemption")
	assert.False(t, written)
	stored, err = model.GetChannelById(quarantine.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.False(t, model.UpdateChannelStatus(quarantine.Id, "fixture-a", common.ChannelStatusEnabled, "fixture re-enable"), "quarantine cannot be lifted without admission")
	// Explicit disable/cleanup cannot be blocked by a model that is no longer authorized.
	patch = model.Channel{Id: good.Id, Status: common.ChannelStatusManuallyDisabled, ModelMapping: &denied}
	require.NoError(t, patch.Update())
}

func TestHostAdmissionCandidateAndCatalog(t *testing.T) {
	engines := []struct{ name, dsn string }{
		{"sqlite", ""},
		{"mysql", os.Getenv("CMH_TEST_MYSQL_DSN")},
		{"postgres", os.Getenv("CMH_TEST_POSTGRES_DSN")},
	}
	for _, engine := range engines {
		t.Run(engine.name, func(t *testing.T) {
			if engine.name != "sqlite" && engine.dsn == "" {
				t.Skip("explicit local test DSN not configured")
			}
			t.Setenv("SQL_DSN", engine.dsn)
			t.Setenv("LOG_SQL_DSN", "")
			t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "admission.db"))
			runHostAdmissionCandidateAndCatalog(t)
		})
	}
}

// Embedding delegates the other ModelGate methods without duplicating policy.
type hostPreselectionWitnessGate struct {
	core.ModelGate
	channelID  int
	candidates []core.Admission
}

func (gate *hostPreselectionWitnessGate) Check(ctx context.Context, admission core.Admission) error {
	if admission.Phase == core.AdmissionCandidate && admission.Channel.ID == gate.channelID {
		gate.candidates = append(gate.candidates, admission)
	}
	return gate.ModelGate.Check(ctx, admission)
}

type hostPreselectionHookProbe struct {
	core.PluginHookGate
	calls []core.PluginHookIdentity
}

func (probe *hostPreselectionHookProbe) CheckHook(ctx context.Context, identity core.PluginHookIdentity) error {
	probe.calls = append(probe.calls, identity)
	return probe.PluginHookGate.CheckHook(ctx, identity)
}

func runHostSharedEndpointPreselection(t *testing.T, noop core.ModelGate) {
	t.Helper()
	require.NoError(t, i18n.Init())
	oldGate, oldRegistry, oldHooks := core.Models, jsplugin.DefaultRegistry, core.PluginHooks
	oldMemory := common.MemoryCacheEnabled
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	common.MemoryCacheEnabled = false
	defer func() {
		core.Models, jsplugin.DefaultRegistry, core.PluginHooks = oldGate, oldRegistry, oldHooks
		common.MemoryCacheEnabled = oldMemory
	}()

	var networkCalls atomic.Int32
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer stub.Close()

	const path = "/v1/images/generations"
	const sourceA = `export const meta={apiVersion:1,key:"cmh-preselection",name:"Preselection fixture",version:"1.0.0",author:{name:"Test"},models:["preselect-model"],fetchMode:"per_task",protocols:["openai_image"]};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit"};}
export function parseSubmitResponse(){return {taskId:"local"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}
export function listArtifacts(){return [];}
export function buildContentRequest(){throw new Error("artifact_not_found");}
export const protocols={openai_image:{decodeRequest:function(ctx){return {kind:"submit",model:ctx.model,requestBody:{decodeMarker:"A"}};},render:function(){return {data:[]};}}};`
	pluginA, err := registry.RegisterFactory(sourceA, jsplugin.Options{})
	require.NoError(t, err)
	generationA := registry.Generation()
	pluginB, err := jsplugin.CompilePlugin(strings.Replace(sourceA, `decodeMarker:"A"`, `decodeMarker:"B"`, 1), jsplugin.Options{})
	require.NoError(t, err)
	require.NotEqual(t, pluginA.SourceSHA256(), pluginB.SourceSHA256())

	setting, base := `{"task_plugin_key":"cmh-preselection"}`, stub.URL
	channel := model.Channel{Id: 95010, Name: "preselection", Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "preselect-model", Group: "default", Key: "fixture", BaseURL: &base, Setting: &setting}
	core.Models = noop
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(model.DB))
	defer func() {
		core.Models = noop
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("id = ?", channel.Id).Delete(&model.Channel{}).Error)
		model.InitChannelCache()
	}()

	public, err := New([]string{"preselect-model"}, []string{"61"})
	require.NoError(t, err)
	upstream, err := New([]string{"preselect-model"}, nil)
	require.NoError(t, err)
	identityA, err := model.PluginExecutorIdentity(&channel, pluginA, generationA.Number, "task", path)
	require.NoError(t, err)
	gateA, err := NewGate(public, upstream, []core.ExecutorIdentity{identityA})
	require.NoError(t, err)
	// The artifact is unpublished here. Only its exact source/config is used to
	// construct the negative policy; generation numbers are not approval.
	identityB, err := model.PluginExecutorIdentity(&channel, pluginB, generationA.Number, "task", path)
	require.NoError(t, err)
	gateB, err := NewGate(public, upstream, []core.ExecutorIdentity{identityB})
	require.NoError(t, err)

	for _, test := range []struct {
		name        string
		policy      *Gate
		wantHandler bool
	}{
		{name: "approved_A_survives_registry_B", policy: gateA, wantHandler: true},
		{name: "B_approval_cannot_authorize_pinned_A", policy: gateB},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Restore the factory A before each request. Each route then captures
			// its current A generation; B is published during the abilities query.
			require.NoError(t, registry.ReplaceOverrides(nil))
			requestGenerationA := registry.Generation()
			witness := &hostPreselectionWitnessGate{ModelGate: test.policy, channelID: channel.Id}
			hooks := &hostPreselectionHookProbe{PluginHookGate: oldHooks}
			core.Models, core.PluginHooks = witness, hooks
			probe := &admissionBillingProbe{}
			changed := false
			const callbackName = "cmh:test_preselection_swap"
			require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if changed || tx.Statement.Table != "abilities" {
					return
				}
				changed = true
				// No DB call, goroutine or sleep occurs inside this callback.
				require.NoError(t, registry.ReplaceOverrides([]*jsplugin.LoadedPlugin{pluginB}))
			}))
			defer func() { require.NoError(t, model.DB.Callback().Query().Remove(callbackName)) }()

			reached := false
			router := gin.New()
			router.POST(path, func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserId, 7)
			}, middleware.RouteTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), middleware.Distribute(), func(c *gin.Context) {
				reached = true
				if !test.wantHandler {
					// This is a tripwire, not real funding: the rejected case must
					// never reach the place where a relay would start reserving.
					require.NoError(t, probe.Reserve(1))
					return
				}
				value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
				require.True(t, exists)
				pinned, ok := value.(jsplugin.PinnedPlugin)
				require.True(t, ok)
				assert.Same(t, pluginA, pinned.Plugin)
				assert.Same(t, requestGenerationA, pinned.Generation)
				assert.Equal(t, map[string]any{"decodeMarker": "A"}, c.MustGet("task_request"))
				for _, filter := range service.GetChannelConstraints(c).Filters {
					assert.NotNil(t, filter.PluginView)
				}
				assert.Equal(t, channel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"preselect-model","prompt":"fixture"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			require.True(t, changed, "the real abilities DB query must hit the replacement seam")
			require.NotEmpty(t, witness.candidates)
			assert.Equal(t, pluginA.SourceSHA256(), witness.candidates[0].Executor.SourceSHA256, "the FIRST preselection witness must be A; fallback final success alone is insufficient")
			for _, candidate := range witness.candidates {
				assert.Equal(t, pluginA.SourceSHA256(), candidate.Executor.SourceSHA256)
			}
			for _, hook := range hooks.calls {
				assert.NotEqual(t, pluginB.SourceSHA256(), hook.SourceSHA256, "B's decoder must never run in this A-pinned request")
			}
			assert.Equal(t, test.wantHandler, reached)
			if test.wantHandler {
				assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
			} else {
				assert.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
			}
			assert.Zero(t, probe.reserve)
			assert.Zero(t, probe.settle)
			assert.Zero(t, probe.refund)
			assert.Zero(t, networkCalls.Load())
		})
	}

	// Direct Pin has no taskPluginEndpointSelection. Captured context A must
	// take precedence over current B, but selected.Generation remains highest
	// precedence in the production implementation.
	require.NoError(t, registry.ReplaceOverrides([]*jsplugin.LoadedPlugin{pluginB}))
	core.Models, core.PluginHooks = gateA, oldHooks
	directReached := false
	direct := gin.New()
	direct.POST(path, func(c *gin.Context) {
		c.Set("cmh_candidate_generation", generationA)
	}, middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), func(c *gin.Context) {
		directReached = true
		value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
		require.True(t, exists)
		pinned, ok := value.(jsplugin.PinnedPlugin)
		require.True(t, ok)
		assert.Same(t, pluginA, pinned.Plugin)
		assert.Same(t, generationA, pinned.Generation)
		assert.Equal(t, map[string]any{"decodeMarker": "A"}, c.MustGet("task_request"))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"preselect-model","prompt":"fixture"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	direct.ServeHTTP(recorder, request)
	assert.True(t, directReached)
	assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Zero(t, networkCalls.Load())
}

func runHostContinuationCredentialRefresh(t *testing.T, noop core.ModelGate) {
	t.Helper()
	oldGate, oldRegistry, oldHooks := core.Models, jsplugin.DefaultRegistry, core.PluginHooks
	oldMemory := common.MemoryCacheEnabled
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry, core.Models, common.MemoryCacheEnabled = registry, noop, false
	defer func() {
		core.Models, jsplugin.DefaultRegistry, core.PluginHooks = oldGate, oldRegistry, oldHooks
		common.MemoryCacheEnabled = oldMemory
	}()
	var networkCalls atomic.Int32
	var driftDuringHook atomic.Bool
	var failRefresh atomic.Bool
	updates := make(chan error, 1)
	const channelID = 95011
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkCalls.Add(1)
		if driftDuringHook.Load() {
			updates <- model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("settings", `{"plugin_config":{"tier":"B"}}`).Error
		}
		w.Header().Set("Content-Type", "application/json")
		if failRefresh.Load() {
			_, _ = w.Write([]byte(`{"fail":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer stub.Close()
	const source = `export const meta={apiVersion:1,key:"cmh-cont-refresh",name:"Continuation refresh",version:"1.0.0",author:{name:"Test"},models:["refresh-model"],fetchMode:"per_task",channelConfig:{type:"object",properties:{tier:{type:"string",title:{en:"Tier"}}}}};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit"};}
export function parseSubmitResponse(){return {taskId:"local"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}
export function login(){return {type:"done",credential:{marker:"initial"},accountId:"refresh-account"};}
export function refresh(ctx){const response=utils.fetch({url:ctx.baseUrl+"/refresh",method:"POST",body:"{}"});if(response.body.fail)throw new Error("temporary fixture refresh failure");return {credential:{marker:"A",tier:ctx.config.tier},headers:{Authorization:"Bearer refreshed-A"},expiresAt:utils.unixNow()+3600,accountId:"refresh-account"};}`
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	config := common.RawMessage(`{"tier":"A"}`)
	credential := jsplugin.Credential{Plugin: plugin.Meta.Key, Data: common.RawMessage(`{"marker":"initial"}`), AccountID: "refresh-account", ExpiresAt: time.Now().Add(20 * time.Second).Unix()}
	line, err := credential.Line()
	require.NoError(t, err)
	setting, base := `{"task_plugin_key":"cmh-cont-refresh"}`, stub.URL
	channel := model.Channel{Id: channelID, Type: constant.ChannelTypeTaskPlugin, Name: "continuation-refresh", Status: common.ChannelStatusEnabled, Models: "refresh-model", Group: "default", BaseURL: &base, Setting: &setting, OtherSettings: `{"plugin_config":{"tier":"A"}}`, Key: line}
	require.NoError(t, model.DB.Create(&channel).Error)
	defer func() {
		require.NoError(t, model.DB.Where("id = ?", channelID).Delete(&model.Channel{}).Error)
		model.InitChannelCache()
	}()
	hooks := &hostPreselectionHookProbe{PluginHookGate: oldHooks}
	core.PluginHooks = hooks
	call := func(ctx context.Context) (string, error) {
		return service.FreshPluginCredentialLineForContinuation(ctx, plugin, channelID, 0, line, base, "", config)
	}
	reset := func() {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Updates(map[string]any{"key": line, "settings": channel.OtherSettings, "setting": setting, "base_url": base}).Error)
		hooks.calls = nil
		networkCalls.Store(0)
	}
	key := func() string {
		stored, err := model.GetChannelById(channelID, true)
		require.NoError(t, err)
		return stored.Key
	}
	fresh, err := call(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, line, fresh)
	parsed, ok := jsplugin.ParseCredential(fresh)
	require.True(t, ok)
	assert.JSONEq(t, `{"marker":"A","tier":"A"}`, string(parsed.Data))
	assert.Equal(t, fresh, key())
	assert.Equal(t, int32(1), networkCalls.Load())
	require.Len(t, hooks.calls, 1)
	assert.Equal(t, "refresh", hooks.calls[0].Export)
	reused, err := call(t.Context())
	require.NoError(t, err)
	assert.Equal(t, fresh, reused)
	assert.Equal(t, int32(1), networkCalls.Load())
	assert.Len(t, hooks.calls, 1)

	info := &relaycommon.RelayInfo{OriginModelName: "refresh-model", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID, ChannelType: constant.ChannelTypeTaskPlugin, ChannelBaseUrl: base, ChannelSetting: kitdto.ChannelSettings{TaskPluginKey: plugin.Meta.Key}, ChannelOtherSettings: kitdto.ChannelOtherSettings{PluginConfig: config}, UpstreamModelName: "refresh-model"}}
	original := taskjsplugin.New(plugin)
	original.Init(info)
	artifact, err := original.CaptureContinuationArtifact(info)
	require.NoError(t, err)
	task := &model.Task{TaskID: "synthetic-cont-refresh", UserId: 7, ChannelId: channelID, Platform: constant.TaskPlatform(plugin.Meta.Key), Properties: model.Properties{OriginModelName: "refresh-model", UpstreamModelName: "refresh-model"}, PrivateData: model.TaskPrivateData{CredentialAccount: "refresh-account", Execution: &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: plugin.Meta.Key}, Continuation: &core.ContinuationRef{ID: "fixture-ref", Digest: "fixture-digest"}}}}
	reset()
	restored, err := taskjsplugin.RestoreContinuationArtifact(artifact)
	require.NoError(t, err)
	require.NoError(t, restored.BindContinuationTask(task, core.ContinuationContent))
	require.NoError(t, restored.AttachContinuationCredentials(t.Context(), task, &channel))
	assert.NotEqual(t, line, key())
	assert.Equal(t, int32(1), networkCalls.Load())

	for _, test := range []struct {
		name  string
		drift func()
	}{
		{name: "source_same_key_version", drift: func() {
			_, err := registry.RegisterFactory(source+"\nexport const sourceRevision=2;", jsplugin.Options{})
			require.NoError(t, err)
		}},
		{name: "effective_config", drift: func() {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("settings", `{"plugin_config":{"tier":"B"}}`).Error)
		}},
		{name: "base_url", drift: func() {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("base_url", base+"/changed").Error)
		}},
		{name: "proxy", drift: func() {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("setting", `{"task_plugin_key":"cmh-cont-refresh","proxy":"http://127.0.0.1:1"}`).Error)
		}},
	} {
		t.Run("continuation_refresh_"+test.name, func(t *testing.T) {
			_, err := registry.RegisterFactory(source, jsplugin.Options{})
			require.NoError(t, err)
			reset()
			test.drift()
			result, err := call(t.Context())
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			assert.Empty(t, result)
			assert.Equal(t, line, key())
			assert.Empty(t, hooks.calls)
			assert.Zero(t, networkCalls.Load())
			frozen, err := taskjsplugin.RestoreContinuationArtifact(artifact)
			require.NoError(t, err)
			require.NoError(t, frozen.BindContinuationTask(task, core.ContinuationContent))
			require.ErrorIs(t, frozen.AttachContinuationCredentials(t.Context(), task, &channel), core.ErrContinuationUnavailable)
			assert.Equal(t, line, key())
			assert.Empty(t, hooks.calls)
			assert.Zero(t, networkCalls.Load())
			freshCredential := credential
			freshCredential.ExpiresAt = time.Now().Add(time.Hour).Unix()
			freshLine, err := freshCredential.Line()
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("key", freshLine).Error)
			result, err = service.FreshPluginCredentialLineForContinuation(t.Context(), plugin, channelID, 0, freshLine, base, "", config)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			assert.Empty(t, result)
			assert.Equal(t, freshLine, key())
			assert.Empty(t, hooks.calls)
			assert.Zero(t, networkCalls.Load())
		})
	}
	_, err = registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	reset()
	substituted := credential
	substituted.AccountID = ""
	substituted.ExpiresAt = time.Now().Add(time.Hour).Unix()
	substitutedLine, err := substituted.Line()
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("key", substitutedLine).Error)
	result, err := call(t.Context())
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	assert.Empty(t, result)
	assert.Equal(t, substitutedLine, key())
	assert.Empty(t, hooks.calls)
	assert.Zero(t, networkCalls.Load())
	for _, invalid := range []jsplugin.Credential{
		{Plugin: plugin.Meta.Key, Data: common.RawMessage(`{}`), AccountID: credential.AccountID, ExpiresAt: time.Now().Add(-time.Hour).Unix()},
		{Plugin: plugin.Meta.Key, Data: common.RawMessage(`{}`), AccountID: credential.AccountID, NeedsLogin: true},
	} {
		reset()
		invalidLine, err := invalid.Line()
		require.NoError(t, err)
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("key", invalidLine).Error)
		result, err := call(t.Context())
		require.ErrorIs(t, err, core.ErrContinuationUnavailable)
		assert.Empty(t, result)
		assert.Empty(t, hooks.calls)
		assert.Zero(t, networkCalls.Load())
		assert.Equal(t, invalidLine, key())
	}
	for _, account := range []string{"different-account", ""} {
		reset()
		changedAccount := credential
		changedAccount.AccountID = account
		changedLine, err := changedAccount.Line()
		require.NoError(t, err)
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("key", changedLine).Error)
		result, err := service.FreshPluginCredentialLineForContinuation(t.Context(), plugin, channelID, 0, changedLine, base, "", config)
		require.ErrorIs(t, err, core.ErrContinuationUnavailable)
		assert.Empty(t, result)
		assert.Equal(t, changedLine, key())
		assert.Equal(t, int32(1), networkCalls.Load())
		assert.Len(t, hooks.calls, 1)
	}
	reset()
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = call(cancelled)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, result)
	assert.Empty(t, hooks.calls)
	assert.Zero(t, networkCalls.Load())
	assert.Equal(t, line, key())
	driftDuringHook.Store(true)
	result, err = call(t.Context())
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	assert.Empty(t, result)
	require.NoError(t, <-updates)
	assert.Equal(t, line, key())
	require.Len(t, hooks.calls, 1)
	assert.Equal(t, int32(1), networkCalls.Load(), "the completed vendor action cannot be rolled back, but its stale credential must not persist")
	reset()
	failRefresh.Store(true)
	result, err = call(t.Context())
	require.ErrorIs(t, err, core.ErrContinuationUnavailable)
	assert.Empty(t, result)
	require.NoError(t, <-updates)
	assert.Equal(t, line, key())
	assert.Len(t, hooks.calls, 1)
	assert.Equal(t, int32(1), networkCalls.Load())
}

func runHostPluginPublication(t *testing.T, noop core.ModelGate) {
	t.Helper()
	oldGate, oldRegistry := core.Models, jsplugin.DefaultRegistry
	registry := jsplugin.NewRegistry()
	jsplugin.DefaultRegistry = registry
	defer func() { core.Models = oldGate; jsplugin.DefaultRegistry = oldRegistry }()
	const source = `export const meta={apiVersion:1,key:"cmh-publication",name:"Publication",version:"1.0.0",author:{name:"Test"},models:["published-task"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit"};}
export function parseSubmitResponse(){return {taskId:"local"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}
export function parseTaskResult(){return {status:"IN_PROGRESS"};}`
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	setting, base := `{"task_plugin_key":"cmh-publication"}`, "https://fixture.invalid"
	channel := model.Channel{Id: 95009, Name: "publication", Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: "published-task", Group: "default", Key: "fixture", BaseURL: &base, Setting: &setting}
	public, err := New([]string{"published-task"}, []string{"61"})
	require.NoError(t, err)
	upstream, err := New([]string{"published-task"}, nil)
	require.NoError(t, err)
	runtime, err := model.PluginExecutorIdentity(&channel, plugin, registry.Generation().Number, "task", "/v1/tasks/cmh-publication")
	require.NoError(t, err)
	runtimeOnly, err := NewGate(public, upstream, []core.ExecutorIdentity{runtime})
	require.NoError(t, err)
	core.Models = runtimeOnly
	assert.Error(t, model.ValidateChannelAdmissionSnapshot(t.Context(), &channel), "runtime approval cannot authorize publication")
	publication, err := model.PluginExecutorIdentity(&channel, plugin, registry.Generation().Number, "publication", "/v1/models")
	require.NoError(t, err)
	_, err = model.PluginExecutorIdentity(&channel, plugin, registry.Generation().Number, "publication", "/v1/tasks/cmh-publication")
	require.Error(t, err)
	_, err = model.ChannelPluginAdmission(&channel, "published-task", core.AdmissionDispatch, jsplugin.PinnedPlugin{Plugin: plugin, Generation: registry.Generation()}, "publication", "/v1/models")
	require.Error(t, err)
	publicationGate, err := NewGate(public, upstream, []core.ExecutorIdentity{publication})
	require.NoError(t, err)
	core.Models = publicationGate
	require.NoError(t, channel.Insert())
	defer func() {
		core.Models = noop
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Where("id = ?", channel.Id).Delete(&model.Channel{}).Error)
		model.InitChannelCache()
	}()
	assert.Contains(t, model.GetGroupEnabledModels("default"), "published-task")
	actual, err := model.ChannelPluginAdmission(&channel, "published-task", core.AdmissionDispatch, jsplugin.PinnedPlugin{Plugin: plugin, Generation: registry.Generation()}, "task", "/v1/tasks/cmh-publication")
	require.NoError(t, err)
	assert.Error(t, publicationGate.Check(t.Context(), actual), "publication approval cannot authorize task dispatch")
	pinnedGeneration := registry.Generation()
	_, err = registry.RegisterFactory(source+"\nexport const revision=2;", jsplugin.Options{})
	require.NoError(t, err)
	assert.NotContains(t, model.GetGroupEnabledModels("default"), "published-task", "source replacement revokes exact publication match")
	assert.Error(t, model.ValidateChannelAdmissionSnapshot(t.Context(), &channel))
	core.Models = runtimeOnly
	filter := hostdto.ChannelFilter{Kind: hostdto.FilterTaskPluginIdentity, TaskPluginKey: plugin.Meta.Key, RequestPath: "/v1/tasks/cmh-publication", PluginView: model.NewPluginCandidateView(pinnedGeneration)}
	currentFilter := filter
	currentFilter.PluginView = nil
	ok, _ := model.ChannelSatisfiesFilters(&channel, "published-task", []hostdto.ChannelFilter{currentFilter})
	assert.False(t, ok, "current source B has no runtime approval")
	ok, _ = model.ChannelSatisfiesFilters(&channel, "published-task", []hostdto.ChannelFilter{filter})
	assert.True(t, ok, "pinned source A must not be replaced by current generation")
	conflict := filter
	conflict.PluginView = model.NewPluginCandidateView(registry.Generation())
	ok, _ = model.ChannelSatisfiesFilters(&channel, "published-task", []hostdto.ChannelFilter{filter, conflict})
	assert.False(t, ok)
	emptyViewFilter := filter
	emptyViewFilter.PluginView = model.NewPluginCandidateView(jsplugin.NewRegistry().Generation())
	ok, _ = model.ChannelSatisfiesFilters(&channel, "published-task", []hostdto.ChannelFilter{emptyViewFilter})
	assert.False(t, ok)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/tasks/cmh-publication", nil)
	ctx.Set("expected_task_plugin_key", plugin.Meta.Key)
	ctx.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Plugin: plugin, Generation: pinnedGeneration})
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	service.GetChannelConstraints(ctx).AddFilter(filter)
	require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, &channel, "published-task"))
	captured, _ := ctx.Get("cmh_candidate_generation")
	assert.Same(t, pinnedGeneration, captured)
	oldMemory := common.MemoryCacheEnabled
	defer func() { common.MemoryCacheEnabled = oldMemory }()
	for _, memory := range []bool{false, true} {
		common.MemoryCacheEnabled = memory
		model.InitChannelCache()
		selected, err := model.GetRandomSatisfiedChannel("default", "published-task", 0, []hostdto.ChannelFilter{filter})
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, channel.Id, selected.Id)
	}
	const providerSource = `export const meta={apiVersion:1,key:"cmh-view-provider",name:"View provider",version:"1.0.0",author:{name:"Test"},relay:[{incoming_path:"/v1/chat/completions",upstream_path:"/chat"}]};`
	providerPlugin, err := registry.RegisterFactory(providerSource, jsplugin.Options{})
	require.NoError(t, err)
	providerGeneration := registry.Generation()
	providerSetting := `{"task_plugin_key":"cmh-view-provider"}`
	providerChannel := channel
	providerChannel.Setting = &providerSetting
	providerFilter := hostdto.ChannelFilter{Kind: hostdto.FilterTaskPluginIdentity, RequestPath: "/v1/chat/completions", PluginView: model.NewPluginCandidateView(providerGeneration)}
	_, err = registry.RegisterFactory(strings.Replace(providerSource, "/v1/chat/completions", "/v1/embeddings", 1), jsplugin.Options{})
	require.NoError(t, err)
	ok, _ = model.ChannelSatisfiesHostFilters(&providerChannel, "published-task", []hostdto.ChannelFilter{providerFilter})
	assert.True(t, ok)
	providerFilter.PluginView = nil
	ok, _ = model.ChannelSatisfiesHostFilters(&providerChannel, "published-task", []hostdto.ChannelFilter{providerFilter})
	assert.False(t, ok)
	assert.NotEmpty(t, providerPlugin.SourceSHA256())
}

func runHostAdmissionCandidateAndCatalog(t *testing.T) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldMaster, oldMemory := common.IsMasterNode, common.MemoryCacheEnabled
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	oldSQLite := common.SQLitePath
	common.SQLitePath = os.Getenv("SQLITE_PATH")
	oldGate := core.Models
	common.IsMasterNode = true
	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	db := model.DB
	var version string
	switch common.MainDatabaseType() {
	case common.DatabaseTypeSQLite:
		require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
	default:
		require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
	}
	t.Logf("real admission database: %s %s", common.MainDatabaseType(), version)
	t.Cleanup(func() {
		core.Models = oldGate
		if sqlDB, err := db.DB(); err == nil {
			require.NoError(t, sqlDB.Close())
		}
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.IsMasterNode, common.MemoryCacheEnabled = oldMaster, oldMemory
		common.RedisEnabled = oldRedis
		common.SQLitePath = oldSQLite
	})

	public, err := New([]string{"gpt-4o-mini,public-chat,denied-only"}, []string{"1,14,61"})
	require.NoError(t, err)
	upstream, err := New([]string{"approved-target"}, nil)
	require.NoError(t, err)
	gate, err := NewGate(public, upstream, nil)
	require.NoError(t, err)
	approved, denied := `{"gpt-4o-mini":"approved-target","public-chat":"approved-target"}`, `{"gpt-4o-mini":"unapproved-target","public-chat":"unapproved-target"}`
	pluginSetting := `{"task_plugin_key":"cmh-unresolved"}`
	high, low := int64(100), int64(1)
	channels := []model.Channel{
		{Id: 91001, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "denied-high", Key: "fixture", Models: "gpt-4o-mini,public-chat", Group: "default", ModelMapping: &denied, Priority: &high},
		{Id: 91002, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "allowed-low", Key: "fixture", Models: "gpt-4o-mini,public-chat", Group: "default", ModelMapping: &approved, Priority: &low},
		{Id: 91003, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: "unresolved-plugin", Key: "fixture", Models: "public-chat", Group: "default", ModelMapping: &approved, Setting: &pluginSetting},
		{Id: 91004, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Name: "disabled-stale-ability", Key: "fixture", Models: "public-chat", Group: "default", ModelMapping: &approved, Priority: &high},
		{Id: 91005, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Name: "denied-owner", Key: "fixture", Models: "gpt-4o-mini,denied-only", Group: "default", ModelMapping: &denied, Priority: &high},
	}
	vendors := []model.Vendor{{Id: 93001, Name: "admission-allowed", Status: 1}, {Id: 93002, Name: "admission-denied", Status: 1}, {Id: 93003, Name: "admission-unrelated", Status: 1}}
	require.NoError(t, db.Create(&vendors).Error)
	metadata := []model.Model{
		{Id: 93001, ModelName: "public-chat", Status: 1, VendorID: 93001, Endpoints: `{"allowed_endpoint":{"path":"/allowed","method":"POST"}}`},
		{Id: 93002, ModelName: "gpt-4o-mini", Status: 1, VendorID: 93001},
		{Id: 93003, ModelName: "denied-only", Status: 1, VendorID: 93002, Endpoints: `{"denied_endpoint":{"path":"/denied","method":"POST"}}`},
		{Id: 93004, ModelName: "unrelated-metadata", Status: 1, VendorID: 93003, Endpoints: `{"ghost_endpoint":{"path":"/ghost","method":"POST"}}`},
	}
	require.NoError(t, db.Create(&metadata).Error)
	for i := range channels {
		require.NoError(t, db.Create(&channels[i]).Error)
		require.NoError(t, channels[i].AddAbilities(db))
	}
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 91004).Update("enabled", true).Error)
	core.Models = gate
	require.True(t, core.ModelAdmissionEnabled())
	for _, memory := range []bool{false, true} {
		common.MemoryCacheEnabled = memory
		model.InitChannelCache()
		selected, err := model.GetRandomSatisfiedChannel("default", "public-chat", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, 91002, selected.Id, "denied high priority must not starve permitted fallback; cache=%v", memory)
		ok, _ := model.ChannelSatisfiesFilters(&channels[0], "public-chat", nil)
		assert.False(t, ok, "pinned predicate must reject even with empty host filters")
		ok, _ = model.ChannelSatisfiesFilters(&channels[3], "public-chat", nil)
		assert.False(t, ok, "stale enabled ability cannot authorize a disabled channel")
		ok, _ = model.ChannelSatisfiesFilters(&channels[2], "public-chat", nil)
		assert.False(t, ok, "unresolved plugin executor must not masquerade as builtin")
		legacyFilters := []hostdto.ChannelFilter{{Kind: hostdto.FilterTaskPluginIdentity, TaskPluginKey: "sora", TaskPluginChannelTypes: []int{constant.ChannelTypeOpenAI}}}
		projection := model.ChannelModelAdmission(&channels[1], "public-chat", core.AdmissionCandidate, legacyFilters...)
		assert.Empty(t, projection.Executor.Kind, "pinned legacy plugin must not become builtin because the model is undeclared")
		ok, _ = model.ChannelSatisfiesFilters(&channels[1], "public-chat", legacyFilters)
		assert.False(t, ok)
		selected, err = model.GetRandomSatisfiedChannel("default", "public-chat", 0, legacyFilters)
		require.NoError(t, err)
		assert.Nil(t, selected, "both DB and memory candidate paths reject unattested pinned executor")
	}
	assert.ElementsMatch(t, []string{"gpt-4o-mini", "public-chat"}, model.GetGroupEnabledModels("default"))
	assert.True(t, model.IsModelAdmittedForPublication("gpt-4o-mini", constant.ChannelTypeOpenAI))
	assert.False(t, model.IsModelAdmittedForPublication("gpt-4o-mini", constant.ChannelTypeAnthropic))
	assert.False(t, model.IsModelAdmittedForPublication("not-published"))
	runHostPluginPublication(t, oldGate)
	runHostSharedEndpointPreselection(t, oldGate)
	runHostContinuationCredentialRefresh(t, oldGate)
	runHostAdmissionRequestBoundary(t, channels, gate, oldGate)
	runHostBoundProviderAdmission(t, gate, oldGate)
	runHostProviderChainAdmission(t, gate, oldGate)
	runHostTaskDriverAdmission(t, gate, oldGate)
	runHostAdmissionSockets(t, channels[1], gate, oldGate)
	runHostAdmissionWrites(t, gate, oldGate)
	owners, err := model.GetPreferredModelOwnerChannelTypes([]string{"gpt-4o-mini"}, []string{"default"})
	require.NoError(t, err)
	assert.Equal(t, constant.ChannelTypeOpenAI, owners["gpt-4o-mini"], "denied high-priority provider cannot own the public projection")
	model.InvalidatePricingCache()
	pricing := model.GetPricing()
	pricingNames := make([]string, 0, len(pricing))
	for _, row := range pricing {
		pricingNames = append(pricingNames, row.ModelName)
		assert.Empty(t, row.BillingPluginVariants, "unattested plugin variants cannot augment builtin pricing")
	}
	assert.ElementsMatch(t, []string{"gpt-4o-mini", "public-chat"}, pricingNames)
	visibleVendors := model.GetVendors()
	require.Len(t, visibleVendors, 1)
	assert.Equal(t, 93001, visibleVendors[0].ID)
	endpoints := model.GetSupportedEndpointMap()
	assert.Contains(t, endpoints, "allowed_endpoint")
	assert.NotContains(t, endpoints, "denied_endpoint")
	assert.NotContains(t, endpoints, "ghost_endpoint")
	assert.Empty(t, model.GetModelSupportEndpointTypes("denied-only"))

	router := gin.New()
	router.GET("/catalog", controller.DashboardListModels)
	router.GET("/models/:model", func(c *gin.Context) { controller.RetrieveModel(c, constant.ChannelTypeOpenAI) })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models/gpt-4o-mini", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"id":"gpt-4o-mini"`)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 91002).Update("status", common.ChannelStatusManuallyDisabled).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 91002).Update("enabled", false).Error)
	assert.Empty(t, model.GetGroupEnabledModels("default"))
	assert.Empty(t, model.GetPricing(), "revoked candidate cannot remain visible through a warm pricing cache")
	assert.Empty(t, model.GetVendors())
	assert.Empty(t, model.GetSupportedEndpointMap())
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models/gpt-4o-mini", nil))
	assert.Contains(t, w.Body.String(), `"code":"model_not_found"`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/catalog", nil))
	assert.NotContains(t, w.Body.String(), `"gpt-4o-mini"`)
	// Query failure must not publish or fall back to an unfiltered capability set.
	require.NoError(t, db.Where("channel_id IN ?", []int{91001, 91002, 91003, 91004, 91005}).Delete(&model.Ability{}).Error)
	require.NoError(t, db.Where("id IN ?", []int{91001, 91002, 91003, 91004, 91005}).Delete(&model.Channel{}).Error)
	require.NoError(t, db.Unscoped().Where("id IN ?", []int{93001, 93002, 93003, 93004}).Delete(&model.Model{}).Error)
	require.NoError(t, db.Unscoped().Where("id IN ?", []int{93001, 93002, 93003}).Delete(&model.Vendor{}).Error)
	common.MemoryCacheEnabled = true
	model.InitChannelCache()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	assert.Empty(t, model.GetGroupEnabledModels("default"))
	assert.False(t, model.IsModelAdmittedForPublication("gpt-4o-mini"))
	assert.Empty(t, model.GetPricing())
	assert.Empty(t, model.GetVendors())
	assert.Empty(t, model.GetSupportedEndpointMap())
	assert.Empty(t, model.GetModelSupportEndpointTypes("public-chat"))
	core.Models = oldGate
	assert.False(t, core.ModelAdmissionEnabled())
	assert.True(t, model.IsModelAdmittedForPublication("gpt-4o-mini"), "default publication must not query a closed database")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models/gpt-4o-mini", nil))
	assert.Contains(t, w.Body.String(), `"id":"gpt-4o-mini"`, "default static retrieve remains compatible")
	assert.NoError(t, oldGate.Check(context.Background(), core.Admission{}), "default boundary stays upstream-compatible")
}

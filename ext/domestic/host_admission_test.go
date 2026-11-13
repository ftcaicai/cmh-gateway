package domestic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/providerplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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
	runHostAdmissionRequestBoundary(t, channels, gate, oldGate)
	runHostBoundProviderAdmission(t, gate, oldGate)
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

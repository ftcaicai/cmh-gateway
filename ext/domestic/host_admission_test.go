package domestic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

func runHostAdmissionWrites(t *testing.T, gate *Gate, noop core.ModelGate) {
	t.Helper()
	approved, denied := `{"public-chat":"approved-target"}`, `{"public-chat":"unapproved-target"}`
	tag := "admission-write-fixture"
	makeChannel := func(id int, mapping *string) model.Channel {
		return model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "admission-write", Key: "fixture", Models: "public-chat", Group: "default", ModelMapping: mapping, Tag: &tag}
	}
	ids := []int{92001, 92002, 92003, 92004, 92005}
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
	stored, err := model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, approved, stored.GetModelMapping(), "partial patch uses merged stored type/status/models")
	patch = model.Channel{Id: good.Id, Name: "admission-renamed"}
	require.NoError(t, patch.Update())
	stored, err = model.GetChannelById(good.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "admission-renamed", stored.Name)
	assert.Equal(t, approved, stored.GetModelMapping())

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
	oldSQLite := common.SQLitePath
	common.SQLitePath = os.Getenv("SQLITE_PATH")
	oldGate := core.Models
	common.IsMasterNode = true
	require.NoError(t, model.InitDB())
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

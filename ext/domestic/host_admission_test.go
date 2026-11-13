package domestic

import (
	"context"
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

	public, err := New([]string{"gpt-4o-mini,public-chat"}, []string{"1,61"})
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
	}
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
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models/gpt-4o-mini", nil))
	assert.Contains(t, w.Body.String(), `"code":"model_not_found"`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/catalog", nil))
	assert.NotContains(t, w.Body.String(), `"gpt-4o-mini"`)
	// Query failure must not publish or fall back to an unfiltered capability set.
	require.NoError(t, db.Where("channel_id IN ?", []int{91001, 91002, 91003, 91004}).Delete(&model.Ability{}).Error)
	require.NoError(t, db.Where("id IN ?", []int{91001, 91002, 91003, 91004}).Delete(&model.Channel{}).Error)
	common.MemoryCacheEnabled = true
	model.InitChannelCache()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	assert.Empty(t, model.GetGroupEnabledModels("default"))
	assert.False(t, model.IsModelAdmittedForPublication("gpt-4o-mini"))
	core.Models = oldGate
	assert.False(t, core.ModelAdmissionEnabled())
	assert.True(t, model.IsModelAdmittedForPublication("gpt-4o-mini"), "default publication must not query a closed database")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models/gpt-4o-mini", nil))
	assert.Contains(t, w.Body.String(), `"id":"gpt-4o-mini"`, "default static retrieve remains compatible")
	assert.NoError(t, oldGate.Check(context.Background(), core.Admission{}), "default boundary stays upstream-compatible")
}

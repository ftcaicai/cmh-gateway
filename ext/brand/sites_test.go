package brand

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOEMSiteHTTPUsesOfficialSessions(t *testing.T) {
	oldDB, oldRedis, oldMaster := model.DB, common.RedisEnabled, common.IsMasterNode
	oldCritical, oldCriticalNum, oldCriticalDuration := common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	common.CriticalRateLimitEnable = false
	t.Cleanup(func() {
		common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = oldCritical, oldCriticalNum, oldCriticalDuration
	})
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/http.sqlite"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	model.DB, common.RedisEnabled, common.IsMasterNode = db, false, true
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.IsMasterNode = oldDB, oldRedis, oldMaster
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.UserAccessToken{}, &model.CasbinRule{}, &model.AuthzRole{}))
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		if oldDB != nil {
			common.IsMasterNode = false // Restore its policies without rewriting the old database.
			require.NoError(t, authz.Init(oldDB))
			return
		}
		// No exported enforcer reset exists. Leave an empty fail-closed policy
		// snapshot, reloaded while this fixture connection is still open.
		require.NoError(t, db.Where("1 = 1").Delete(&model.CasbinRule{}).Error)
		require.NoError(t, authz.ReloadPolicy())
	})
	require.NoError(t, migrations.MigrateOEM(db))
	root := model.User{Username: "oem-root-http", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	customer := model.User{Username: "oem-customer-http", AffCode: "oem-customer-http", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&customer).Error)
	admin := model.User{Username: "oem-admin-http", AffCode: "oem-admin-http", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	otherAdmin := model.User{Username: "oem-other-http", AffCode: "oem-other-http", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&otherAdmin).Error)
	t.Cleanup(func() {
		for _, id := range []int{admin.Id, otherAdmin.Id, customer.Id} {
			require.NoError(t, authz.ClearUserPermissions(id))
		}
	})
	adminSession, err := service.CreateLoginSession(admin.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	otherSession, err := service.CreateLoginSession(otherAdmin.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	rootSession, err := service.CreateLoginSession(root.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	customerSession, err := service.CreateLoginSession(customer.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	engine := gin.New()
	MountOEM(engine)
	patRaw := model.AccessTokenPrefix + "synthetic-oem-browser-only"
	pat := &model.UserAccessToken{Name: "OEM rejection fixture", TokenHash: model.AccessTokenFingerprint(patRaw), TokenHint: model.AccessTokenHint(patRaw)}
	require.NoError(t, pat.SetScopes([]string{}))
	require.NoError(t, model.CreateUserAccessToken(admin.Id, pat, 10))
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/ext/oem/sites", strings.NewReader(`{"name":"must-not-create"}`))
		request.Header.Set("Authorization", "Bearer "+patRaw)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		assert.Equal(t, http.StatusForbidden, response.Code)
	}
	var patSites, patAudits int64
	require.NoError(t, db.Model(&migrations.OEMSiteRecord{}).Count(&patSites).Error)
	require.NoError(t, db.Model(&migrations.OEMSiteAuditRecord{}).Count(&patAudits).Error)
	assert.Zero(t, patSites)
	assert.Zero(t, patAudits)
	for _, tc := range []struct {
		name, token string
		status      int
	}{{"anonymous", "", 401}, {"regular session", customerSession.AccessToken, 403}, {"root session", rootSession.AccessToken, 200}, {"seeded admin session", adminSession.AccessToken, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/ext/oem/sites", nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
		})
	}
	// Use the same official session before and after a per-user deny/revoke.
	// true matches the managed-admin baseline and does not grant a common user.
	for _, tc := range []struct {
		name    string
		userID  int
		token   string
		allowed bool
		status  int
	}{
		{"admin deny", admin.Id, adminSession.AccessToken, false, http.StatusForbidden},
		{"admin restore baseline", admin.Id, adminSession.AccessToken, true, http.StatusOK},
		{"admin revoke again", admin.Id, adminSession.AccessToken, false, http.StatusForbidden},
		{"common cannot gain admin baseline", customer.Id, customerSession.AccessToken, true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, authz.SetUserPermissions(tc.userID, authz.PermissionsMap{"cmh_oem": {"read": tc.allowed, "write": tc.allowed}}))
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				req := httptest.NewRequest(method, "/api/ext/oem/sites", bytes.NewBufferString(`{"name":"Permission probe"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+tc.token)
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)
				assert.Equal(t, tc.status, rec.Code, rec.Body.String())
			}
		})
	}
	require.NoError(t, authz.ClearUserPermissions(admin.Id))
	adminSites, err := NewSiteStore(db).List(t.Context(), admin.Id)
	require.NoError(t, err)
	require.Len(t, adminSites, 1, "only the restored admin write may create a site")
	for _, tc := range []struct{ method, suffix string }{{http.MethodGet, ""}, {http.MethodPut, ""}, {http.MethodGet, "/audits"}} {
		req := httptest.NewRequest(tc.method, "/api/ext/oem/sites/"+adminSites[0].ID+tc.suffix, bytes.NewBufferString(`{"name":"Foreign write","version":1}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+otherSession.AccessToken)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	}
	unchangedAdmin, err := NewSiteStore(db).Get(t.Context(), admin.Id, adminSites[0].ID)
	require.NoError(t, err)
	assert.Equal(t, adminSites[0], unchangedAdmin)
	adminAudits, err := NewSiteStore(db).Audits(t.Context(), admin.Id, adminSites[0].ID, Page{})
	require.NoError(t, err)
	require.Len(t, adminAudits, 1)
	invitePath := "/api/ext/oem/sites/" + adminSites[0].ID + "/customer-invitations"
	for _, allowed := range []bool{false, true} {
		require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"cmh_oem": {"read": allowed, "write": allowed}}))
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req := httptest.NewRequest(method, invitePath, strings.NewReader(fmt.Sprintf(`{"target_user_id":%d}`, customer.Id)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminSession.AccessToken)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if allowed {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			} else {
				assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			}
		}
	}
	for _, tc := range []struct{ method, suffix string }{
		{http.MethodGet, "/customer-invitations"}, {http.MethodPost, "/customer-invitations"},
		{http.MethodGet, "/customers"}, {http.MethodGet, "/customer-audits"},
	} {
		req := httptest.NewRequest(tc.method, "/api/ext/oem/sites/"+adminSites[0].ID+tc.suffix, strings.NewReader(fmt.Sprintf(`{"target_user_id":%d}`, customer.Id)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+otherSession.AccessToken)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	}
	var invitationsBefore, customerAuditsBefore, customersBefore int64
	require.NoError(t, db.Model(&migrations.OEMCustomerInvitationRecord{}).Count(&invitationsBefore).Error)
	require.NoError(t, db.Model(&migrations.OEMCustomerAuditRecord{}).Count(&customerAuditsBefore).Error)
	require.NoError(t, db.Model(&migrations.OEMCustomerRecord{}).Count(&customersBefore).Error)
	assert.Equal(t, int64(1), invitationsBefore)
	assert.Equal(t, int64(1), customerAuditsBefore)
	assert.Zero(t, customersBefore)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, invitePath, ""},
		{http.MethodPost, invitePath, fmt.Sprintf(`{"target_user_id":%d}`, customer.Id)},
		{http.MethodGet, "/api/ext/oem/customer-membership", ""},
		{http.MethodPost, "/api/ext/oem/customer-invitations/accept", `{"token":"invalid"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "new_api_refresh", Value: customerSession.RefreshToken})
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, rec.Code, rec.Body.String())
	}
	disabled := model.User{Username: "oem-disabled-target", AffCode: "oem-disabled-target", Role: common.RoleCommonUser, Status: common.UserStatusDisabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&disabled).Error)
	reqDisabled := httptest.NewRequest(http.MethodPost, invitePath, strings.NewReader(fmt.Sprintf(`{"target_user_id":%d}`, disabled.Id)))
	reqDisabled.Header.Set("Content-Type", "application/json")
	reqDisabled.Header.Set("Authorization", "Bearer "+adminSession.AccessToken)
	recDisabled := httptest.NewRecorder()
	engine.ServeHTTP(recDisabled, reqDisabled)
	assert.Equal(t, http.StatusBadRequest, recDisabled.Code, recDisabled.Body.String())
	// A fresh user ID avoids retained process-wide limiter keys, including -count runs.
	rateUser := model.User{Id: 1000000 + rand.IntN(1000000000), Username: "oem-rate-user", AffCode: "oem-rate-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&rateUser).Error)
	rateSession, err := service.CreateLoginSession(rateUser.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = true, 1, 3600
	rateEngine := gin.New()
	MountOEM(rateEngine)
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/api/ext/oem/customer-invitations/accept", strings.NewReader(`{"token":"invalid"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+rateSession.AccessToken)
		rec := httptest.NewRecorder()
		rateEngine.ServeHTTP(rec, req)
		assert.Equal(t, status, rec.Code, rec.Body.String())
	}
	for _, tc := range []struct {
		model any
		count int64
	}{{&migrations.OEMCustomerInvitationRecord{}, invitationsBefore}, {&migrations.OEMCustomerAuditRecord{}, customerAuditsBefore}, {&migrations.OEMCustomerRecord{}, customersBefore}} {
		var count int64
		require.NoError(t, db.Model(tc.model).Count(&count).Error)
		assert.Equal(t, tc.count, count, "cookie-only, disabled target and rate-limited attempts must not mutate customer records")
	}
	var sitesBefore, auditsBefore int64
	require.NoError(t, db.Model(&migrations.OEMSiteRecord{}).Count(&sitesBefore).Error)
	require.NoError(t, db.Model(&migrations.OEMSiteAuditRecord{}).Count(&auditsBefore).Error)
	for _, tc := range []struct{ name, body string }{
		{"unknown owner", `{"name":"Brand","owner_user_id":999}`},
		{"duplicate name", `{"name":"Brand","name":"Other"}`},
		{"escaped duplicate name", `{"name":"Brand","na\u006de":"Other"}`},
		{"case alias", `{"Name":"Brand"}`},
		{"null field", `{"name":null}`},
		{"null root", `null`},
		{"array root", `[]`},
		{"oversized body", `{"name":"Brand","description":"` + strings.Repeat("x", 8193) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/ext/oem/sites", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminSession.AccessToken)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var sitesAfter, auditsAfter int64
			require.NoError(t, db.Model(&migrations.OEMSiteRecord{}).Count(&sitesAfter).Error)
			require.NoError(t, db.Model(&migrations.OEMSiteAuditRecord{}).Count(&auditsAfter).Error)
			assert.Equal(t, sitesBefore, sitesAfter)
			assert.Equal(t, auditsBefore, auditsAfter)
		})
	}
	request := httptest.NewRequest(http.MethodPost, "/api/ext/oem/sites", bytes.NewBufferString(`{"name":"HTTP persisted brand","support_email":"support@example.test","description":"Synthetic"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+rootSession.AccessToken)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, 200, response.Code, response.Body.String())
	// The console keeps the returned profile as the selected one and shows the
	// owner panels only for access "owner", like the list and detail responses.
	var created struct {
		Data ReadableSite `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &created))
	assert.Equal(t, "owner", created.Data.Access, "a created profile is the caller's own")
	owned, err := NewSiteStore(db).List(t.Context(), root.Id)
	require.NoError(t, err)
	require.Len(t, owned, 1)
	assert.Equal(t, "HTTP persisted brand", owned[0].Name)
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"save", `{"name":"Updated over HTTP","support_email":"","description":"saved","version":1}`, http.StatusOK},
		{"stale save", `{"name":"Stale","version":1}`, http.StatusConflict},
		{"invalid input", `{"name":"","version":2}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/ext/oem/sites/"+owned[0].ID, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+rootSession.AccessToken)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			assert.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.status == http.StatusOK {
				var saved struct {
					Data ReadableSite `json:"data"`
				}
				require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &saved))
				assert.Equal(t, "owner", saved.Data.Access, "a saved profile stays the caller's own")
			}
		})
	}
	request = httptest.NewRequest(http.MethodGet, "/api/ext/oem/sites/"+owned[0].ID+"/audits", nil)
	request.Header.Set("Authorization", "Bearer "+rootSession.AccessToken)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var history struct {
		Success bool        `json:"success"`
		Data    []SiteAudit `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &history))
	require.Len(t, history.Data, 2)
	assert.Equal(t, int64(2), history.Data[0].Version)
	// Paging: a window past the first row, an oversized limit capped to the
	// maximum, and junk values falling back to the first page.
	for _, tc := range []struct {
		query    string
		versions []int64
	}{
		{"?offset=1&limit=1", []int64{1}},
		{"?offset=0&limit=100000", []int64{2, 1}},
		{"?offset=junk&limit=-5", []int64{2, 1}},
		{"?offset=2", []int64{}},
	} {
		request = httptest.NewRequest(http.MethodGet, "/api/ext/oem/sites/"+owned[0].ID+"/audits"+tc.query, nil)
		request.Header.Set("Authorization", "Bearer "+rootSession.AccessToken)
		response = httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, tc.query)
		history.Data = nil
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &history))
		versions := make([]int64, 0, len(history.Data))
		for _, audit := range history.Data {
			versions = append(versions, audit.Version)
		}
		assert.Equal(t, tc.versions, versions, tc.query)
	}
	foreign, err := NewSiteStore(db).List(t.Context(), customer.Id)
	require.NoError(t, err)
	assert.Empty(t, foreign)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", root.Id).Update("status", common.UserStatusDisabled).Error)
	request = httptest.NewRequest(http.MethodGet, "/api/ext/oem/sites", nil)
	request.Header.Set("Authorization", "Bearer "+rootSession.AccessToken)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestOEMSitePersistenceAndOwnership(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) { runOEMSitePersistence(t, engine) })
	}
}

func runOEMSitePersistence(t *testing.T, engine string) {
	t.Helper()
	path := t.TempDir() + "/oem.sqlite"
	open := func() (*gorm.DB, error) {
		var dialect gorm.Dialector
		switch engine {
		case "mysql":
			path = os.Getenv("CMH_OEM_TEST_MYSQL_DSN")
			if path == "" {
				t.Skip("isolated MySQL DSN not configured")
			}
			dialect = mysql.Open(path)
		case "postgres":
			path = os.Getenv("CMH_OEM_TEST_POSTGRES_DSN")
			if path == "" {
				t.Skip("isolated PostgreSQL DSN not configured")
			}
			dialect = postgres.New(postgres.Config{DSN: path, PreferSimpleProtocol: true})
		default:
			dialect = sqlite.Open(path)
		}
		return gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	}
	db, err := open()
	require.NoError(t, err)
	var version string
	query := "SELECT version()"
	if engine == "sqlite" {
		query = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(query).Scan(&version).Error)
	t.Logf("real OEM database: %s %s", engine, version)
	// The explicit opt-in DSNs are isolated fixtures. Never touch official tables.
	tables := []any{&migrations.OEMSiteAuditRecord{}, &migrations.OEMSiteRecord{}, &migrations.OEMSitePauseRecord{}, &migrations.OEMSiteTLSRecord{}, &migrations.OEMSiteLogoRecord{}, &migrations.OEMSiteLogoHistoryRecord{}, &migrations.OEMSiteDomainRecord{}}
	for _, table := range tables {
		require.NoError(t, db.Migrator().DropTable(table))
	}
	connection, err := db.DB()
	require.NoError(t, err)
	defer connection.Close()
	defer func() {
		for _, table := range tables {
			require.NoError(t, db.Migrator().DropTable(table))
		}
	}()
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, migrations.MigrateOEM(db))
	store := NewSiteStore(db)
	ctx := context.Background()
	original := SiteInput{Name: "Synthetic brand", SupportEmail: "support@example.test", Description: "Independent OEM"}
	site, err := store.Create(ctx, 11, original)
	require.NoError(t, err)
	assert.Equal(t, int64(1), site.Version)
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, migrations.MigrateOEM(db))
	second, err := open()
	require.NoError(t, err)
	secondConnection, err := second.DB()
	require.NoError(t, err)
	defer secondConnection.Close()
	restarted := NewSiteStore(second)
	persisted, err := restarted.Get(ctx, 11, site.ID)
	require.NoError(t, err)
	assert.Equal(t, site, persisted)
	hidden, err := restarted.List(ctx, 22)
	require.NoError(t, err)
	assert.Empty(t, hidden)
	_, err = restarted.Get(ctx, 22, site.ID)
	require.ErrorIs(t, err, ErrOEMNotFound)
	_, err = restarted.Update(ctx, 22, site.ID, 1, original)
	require.ErrorIs(t, err, ErrOEMNotFound)
	revised := SiteInput{Name: "Saved brand", Description: "Saved after restart"}
	updated, err := restarted.Update(ctx, 11, site.ID, 1, revised)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.Version)
	assert.Equal(t, site.CreatedAt, updated.CreatedAt)
	_, err = store.Update(ctx, 11, site.ID, 1, original)
	require.ErrorIs(t, err, ErrOEMConflict)
	var audits []migrations.OEMSiteAuditRecord
	require.NoError(t, db.Order("version").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, "site.created", audits[0].Action)
	assert.Equal(t, "site.updated", audits[1].Action)
	visibleAudits, err := store.Audits(ctx, 11, site.ID, Page{})
	require.NoError(t, err)
	require.Len(t, visibleAudits, 2)
	assert.Equal(t, int64(2), visibleAudits[0].Version)
	_, err = store.Audits(ctx, 22, site.ID, Page{})
	require.ErrorIs(t, err, ErrOEMNotFound)
	// P2 tables on this engine: pause (insert-or-nothing), logo history
	// (bytes, per-site order, pruning) and the HTTPS result (upsert and the
	// due-check join). The global DB is set only for the host-map reload.
	oldGlobal := model.DB
	model.DB = db
	paused, err := store.SetPaused(ctx, 11, site.ID, true)
	require.NoError(t, err)
	again, err := store.SetPaused(ctx, 11, site.ID, true)
	require.NoError(t, err)
	assert.Equal(t, paused.PausedAt, again.PausedAt)
	resumed, err := store.SetPaused(ctx, 11, site.ID, false)
	require.NoError(t, err)
	assert.False(t, resumed.Paused)
	logos := make([][]byte, 0, 5)
	for size := range 5 {
		var buffer bytes.Buffer
		require.NoError(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, size+1, size+1))))
		logos = append(logos, buffer.Bytes())
		_, err := store.SetLogo(ctx, 11, site.ID, buffer.Bytes())
		require.NoError(t, err)
	}
	history, err := store.LogoHistory(ctx, 11, site.ID)
	require.NoError(t, err)
	require.Len(t, history, logoHistoryKeep)
	newest, err := store.HistoricLogo(ctx, 11, site.ID, history[0].ID)
	require.NoError(t, err)
	assert.Equal(t, logos[3], newest.Data, "bytes survive the round trip and order is by sequence")
	restored, err := store.RestoreLogo(ctx, 11, site.ID, history[2].ID)
	require.NoError(t, err)
	assert.True(t, restored.HasLogo)
	verifiedHost := "shop.example.com"
	require.NoError(t, db.Create(&migrations.OEMSiteDomainRecord{SiteID: site.ID, OwnerUserID: 11, Host: verifiedHost, Challenge: strings.Repeat("a", 32), VerifiedHost: &verifiedHost, VerifiedAt: 1}).Error)
	oldProbe := probeTLS
	probeTLS = func(context.Context, string) (time.Time, error) { return time.Unix(2000000000, 0), nil }
	checked, err := store.checkDueTLS(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, checked)
	checked, err = store.checkDueTLS(ctx)
	require.NoError(t, err)
	assert.Zero(t, checked, "a ready host is not due again within a day")
	require.NoError(t, store.checkTLS(ctx, site.ID, verifiedHost), "a second write for the same site is an update")
	withTLS, err := store.Domain(ctx, 11, site.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2000000000), withTLS.TLSNotAfter)
	probeTLS = oldProbe
	model.DB = oldGlobal
	// An audit failure must roll back the profile, not leave an unaudited save.
	require.NoError(t, db.Migrator().DropTable(&migrations.OEMSiteAuditRecord{}))
	_, err = store.Update(ctx, 11, site.ID, 2, original)
	require.Error(t, err)
	unchanged, err := store.Get(ctx, 11, site.ID)
	require.NoError(t, err)
	assert.Equal(t, updated, unchanged)
	for _, invalid := range []SiteInput{{Name: ""}, {Name: " brand"}, {Name: "bad\nname"}, {Name: "brand", SupportEmail: "Person <a@example.test>"}, {Name: "brand", Description: string([]byte{0xff})}} {
		_, err := store.Create(ctx, 11, invalid)
		require.ErrorIs(t, err, ErrOEMInvalid)
	}
}

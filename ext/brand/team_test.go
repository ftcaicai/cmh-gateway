package brand

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOEMTeamReadOnlyCollaboration(t *testing.T) {
	oldDB, oldRedis, oldMaster, oldCritical := model.DB, common.RedisEnabled, common.IsMasterNode, common.CriticalRateLimitEnable
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/team.sqlite"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	model.DB, common.RedisEnabled, common.IsMasterNode, common.CriticalRateLimitEnable = db, false, true, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.IsMasterNode, common.CriticalRateLimitEnable = oldDB, oldRedis, oldMaster, oldCritical
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.UserAccessToken{}, &model.CasbinRule{}, &model.AuthzRole{}))
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		if oldDB != nil {
			common.IsMasterNode = false
			require.NoError(t, authz.Init(oldDB))
		} else {
			require.NoError(t, db.Where("1 = 1").Delete(&model.CasbinRule{}).Error)
			require.NoError(t, authz.ReloadPolicy())
		}
	})
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, migrations.MigrateOEM(db))
	users := make([]model.User, 5)
	tokens := make([]string, 5)
	for i := range users {
		role := common.RoleAdminUser
		if i == 3 {
			role = common.RoleCommonUser
		}
		if i == 4 {
			role = common.RoleRootUser
		}
		users[i] = model.User{Username: fmt.Sprintf("team-%d", i), AffCode: fmt.Sprintf("team-%d", i), Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
		require.NoError(t, db.Create(&users[i]).Error)
		session, e := service.CreateLoginSession(users[i].Id, "test", "127.0.0.1", "synthetic")
		require.NoError(t, e)
		tokens[i] = session.AccessToken
	}
	t.Cleanup(func() {
		for _, u := range users {
			require.NoError(t, authz.ClearUserPermissions(u.Id))
		}
	})
	store := NewSiteStore(db)
	ctx := context.Background()
	owner, member, outsider := users[0].Id, users[1].Id, users[2].Id
	site, err := store.Create(ctx, owner, SiteInput{Name: "Shared"})
	require.NoError(t, err)
	customerInvite, err := store.InviteCustomer(ctx, owner, site.ID, users[3].Id)
	require.NoError(t, err)
	_, err = store.AcceptCustomerInvitation(ctx, users[3].Id, customerInvite.Token)
	require.NoError(t, err)
	_, err = store.UpdateCustomerNote(ctx, owner, site.ID, users[3].Id, "merchant-private-real-note", 0)
	require.NoError(t, err)
	other, err := store.Create(ctx, outsider, SiteInput{Name: "Other"})
	require.NoError(t, err)
	engine := gin.New()
	MountOEM(engine)
	request := func(actor int, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/ext/oem"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if actor >= 0 {
			req.Header.Set("Authorization", "Bearer "+tokens[actor])
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}
	path := "/sites/" + site.ID
	patRaw := model.AccessTokenPrefix + "synthetic-team-only"
	pat := &model.UserAccessToken{Name: "Team rejection fixture", TokenHash: model.AccessTokenFingerprint(patRaw), TokenHint: model.AccessTokenHint(patRaw)}
	require.NoError(t, pat.SetScopes([]string{}))
	require.NoError(t, model.CreateUserAccessToken(owner, pat, 10))
	patRequest := httptest.NewRequest("POST", "/api/ext/oem"+path+"/team-invitations", strings.NewReader(fmt.Sprintf(`{"target_user_id":%d}`, member)))
	patRequest.Header.Set("Authorization", "Bearer "+patRaw)
	patRequest.Header.Set("Content-Type", "application/json")
	patResponse := httptest.NewRecorder()
	engine.ServeHTTP(patResponse, patRequest)
	assert.Equal(t, 403, patResponse.Code)
	assert.Equal(t, 403, request(3, "POST", "/team-invitations/accept", `{"token":"`+strings.Repeat("a", 64)+`"}`).Code)
	assert.Equal(t, 401, request(-1, "POST", "/team-invitations/accept", `{"token":"`+strings.Repeat("a", 64)+`"}`).Code)
	assert.Equal(t, 404, request(1, "GET", path, "").Code)
	for _, target := range []int{owner, users[3].Id, users[4].Id} {
		_, e := store.InviteTeamMember(ctx, owner, site.ID, target)
		assert.ErrorIs(t, e, ErrOEMInvalid)
	}
	invited, err := store.InviteTeamMember(ctx, owner, site.ID, member)
	require.NoError(t, err)
	staleUnused, err := store.InviteTeamMember(ctx, owner, site.ID, member)
	require.NoError(t, err)
	var raw migrations.OEMTeamInvitationRecord
	require.NoError(t, db.First(&raw, "id = ?", invited.Invitation.ID).Error)
	digest := sha256.Sum256([]byte(invited.Token))
	assert.Equal(t, hex.EncodeToString(digest[:]), raw.TokenHash)
	assert.NotEqual(t, invited.Token, raw.TokenHash)
	assert.EqualValues(t, 86400, raw.ExpiresAt-raw.CreatedAt)
	_, err = store.AcceptTeamInvitation(ctx, outsider, invited.Token)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	require.NoError(t, authz.SetUserPermissions(member, authz.PermissionsMap{"cmh_oem": {"read": true, "write": false}}))
	accepted := request(1, "POST", "/team-invitations/accept", `{"token":"`+invited.Token+`"}`)
	require.Equal(t, 200, accepted.Code, accepted.Body.String())
	members, err := store.TeamMembers(ctx, owner, site.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.EqualValues(t, 1, members[0].Version)
	_, err = store.AcceptTeamInvitation(ctx, member, invited.Token)
	require.NoError(t, err)
	audits, err := store.TeamAudits(ctx, owner, site.ID, Page{})
	require.NoError(t, err)
	assert.Len(t, audits, 3)
	for _, suffix := range []string{"", "/audits", "/customers", "/customer-audits"} {
		rec := request(1, "GET", path+suffix, "")
		assert.Equal(t, 200, rec.Code, rec.Body.String())
	}
	assert.Contains(t, request(1, "GET", path, "").Body.String(), `"access":"readonly"`)
	assert.Contains(t, request(0, "GET", path, "").Body.String(), `"access":"owner"`)
	assert.Contains(t, request(1, "GET", "/sites", "").Body.String(), site.ID)
	assert.NotContains(t, request(1, "GET", "/sites", "").Body.String(), other.ID)
	assert.Equal(t, 404, request(1, "GET", "/sites/"+other.ID, "").Code)
	assert.Equal(t, 404, request(4, "GET", path, "").Code, "root is not an implicit collaborator")
	notePath := path + fmt.Sprintf("/customers/%d/notes", users[3].Id)
	ownerNote := request(0, "GET", notePath, "")
	require.Equal(t, 200, ownerNote.Code)
	assert.Contains(t, ownerNote.Body.String(), "merchant-private-real-note")
	assert.Equal(t, 404, request(1, "GET", notePath, "").Code)
	assert.NotContains(t, request(1, "GET", path+"/customer-audits", "").Body.String(), "merchant-private-real-note")
	for _, suffix := range []string{"/team", "/team-invitations", "/team-audits", "/customer-invitations", "/customers/123/notes"} {
		assert.Equal(t, 404, request(1, "GET", path+suffix, "").Code, suffix)
	}
	// Even an independent write permission never changes the site's ownership rule.
	require.NoError(t, authz.ClearUserPermissions(member))
	assert.Equal(t, 404, request(1, "PUT", notePath, `{"note":"must-not-write","version":1}`).Code)
	assert.Equal(t, 404, request(1, "POST", path+fmt.Sprintf("/customers/%d/revoke", users[3].Id), "").Code)
	assert.Equal(t, 404, request(1, "POST", path+fmt.Sprintf("/team/%d/revoke", member), `{"version":1}`).Code)
	for _, tc := range []struct{ method, suffix, body string }{{"PUT", "", `{"name":"Foreign","version":1}`}, {"PUT", "/customers/123/notes", `{"note":"secret","version":0}`}, {"POST", "/customer-invitations", fmt.Sprintf(`{"target_user_id":%d}`, users[3].Id)}, {"POST", "/team-invitations", fmt.Sprintf(`{"target_user_id":%d}`, outsider)}} {
		assert.Equal(t, 404, request(1, tc.method, path+tc.suffix, tc.body).Code, tc.suffix)
	}
	require.NoError(t, authz.SetUserPermissions(member, authz.PermissionsMap{"cmh_oem": {"read": false}}))
	assert.Equal(t, 403, request(1, "GET", path, "").Code)
	require.NoError(t, authz.ClearUserPermissions(member))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member).Update("role", common.RoleCommonUser).Error)
	_, _, err = store.ReadSiteOwner(ctx, member, site.ID)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member).Update("status", common.UserStatusDisabled).Error)
	_, _, err = store.ReadSiteOwner(ctx, member, site.ID)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member).Update("status", common.UserStatusEnabled).Error)
	for _, body := range []string{`{}`, `{"target_user_id":1,"target_user_id":2}`, `{"TARGET_USER_ID":1}`, `{"target_user_id":null}`, `{"target_user_id":1,"extra":2}`, strings.Repeat(" ", 1025)} {
		assert.Equal(t, 400, request(0, "POST", path+"/team-invitations", body).Code)
	}
	for _, body := range []string{`{"version":null}`, `{"version":1.5}`, `{"version":1,"version":1}`, `{"Version":1}`, `{"version":1,"extra":true}`, `{"version":"1"}`} {
		assert.Equal(t, 400, request(0, "POST", path+fmt.Sprintf("/team/%d/revoke", member), body).Code)
	}
	for _, body := range []string{`{"token":null}`, `{"TOKEN":"x"}`, `{"token":"x","token":"y"}`, `{"token":"x","extra":1}`} {
		assert.Equal(t, 400, request(1, "POST", "/team-invitations/accept", body).Code)
	}
	revoked := request(0, "POST", path+fmt.Sprintf("/team/%d/revoke", member), `{"version":1}`)
	require.Equal(t, 200, revoked.Code, revoked.Body.String())
	assert.Equal(t, 404, request(1, "GET", path, "").Code, "same official session loses shared read")
	assert.NotContains(t, request(1, "GET", "/sites", "").Body.String(), site.ID)
	_, err = store.AcceptTeamInvitation(ctx, member, invited.Token)
	assert.ErrorIs(t, err, ErrOEMConflict)
	_, err = store.AcceptTeamInvitation(ctx, member, staleUnused.Token)
	assert.ErrorIs(t, err, ErrOEMConflict)
	reinvited, err := store.InviteTeamMember(ctx, owner, site.ID, member)
	require.NoError(t, err)
	restored, err := store.AcceptTeamInvitation(ctx, member, reinvited.Token)
	require.NoError(t, err)
	assert.EqualValues(t, 3, restored.Version)
	_, err = store.AcceptTeamInvitation(ctx, member, invited.Token)
	assert.ErrorIs(t, err, ErrOEMConflict)
	_, err = store.RevokeTeamMember(ctx, owner, site.ID, member, 1)
	assert.ErrorIs(t, err, ErrOEMConflict)
	expired, err := store.InviteTeamMember(ctx, owner, site.ID, outsider)
	require.NoError(t, err)
	require.NoError(t, db.Model(&migrations.OEMTeamInvitationRecord{}).Where("id = ?", expired.Invitation.ID).Update("expires_at", 1).Error)
	_, err = store.AcceptTeamInvitation(ctx, outsider, expired.Token)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	_, _, err = store.ReadSiteOwner(ctx, outsider, site.ID)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	var staleRecord migrations.OEMTeamInvitationRecord
	require.NoError(t, db.First(&staleRecord, "id = ?", staleUnused.Invitation.ID).Error)
	assert.Zero(t, staleRecord.AcceptedAt, "version mismatch rolls back consumption")
	// Audit write failure rolls back both invitation consumption and relationship.
	pending, err := store.InviteTeamMember(ctx, owner, site.ID, outsider)
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&migrations.OEMTeamAuditRecord{}))
	_, err = store.AcceptTeamInvitation(ctx, outsider, pending.Token)
	require.Error(t, err)
	raw = migrations.OEMTeamInvitationRecord{}
	require.NoError(t, db.First(&raw, "id = ?", pending.Invitation.ID).Error)
	assert.Zero(t, raw.AcceptedAt)
	var count int64
	require.NoError(t, db.Model(&migrations.OEMTeamMemberRecord{}).Where("site_id = ? AND user_id = ?", site.ID, outsider).Count(&count).Error)
	assert.Zero(t, count)
}

func TestOEMTeamDatabaseMatrix(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) { runOEMTeamDatabase(t, engine) })
	}
}

func runOEMTeamDatabase(t *testing.T, engine string) {
	t.Helper()
	dsn := t.TempDir() + "/team-matrix.sqlite"
	open := func() (*gorm.DB, error) {
		var dialect gorm.Dialector
		switch engine {
		case "mysql":
			if dsn = os.Getenv("CMH_OEM_TEST_MYSQL_DSN"); dsn == "" {
				t.Skip("isolated MySQL DSN not configured")
			}
			dialect = mysql.Open(dsn)
		case "postgres":
			if dsn = os.Getenv("CMH_OEM_TEST_POSTGRES_DSN"); dsn == "" {
				t.Skip("isolated PostgreSQL DSN not configured")
			}
			dialect = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		default:
			dialect = sqlite.Open(dsn + "?_pragma=busy_timeout(5000)")
		}
		return gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	}
	db, err := open()
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	oldDB, oldRedis, oldMaster := model.DB, common.RedisEnabled, common.IsMasterNode
	model.DB, common.RedisEnabled, common.IsMasterNode = db, false, true
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.IsMasterNode = oldDB, oldRedis, oldMaster
		require.NoError(t, pool.Close())
	})
	var version string
	query := "SELECT version()"
	if engine == "sqlite" {
		query = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(query).Scan(&version).Error)
	t.Logf("real OEM team database: %s %s", engine, version)
	tables := []any{&migrations.OEMTeamAuditRecord{}, &migrations.OEMTeamMemberRecord{}, &migrations.OEMTeamInvitationRecord{}, &migrations.OEMCustomerNoteRecord{}, &migrations.OEMCustomerAuditRecord{}, &migrations.OEMCustomerRecord{}, &migrations.OEMCustomerInvitationRecord{}, &migrations.OEMSiteAuditRecord{}, &migrations.OEMSiteRecord{}}
	for _, table := range tables {
		require.NoError(t, db.Migrator().DropTable(table))
	}
	t.Cleanup(func() {
		for _, table := range tables {
			require.NoError(t, db.Migrator().DropTable(table))
		}
	})
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, migrations.MigrateOEM(db))
	for _, table := range tables {
		require.True(t, db.Migrator().HasTable(table))
		require.NoError(t, db.Migrator().DropTable(table))
	}
	// Recreate the six-table immediately released schema, populated before upgrade.
	require.NoError(t, db.AutoMigrate(&migrations.OEMSiteRecord{}, &migrations.OEMSiteAuditRecord{}, &migrations.OEMCustomerInvitationRecord{}, &migrations.OEMCustomerRecord{}, &migrations.OEMCustomerAuditRecord{}, &migrations.OEMCustomerNoteRecord{}))
	ctx := t.Context()
	store := NewSiteStore(db)
	legacy, err := store.Create(ctx, 71001, SiteInput{Name: "Released six-table profile"})
	require.NoError(t, err)
	customer := migrations.OEMCustomerRecord{UserID: 71002, SiteID: legacy.ID, OwnerUserID: 71001, InvitationID: "released-invite", Active: true, JoinedAt: 1}
	require.NoError(t, db.Create(&customer).Error)
	legacyInvitation := migrations.OEMCustomerInvitationRecord{ID: "released-invite", SiteID: legacy.ID, OwnerUserID: 71001, TargetUserID: 71002, TokenHash: strings.Repeat("a", 64), CreatedAt: 1, ExpiresAt: 86401, AcceptedAt: 2}
	require.NoError(t, db.Create(&legacyInvitation).Error)
	_, err = store.UpdateCustomerNote(ctx, 71001, legacy.ID, 71002, "preserved private note", 0)
	require.NoError(t, err)
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, migrations.MigrateOEM(db))
	reopened, err := open()
	require.NoError(t, err)
	secondPool, err := reopened.DB()
	require.NoError(t, err)
	defer secondPool.Close()
	secondPool.SetMaxOpenConns(1)
	restored, err := NewSiteStore(reopened).Get(ctx, 71001, legacy.ID)
	require.NoError(t, err)
	assert.Equal(t, legacy, restored)
	note, err := NewSiteStore(reopened).CustomerNote(ctx, 71001, legacy.ID, 71002)
	require.NoError(t, err)
	assert.Equal(t, "preserved private note", note.Note)
	assert.EqualValues(t, 1, note.Version)
	var kept migrations.OEMCustomerRecord
	require.NoError(t, reopened.First(&kept, "user_id = ?", 71002).Error)
	assert.Equal(t, customer, kept)
	var keptInvitation migrations.OEMCustomerInvitationRecord
	require.NoError(t, reopened.First(&keptInvitation, "id = ?", legacyInvitation.ID).Error)
	assert.Equal(t, legacyInvitation, keptInvitation)
	customerHistory, err := NewSiteStore(reopened).CustomerAudits(ctx, 71001, legacy.ID, Page{})
	require.NoError(t, err)
	require.Len(t, customerHistory, 1)
	assert.Equal(t, "customer.note_updated", customerHistory[0].Action)
	require.Error(t, reopened.Create(&customer).Error)
	history, err := store.Audits(ctx, 71001, legacy.ID, Page{})
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CasbinRule{}, &model.AuthzRole{}))
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		if oldDB != nil {
			common.IsMasterNode = false
			require.NoError(t, authz.Init(oldDB))
		} else {
			require.NoError(t, db.Where("1 = 1").Delete(&model.CasbinRule{}).Error)
			require.NoError(t, authz.ReloadPolicy())
		}
	})
	users := make([]model.User, 4)
	for i := range users {
		users[i] = model.User{Username: fmt.Sprintf("team-matrix-%s-%d", legacy.ID[:12], i), AffCode: fmt.Sprintf("tm-%s-%d", legacy.ID[:12], i), Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
		require.NoError(t, db.Create(&users[i]).Error)
	}
	t.Cleanup(func() {
		for _, u := range users {
			require.NoError(t, authz.ClearUserPermissions(u.Id))
			require.NoError(t, db.Unscoped().Where("id = ?", u.Id).Delete(&model.User{}).Error)
		}
	})
	owner, target := users[0].Id, users[1].Id
	site, err := store.Create(ctx, owner, SiteInput{Name: "Team matrix"})
	require.NoError(t, err)
	first, err := store.InviteTeamMember(ctx, owner, site.ID, target)
	require.NoError(t, err)
	stale, err := store.InviteTeamMember(ctx, owner, site.ID, target)
	require.NoError(t, err)
	var raw migrations.OEMTeamInvitationRecord
	require.NoError(t, db.First(&raw, "id = ?", first.Invitation.ID).Error)
	digest := sha256.Sum256([]byte(first.Token))
	duplicateHash := raw
	duplicateHash.ID = strings.Repeat("f", 32)
	require.Error(t, db.Create(&duplicateHash).Error, "team invitation hash is unique")
	assert.Equal(t, hex.EncodeToString(digest[:]), raw.TokenHash)
	assert.EqualValues(t, 86400, raw.ExpiresAt-raw.CreatedAt)
	// Separate pools, same invitation: one acceptance audit; retry is exact replay.
	race := func(tokens []string, actor int) int {
		start := make(chan struct{})
		outcomes := make(chan error, 2)
		raceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		for i, token := range tokens {
			selected := store
			if i == 1 {
				selected = NewSiteStore(reopened)
			}
			go func() { <-start; _, e := selected.AcceptTeamInvitation(raceCtx, actor, token); outcomes <- e }()
		}
		close(start)
		wins := 0
		for range 2 {
			if <-outcomes == nil {
				wins++
			}
		}
		return wins
	}
	assert.GreaterOrEqual(t, race([]string{first.Token, first.Token}, target), 1)
	accepted, err := store.AcceptTeamInvitation(ctx, target, first.Token)
	require.NoError(t, err)
	assert.EqualValues(t, 1, accepted.Version)
	var auditCount int64
	require.NoError(t, db.Model(&migrations.OEMTeamAuditRecord{}).Where("site_id = ? AND user_id = ? AND action = ?", site.ID, target, "team.accepted").Count(&auditCount).Error)
	assert.EqualValues(t, 1, auditCount)
	var record migrations.OEMTeamMemberRecord
	require.NoError(t, reopened.First(&record, "site_id = ? AND user_id = ?", site.ID, target).Error)
	require.Error(t, reopened.Create(&record).Error, "composite site-user uniqueness")
	_, err = store.RevokeTeamMember(ctx, owner, site.ID, target, 1)
	require.NoError(t, err)
	for _, token := range []string{first.Token, stale.Token} {
		_, err = store.AcceptTeamInvitation(ctx, target, token)
		assert.ErrorIs(t, err, ErrOEMConflict)
	}
	fresh, err := store.InviteTeamMember(ctx, owner, site.ID, target)
	require.NoError(t, err)
	reactivated, err := store.AcceptTeamInvitation(ctx, target, fresh.Token)
	require.NoError(t, err)
	assert.EqualValues(t, 3, reactivated.Version)
	_, err = store.RevokeTeamMember(ctx, owner, site.ID, target, 1)
	assert.ErrorIs(t, err, ErrOEMConflict)
	var unused migrations.OEMTeamInvitationRecord
	require.NoError(t, db.First(&unused, "id = ?", stale.Invitation.ID).Error)
	assert.Zero(t, unused.AcceptedAt)
	// Different invitations competing for the same site-user slot must not both win.
	racer := users[2].Id
	a, err := store.InviteTeamMember(ctx, owner, site.ID, racer)
	require.NoError(t, err)
	b, err := store.InviteTeamMember(ctx, owner, site.ID, racer)
	require.NoError(t, err)
	require.Equal(t, 1, race([]string{a.Token, b.Token}, racer))
	require.NoError(t, db.Model(&migrations.OEMTeamAuditRecord{}).Where("site_id = ? AND user_id = ? AND action = ?", site.ID, racer, "team.accepted").Count(&auditCount).Error)
	assert.EqualValues(t, 1, auditCount)
	var consumed int64
	require.NoError(t, db.Model(&migrations.OEMTeamInvitationRecord{}).Where("site_id = ? AND target_user_id = ? AND accepted_at > 0", site.ID, racer).Count(&consumed).Error)
	assert.EqualValues(t, 1, consumed)
	// The same user may separately consent to a different brand; key is not global UserID.
	otherSite, err := store.Create(ctx, owner, SiteInput{Name: "Independent site"})
	require.NoError(t, err)
	otherInvite, err := store.InviteTeamMember(ctx, owner, otherSite.ID, target)
	require.NoError(t, err)
	_, err = store.AcceptTeamInvitation(ctx, target, otherInvite.Token)
	require.NoError(t, err)
	pending, err := store.InviteTeamMember(ctx, owner, site.ID, users[3].Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&migrations.OEMTeamInvitationRecord{}).Where("id = ?", pending.Invitation.ID).Update("expires_at", 1).Error)
	_, err = store.AcceptTeamInvitation(ctx, users[3].Id, pending.Token)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	require.NoError(t, db.Model(&migrations.OEMTeamInvitationRecord{}).Where("id = ?", pending.Invitation.ID).Update("expires_at", time.Now().Unix()+60).Error)
	// Missing audit table must roll back both acceptance consumption and seed/CAS.
	require.NoError(t, db.Migrator().DropTable(&migrations.OEMTeamAuditRecord{}))
	_, err = store.AcceptTeamInvitation(ctx, users[3].Id, pending.Token)
	require.Error(t, err)
	var rolled migrations.OEMTeamInvitationRecord
	require.NoError(t, db.First(&rolled, "id = ?", pending.Invitation.ID).Error)
	assert.Zero(t, rolled.AcceptedAt)
	var members int64
	require.NoError(t, db.Model(&migrations.OEMTeamMemberRecord{}).Where("site_id = ? AND user_id = ?", site.ID, users[3].Id).Count(&members).Error)
	assert.Zero(t, members)
	_, err = store.RevokeTeamMember(ctx, owner, site.ID, target, 3)
	require.Error(t, err)
	require.NoError(t, db.First(&record, "site_id = ? AND user_id = ?", site.ID, target).Error)
	assert.True(t, record.Active)
	assert.EqualValues(t, 3, record.Version)
}

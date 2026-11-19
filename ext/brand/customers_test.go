package brand

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOEMCustomerOwnership(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) { runOEMCustomerOwnership(t, engine) })
	}
}

func runOEMCustomerOwnership(t *testing.T, engine string) {
	t.Helper()
	oldDB, oldRedis := model.DB, common.RedisEnabled
	dsn := t.TempDir() + "/customers.sqlite"
	open := func() (*gorm.DB, error) {
		var dialect gorm.Dialector
		switch engine {
		case "mysql":
			dsn = os.Getenv("CMH_OEM_TEST_MYSQL_DSN")
			if dsn == "" {
				t.Skip("isolated MySQL DSN not configured")
			}
			dialect = mysql.Open(dsn)
		case "postgres":
			dsn = os.Getenv("CMH_OEM_TEST_POSTGRES_DSN")
			if dsn == "" {
				t.Skip("isolated PostgreSQL DSN not configured")
			}
			dialect = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		default:
			dialect = sqlite.Open(dsn)
		}
		return gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	}
	db, err := open()
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() { model.DB, common.RedisEnabled = oldDB, oldRedis; require.NoError(t, conn.Close()) })
	var version string
	query := "SELECT version()"
	if engine == "sqlite" {
		query = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(query).Scan(&version).Error)
	t.Logf("real OEM customer database: %s %s", engine, version)
	tables := []any{&migrations.OEMCustomerNoteRecord{}, &migrations.OEMCustomerAuditRecord{}, &migrations.OEMCustomerRecord{}, &migrations.OEMCustomerInvitationRecord{}, &migrations.OEMSiteAuditRecord{}, &migrations.OEMSiteRecord{}}
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
		require.NoError(t, db.Migrator().DropTable(table))
	}
	// Upgrade the immediately preceding five-table OEM schema with real data.
	require.NoError(t, db.AutoMigrate(&migrations.OEMSiteRecord{}, &migrations.OEMSiteAuditRecord{}, &migrations.OEMCustomerInvitationRecord{}, &migrations.OEMCustomerRecord{}, &migrations.OEMCustomerAuditRecord{}))
	legacy, err := NewSiteStore(db).Create(t.Context(), 71001, SiteInput{Name: "Previous OEM profile"})
	require.NoError(t, err)
	legacyCustomer := migrations.OEMCustomerRecord{UserID: 71002, SiteID: legacy.ID, OwnerUserID: 71001, InvitationID: "legacy-invite", Active: true, JoinedAt: 1}
	require.NoError(t, db.Create(&legacyCustomer).Error)
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, migrations.MigrateOEM(db))
	reopened, err := open()
	require.NoError(t, err)
	reopenedConn, err := reopened.DB()
	require.NoError(t, err)
	restored, err := NewSiteStore(reopened).Get(t.Context(), 71001, legacy.ID)
	require.NoError(t, err)
	assert.Equal(t, legacy, restored)
	history, err := NewSiteStore(reopened).Audits(t.Context(), 71001, legacy.ID, Page{})
	require.NoError(t, err)
	require.Len(t, history, 1)
	var restoredCustomer migrations.OEMCustomerRecord
	require.NoError(t, reopened.First(&restoredCustomer, "user_id = ?", 71002).Error)
	assert.Equal(t, legacyCustomer, restoredCustomer)
	require.Error(t, reopened.Create(&legacyCustomer).Error, "global customer uniqueness survives upgrade")
	require.NoError(t, reopened.Delete(&legacyCustomer).Error)
	require.NoError(t, reopenedConn.Close())
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.UserAccessToken{}))
	owner := model.User{Username: "customer-owner", AffCode: "customer-owner", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	target := model.User{Username: "customer-target", AffCode: "customer-target", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	other := model.User{Username: "customer-other", AffCode: "customer-other", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	suffix := strings.ReplaceAll(legacy.ID[:12], "-", "")
	for _, u := range []*model.User{&owner, &target, &other} {
		u.Username += suffix
		u.AffCode += suffix
		require.NoError(t, db.Create(u).Error)
	}
	t.Cleanup(func() {
		ids := []int{owner.Id, target.Id, other.Id}
		for _, record := range []any{&model.UserAccessToken{}, &model.UserSession{}} {
			require.NoError(t, db.Where("user_id IN ?", ids).Delete(record).Error)
		}
		require.NoError(t, db.Unscoped().Where("id IN ?", ids).Delete(&model.User{}).Error)
	})
	store := NewSiteStore(db)
	ctx := context.Background()
	site, err := store.Create(ctx, owner.Id, SiteInput{Name: "Customer site"})
	require.NoError(t, err)
	_, err = store.InviteCustomer(ctx, other.Id, site.ID, target.Id)
	require.ErrorIs(t, err, ErrOEMNotFound)
	_, err = store.InviteCustomer(ctx, owner.Id, site.ID, owner.Id)
	require.ErrorIs(t, err, ErrOEMInvalid)
	_, err = store.InviteCustomer(ctx, owner.Id, site.ID, 999999)
	require.ErrorIs(t, err, ErrOEMInvalid)
	invite, err := store.InviteCustomer(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	require.Len(t, invite.Token, 64)
	assert.Equal(t, int64(CustomerInvitationTTL/time.Second), invite.Invitation.ExpiresAt-invite.Invitation.CreatedAt)
	var saved migrations.OEMCustomerInvitationRecord
	require.NoError(t, db.First(&saved, "id = ?", invite.Invitation.ID).Error)
	assert.NotEqual(t, invite.Token, saved.TokenHash)
	collision, err := store.InviteCustomer(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	_, err = store.AcceptCustomerInvitation(ctx, other.Id, invite.Token)
	require.ErrorIs(t, err, ErrOEMNotFound)
	var accepted Customer
	var successes int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			v, e := store.AcceptCustomerInvitation(ctx, target.Id, invite.Token)
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				successes++
				accepted = v
			}
		})
	}
	wg.Wait()
	require.GreaterOrEqual(t, successes, 1)
	require.True(t, accepted.Active)
	replay, err := store.AcceptCustomerInvitation(ctx, target.Id, invite.Token)
	require.NoError(t, err)
	assert.Equal(t, accepted, replay)
	_, err = store.AcceptCustomerInvitation(ctx, target.Id, collision.Token)
	require.ErrorIs(t, err, ErrOEMConflict)
	var collisionRecord migrations.OEMCustomerInvitationRecord
	require.NoError(t, db.First(&collisionRecord, "id = ?", collision.Invitation.ID).Error)
	assert.Zero(t, collisionRecord.AcceptedAt)
	auditViews, err := store.CustomerAudits(ctx, owner.Id, site.ID, Page{})
	require.NoError(t, err)
	assert.Len(t, auditViews, 3)
	_, err = store.CustomerAudits(ctx, other.Id, site.ID, Page{})
	require.ErrorIs(t, err, ErrOEMNotFound)
	var count int64
	require.NoError(t, db.Model(&migrations.OEMCustomerAuditRecord{}).Where("action = ?", "customer.accepted").Count(&count).Error)
	assert.Equal(t, int64(1), count)
	list, err := store.Customers(ctx, owner.Id, site.ID, Page{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	_, err = store.Customers(ctx, other.Id, site.ID, Page{})
	require.ErrorIs(t, err, ErrOEMNotFound)
	_, err = store.InviteCustomer(ctx, owner.Id, site.ID, target.Id)
	require.ErrorIs(t, err, ErrOEMConflict)
	membership, err := store.CustomerMembership(ctx, target.Id)
	require.NoError(t, err)
	require.NotNil(t, membership)
	assert.Equal(t, accepted, *membership)
	// Notes are a distinct private projection; membership never acquires them.
	emptyNote, err := store.CustomerNote(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	assert.Equal(t, CustomerNote{}, emptyNote)
	note, err := store.UpdateCustomerNote(ctx, owner.Id, site.ID, target.Id, "private\nmerchant\tnote", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), note.Version)
	_, err = store.UpdateCustomerNote(ctx, owner.Id, site.ID, target.Id, "stale", 0)
	require.ErrorIs(t, err, ErrOEMConflict)
	_, err = store.CustomerNote(ctx, other.Id, site.ID, target.Id)
	require.ErrorIs(t, err, ErrOEMNotFound)
	anotherSite, err := store.Create(ctx, owner.Id, SiteInput{Name: "Other owned site"})
	require.NoError(t, err)
	_, err = store.UpdateCustomerNote(ctx, owner.Id, anotherSite.ID, target.Id, "wrong site", 1)
	require.ErrorIs(t, err, ErrOEMNotFound)
	for _, invalid := range []string{strings.Repeat("字", 667), "bad\rline", "bad\x00", string([]byte{0xff})} {
		_, err = store.UpdateCustomerNote(ctx, owner.Id, site.ID, target.Id, invalid, 1)
		require.ErrorIs(t, err, ErrOEMInvalid)
	}
	noteDB, err := open()
	require.NoError(t, err)
	noteConn, err := noteDB.DB()
	require.NoError(t, err)
	persistedNote, err := NewSiteStore(noteDB).CustomerNote(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	assert.Equal(t, note, persistedNote)
	require.NoError(t, noteConn.Close())
	membership, err = store.CustomerMembership(ctx, target.Id)
	require.NoError(t, err)
	encodedMembership, err := common.Marshal(membership)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedMembership), "note")
	// A failed audit must roll back both note update and association revocation.
	require.NoError(t, db.Migrator().DropTable(&migrations.OEMCustomerAuditRecord{}))
	_, err = store.UpdateCustomerNote(ctx, owner.Id, site.ID, target.Id, "must roll back", 1)
	require.Error(t, err)
	persistedNote, err = store.CustomerNote(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	assert.Equal(t, note, persistedNote)
	_, err = store.RevokeCustomer(ctx, owner.Id, site.ID, target.Id)
	require.Error(t, err)
	membership, err = store.CustomerMembership(ctx, target.Id)
	require.NoError(t, err)
	require.True(t, membership.Active)
	require.NoError(t, migrations.MigrateOEM(db))
	revoked, err := store.RevokeCustomer(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	assert.False(t, revoked.Active)
	note, err = store.UpdateCustomerNote(ctx, owner.Id, site.ID, target.Id, "", 1)
	require.NoError(t, err, "revoked historical relationship remains merchant-readable/editable")
	assert.Equal(t, int64(2), note.Version)
	assert.Empty(t, note.Note)
	persistedNote, err = store.CustomerNote(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	assert.Equal(t, note, persistedNote)
	repeat, err := store.RevokeCustomer(ctx, owner.Id, site.ID, target.Id)
	require.NoError(t, err)
	assert.Equal(t, revoked, repeat)
	_, err = store.AcceptCustomerInvitation(ctx, target.Id, invite.Token)
	require.ErrorIs(t, err, ErrOEMConflict)
	_, err = store.InviteCustomer(ctx, owner.Id, site.ID, target.Id)
	require.ErrorIs(t, err, ErrOEMConflict)
	// Expiry never consumes an invitation; audit failure rolls back acceptance.
	expiring, err := store.InviteCustomer(ctx, owner.Id, site.ID, other.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&migrations.OEMCustomerInvitationRecord{}).Where("id = ?", expiring.Invitation.ID).Update("expires_at", time.Now().Unix()-1).Error)
	_, err = store.AcceptCustomerInvitation(ctx, other.Id, expiring.Token)
	require.ErrorIs(t, err, ErrOEMNotFound)
	require.NoError(t, db.Model(&migrations.OEMCustomerInvitationRecord{}).Where("id = ?", expiring.Invitation.ID).Update("expires_at", time.Now().Unix()+60).Error)
	require.NoError(t, db.Migrator().DropTable(&migrations.OEMCustomerAuditRecord{}))
	_, err = store.AcceptCustomerInvitation(ctx, other.Id, expiring.Token)
	require.Error(t, err)
	saved = migrations.OEMCustomerInvitationRecord{}
	require.NoError(t, db.First(&saved, "id = ?", expiring.Invitation.ID).Error)
	assert.Zero(t, saved.AcceptedAt)
	absent, err := store.CustomerMembership(ctx, other.Id)
	require.NoError(t, err)
	assert.Nil(t, absent)
	require.NoError(t, migrations.MigrateOEM(db))
	ownerSession, err := service.CreateLoginSession(owner.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	targetSession, err := service.CreateLoginSession(other.Id, "test", "127.0.0.1", "synthetic")
	require.NoError(t, err)
	router := gin.New()
	MountOEM(router)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	assert.Equal(t, 401, request(http.MethodPost, "/api/ext/oem/customer-invitations/accept", `{"token":"`+expiring.Token+`"}`, "").Code)
	assert.Equal(t, 400, request(http.MethodPost, "/api/ext/oem/customer-invitations/accept", `{"token":"a","Token":"b"}`, targetSession.AccessToken).Code)
	assert.Equal(t, 200, request(http.MethodPost, "/api/ext/oem/customer-invitations/accept", `{"token":"`+expiring.Token+`"}`, targetSession.AccessToken).Code)
	assert.Equal(t, 200, request(http.MethodGet, "/api/ext/oem/customer-membership", "", targetSession.AccessToken).Code)
	response := request(http.MethodGet, "/api/ext/oem/sites/"+site.ID+"/customer-invitations", "", ownerSession.AccessToken)
	require.Equal(t, 200, response.Code)
	assert.NotContains(t, response.Body.String(), expiring.Token)
	assert.NotContains(t, response.Body.String(), "token_hash")
	notePath := "/api/ext/oem/sites/" + site.ID + "/customers/" + strconv.Itoa(other.Id) + "/notes"
	assert.Equal(t, 200, request(http.MethodGet, notePath, "", ownerSession.AccessToken).Code)
	assert.Equal(t, 200, request(http.MethodPut, notePath, `{"note":"private HTTP note","version":0}`, ownerSession.AccessToken).Code)
	assert.Equal(t, 409, request(http.MethodPut, notePath, `{"note":"stale","version":0}`, ownerSession.AccessToken).Code)
	for _, bad := range []string{`{"note":"a","note":"b","version":1}`, `{"note":"a","\u006eote":"b","version":1}`, `{"Note":"a","version":1}`, `{"note":"a","version":1,"extra":0}`, `{"note":null,"version":1}`, `{"note":"a","version":"1"}`, `{"note":"a","version":1.5}`, `{"note":"a","version":-1}`, `{"note":"a"}`, `{"note":"` + strings.Repeat("x", 8192) + `","version":1}`} {
		assert.Equal(t, 400, request(http.MethodPut, notePath, bad, ownerSession.AccessToken).Code)
	}
	foreignSite, err := store.Create(ctx, other.Id, SiteInput{Name: "Foreign note site"})
	require.NoError(t, err)
	for _, wrongSite := range []string{foreignSite.ID, anotherSite.ID} {
		wrongPath := "/api/ext/oem/sites/" + wrongSite + "/customers/" + strconv.Itoa(other.Id) + "/notes"
		assert.Equal(t, 404, request(http.MethodGet, wrongPath, "", ownerSession.AccessToken).Code)
		assert.Equal(t, 404, request(http.MethodPut, wrongPath, `{"note":"denied","version":0}`, ownerSession.AccessToken).Code)
	}
	assert.Equal(t, 403, request(http.MethodGet, notePath, "", targetSession.AccessToken).Code)
	assert.Equal(t, 403, request(http.MethodPut, notePath, `{"note":"bad","version":1}`, targetSession.AccessToken).Code)
	selfResponse := request(http.MethodGet, "/api/ext/oem/customer-membership", "", targetSession.AccessToken)
	assert.NotContains(t, selfResponse.Body.String(), "private HTTP note")
	assert.NotContains(t, selfResponse.Body.String(), `"note"`)
	auditsResponse := request(http.MethodGet, "/api/ext/oem/sites/"+site.ID+"/customer-audits", "", ownerSession.AccessToken)
	assert.Contains(t, auditsResponse.Body.String(), "customer.note_updated")
	assert.NotContains(t, auditsResponse.Body.String(), "private HTTP note")
	// Two independent connection pools compete for one target with different
	// invitations. The start barrier coordinates attempts, not exact lock overlap.
	racing := model.User{Username: "customer-race" + suffix, AffCode: "customer-race" + suffix, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&racing).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Where("id = ?", racing.Id).Delete(&model.User{}).Error) })
	firstRace, err := store.InviteCustomer(ctx, owner.Id, site.ID, racing.Id)
	require.NoError(t, err)
	secondRace, err := store.InviteCustomer(ctx, owner.Id, site.ID, racing.Id)
	require.NoError(t, err)
	raceDB, err := open()
	require.NoError(t, err)
	raceConnection, err := raceDB.DB()
	require.NoError(t, err)
	defer raceConnection.Close()
	start := make(chan struct{})
	raceContext, cancelRace := context.WithTimeout(ctx, 10*time.Second)
	defer cancelRace()
	outcomes := make(chan error, 2)
	for i, token := range []string{firstRace.Token, secondRace.Token} {
		raceStore := store
		if i == 1 {
			raceStore = NewSiteStore(raceDB)
		}
		go func() {
			<-start
			_, acceptErr := raceStore.AcceptCustomerInvitation(raceContext, racing.Id, token)
			outcomes <- acceptErr
		}()
	}
	close(start)
	raceSuccesses := 0
	for range 2 {
		if <-outcomes == nil {
			raceSuccesses++
		}
	}
	require.Equal(t, 1, raceSuccesses)
	var associations, acceptedAudits, consumedInvitations int64
	require.NoError(t, db.Model(&migrations.OEMCustomerRecord{}).Where("user_id = ? AND active = ?", racing.Id, true).Count(&associations).Error)
	require.NoError(t, db.Model(&migrations.OEMCustomerAuditRecord{}).Where("user_id = ? AND action = ?", racing.Id, "customer.accepted").Count(&acceptedAudits).Error)
	require.NoError(t, db.Model(&migrations.OEMCustomerInvitationRecord{}).Where("target_user_id = ? AND accepted_at > ?", racing.Id, 0).Count(&consumedInvitations).Error)
	assert.Equal(t, int64(1), associations)
	assert.Equal(t, int64(1), acceptedAudits)
	assert.Equal(t, int64(1), consumedInvitations)
	for _, version := range []int64{-1, 1<<53 - 1, 1 << 53} {
		_, err = store.UpdateCustomerNote(ctx, owner.Id, site.ID, racing.Id, "invalid version", version)
		require.ErrorIs(t, err, ErrOEMInvalid)
	}
	// Independent pools race the absent-note seed and the same expected version.
	for _, version := range []int64{0, 1} {
		noteStart := make(chan struct{})
		noteOutcomes := make(chan error, 2)
		for i := range 2 {
			noteStore := store
			if i == 1 {
				noteStore = NewSiteStore(raceDB)
			}
			go func() {
				<-noteStart
				_, writeErr := noteStore.UpdateCustomerNote(raceContext, owner.Id, site.ID, racing.Id, "race winner", version)
				noteOutcomes <- writeErr
			}()
		}
		close(noteStart)
		wins := 0
		for range 2 {
			if <-noteOutcomes == nil {
				wins++
			}
		}
		require.Equal(t, 1, wins, "one note CAS winner per expected version across independent pools")
		racedNote, readErr := store.CustomerNote(ctx, owner.Id, site.ID, racing.Id)
		require.NoError(t, readErr)
		assert.Equal(t, version+1, racedNote.Version)
		var noteAuditCount int64
		require.NoError(t, db.Model(&migrations.OEMCustomerAuditRecord{}).Where("user_id = ? AND action = ?", racing.Id, "customer.note_updated").Count(&noteAuditCount).Error)
		assert.Equal(t, version+1, noteAuditCount)
	}
	// Session-only self routes reject PAT, even when owned by an accepted customer.
	raw := model.AccessTokenPrefix + "synthetic-customer-pat"
	pat := &model.UserAccessToken{Name: "fixture", TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
	require.NoError(t, pat.SetScopes([]string{}))
	require.NoError(t, model.CreateUserAccessToken(other.Id, pat, 10))
	assert.Equal(t, 403, request(http.MethodPost, "/api/ext/oem/customer-invitations/accept", `{"token":"`+expiring.Token+`"}`, raw).Code)
	assert.Equal(t, 403, request(http.MethodGet, "/api/ext/oem/customer-membership", "", raw).Code)
}

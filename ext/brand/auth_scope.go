package brand

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// H8 brand-domain login. Rules and limits:
// ext/docs/rfc/2026-11-17-brand-direct-login-boundary.md.
//
// A brand host signs in with the official password and TOTP checks only. Every
// login flow and session is bound, in the transaction that creates it, to the
// site and browser it was started on; the main site and other brands reject it.

// browserCookieName is a random, host-only, HttpOnly identifier of the browser
// a brand login was started in. It is not a credential: only its SHA-256 is
// stored, and it is useless without the session it is bound to.
const browserCookieName = "__Host-cmh_browser"

var (
	authOnce    sync.Once
	authEnabled bool
)

// EnableAuth installs the H8 participant. It runs at startup only; there is no
// way to turn it off again in a running process. From then on every flow and
// session, main-site ones included, needs a binding row, so the extension
// tables must exist and BackfillMainAuthBindings must have run.
func EnableAuth() {
	authOnce.Do(func() {
		if err := model.RegisterAuthScopeParticipant(authScopeStore{}); err != nil {
			panic("cmh: H8 auth scope participant is already installed")
		}
		authEnabled = true
	})
}

type authScopeStore struct{}

// Admit is the early identity gate of a login: the main site admits every
// official account, a brand only its active customers. Any other outcome,
// read errors included, is the same rejection.
func (authScopeStore) Admit(tx *gorm.DB, scope core.AuthScope, userID int) error {
	if !scope.Valid() || userID <= 0 {
		return core.ErrAuthScopeRejected
	}
	if scope.IsMain() {
		return nil
	}
	if err := requireActiveCustomer(tx, scope.SiteID(), userID); err != nil {
		if !errors.Is(err, core.ErrAuthScopeRejected) {
			common.SysLog(common.LogText("brand customer lookup failed: %s", err.Error()))
		}
		return core.ErrAuthScopeRejected
	}
	return nil
}

// RevokeCustomerSessions ends the brand sessions of one customer of one site,
// so a revoked relationship cannot resume an old session if it is ever
// restored. Check already rejects them while the relationship is inactive.
func RevokeCustomerSessions(db *gorm.DB, siteID string, userID int) error {
	var bindings []migrations.AuthScopeBindingRecord
	if err := db.Select("record_id").Where("kind = ? AND site_id = ? AND user_id = ? AND expires_at > ?", string(core.AuthBindingSession), siteID, userID, time.Now().Unix()).Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		if _, err := model.RevokeUserSession(userID, binding.RecordID, "brand_customer_revoked"); err != nil {
			return err
		}
	}
	return nil
}

// Bind persists the scope of a new flow or session. For a brand scope it first
// requires an active customer relationship with that site, inside tx.
func (authScopeStore) Bind(tx *gorm.DB, binding core.AuthScopeBinding) error {
	scope := binding.Scope()
	if !scope.Valid() || len(binding.RecordID()) > 128 {
		return core.ErrAuthScopeRejected
	}
	record := migrations.AuthScopeBindingRecord{
		Kind: string(binding.Kind()), RecordID: binding.RecordID(), UserID: binding.UserID(),
		Scope: "main", ExpiresAt: binding.ExpiresAt(), CreatedAt: time.Now().Unix(),
	}
	if !scope.IsMain() {
		if err := requireActiveCustomer(tx, scope.SiteID(), binding.UserID()); err != nil {
			return err
		}
		record.Scope, record.SiteID, record.Origin, record.BrowserDigest = "brand", scope.SiteID(), scope.Origin(), scope.BrowserDigest()
	}
	return tx.Create(&record).Error
}

// Check fails closed: a missing row, a read error, an expired or mismatching
// binding and a revoked customer relationship are all the same rejection.
func (authScopeStore) Check(tx *gorm.DB, scope core.AuthScope, kind core.AuthBindingKind, recordID string, userID int) error {
	if !scope.Valid() || recordID == "" {
		return core.ErrAuthScopeRejected
	}
	var record migrations.AuthScopeBindingRecord
	if err := tx.Where("kind = ? AND record_id = ?", string(kind), recordID).Take(&record).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysLog(common.LogText("auth scope binding lookup failed: %s", err.Error()))
		}
		return core.ErrAuthScopeRejected
	}
	stored := core.MainAuthScope()
	if record.Scope != "main" {
		var err error
		if stored, err = core.NewBrandAuthScope(record.SiteID, record.Origin, record.BrowserDigest); err != nil {
			return core.ErrAuthScopeRejected
		}
	}
	now := time.Now().Unix()
	binding, err := core.NewAuthScopeBinding(stored, core.AuthBindingKind(record.Kind), record.RecordID, record.UserID, record.ExpiresAt, now)
	if err != nil {
		return core.ErrAuthScopeRejected
	}
	if err := binding.Check(scope, kind, recordID, userID, now); err != nil {
		return err
	}
	if scope.IsMain() {
		return nil
	}
	return requireActiveCustomer(tx, scope.SiteID(), userID)
}

// requireActiveCustomer is the only identity a brand login admits in this
// batch: an active customer of exactly that site. Owners and team members sign
// in on the main site.
func requireActiveCustomer(tx *gorm.DB, siteID string, userID int) error {
	var count int64
	err := tx.Model(&migrations.OEMCustomerRecord{}).
		Where("user_id = ? AND site_id = ? AND active = ?", userID, siteID, true).
		Where("EXISTS (?)", tx.Model(&migrations.OEMSiteRecord{}).Select("1").Where("id = ?", siteID)).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return core.ErrAuthScopeRejected
	}
	return nil
}

// BackfillMainAuthBindings gives every still-valid official session and login
// verification flow an explicit main-site binding. Run it once, with the
// gateway stopped, before the first start with H8 enabled; it is idempotent
// and never overwrites an existing binding. It returns the number of rows added.
func BackfillMainAuthBindings(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, errors.New("database unavailable")
	}
	now := time.Now()
	var added int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var sessions []model.UserSession
		if err := tx.Select("sid", "user_id", "expires_at").Where("status = ? AND expires_at > ?", model.UserSessionStatusActive, now.Unix()).Find(&sessions).Error; err != nil {
			return err
		}
		var flows []model.AuthFlow
		if err := tx.Where("purpose = ? AND consumed_at IS NULL AND expires_at > ?", model.AuthFlowPurposeLoginVerification, now).Find(&flows).Error; err != nil {
			return err
		}
		records := make([]migrations.AuthScopeBindingRecord, 0, len(sessions)+len(flows))
		for _, session := range sessions {
			records = append(records, migrations.AuthScopeBindingRecord{Kind: string(core.AuthBindingSession), RecordID: session.SID, UserID: session.UserID, Scope: "main", ExpiresAt: session.ExpiresAt, CreatedAt: now.Unix()})
		}
		for _, flow := range flows {
			records = append(records, migrations.AuthScopeBindingRecord{Kind: string(core.AuthBindingFlow), RecordID: flow.TokenHash, UserID: flow.UserId, Scope: "main", ExpiresAt: flow.ExpiresAt.Unix(), CreatedAt: now.Unix()})
		}
		for _, record := range records {
			var existing int64
			if err := tx.Model(&migrations.AuthScopeBindingRecord{}).Where("kind = ? AND record_id = ?", record.Kind, record.RecordID).Count(&existing).Error; err != nil {
				return err
			}
			if existing != 0 {
				continue
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			added++
		}
		return nil
	})
	return added, err
}

// RevokeBrandSessions revokes every still-active session bound to a brand
// scope. Without H8 the official code no longer reads bindings, so a brand
// session would be accepted as a main-site one: run this with the gateway
// stopped before any start without CMH_BRAND_AUTH. It revokes through the
// official model function, which reads model.DB; the caller sets that up.
func RevokeBrandSessions(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, errors.New("database unavailable")
	}
	var bindings []migrations.AuthScopeBindingRecord
	if err := db.Select("record_id", "user_id").Where("kind = ? AND scope <> ? AND expires_at > ?", string(core.AuthBindingSession), "main", time.Now().Unix()).Find(&bindings).Error; err != nil {
		return 0, err
	}
	var revoked int64
	for _, binding := range bindings {
		done, err := model.RevokeUserSession(binding.UserID, binding.RecordID, "brand_auth_disabled")
		if err != nil {
			return revoked, err
		}
		if done {
			revoked++
		}
	}
	return revoked, nil
}

// brandAuthScope builds the request scope of a brand host, or reports false
// after answering the request itself. The site comes from the operator's host
// mapping and the origin from the matched Host; neither is taken from a header
// a client or proxy could choose freely.
func brandAuthScope(c *gin.Context, siteID string) (core.AuthScope, bool) {
	origin := "https://" + strings.TrimSuffix(hostAuthority(c.Request.Host), ":443")
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		// State-changing requests must come from this exact origin. The global
		// trusted-origin list of the main site is deliberately not consulted.
		if origins := c.Request.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
			c.AbortWithStatus(http.StatusForbidden)
			return core.AuthScope{}, false
		}
	}
	browser, err := c.Cookie(browserCookieName)
	if err != nil || len(browser) != 64 {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.AbortWithStatus(http.StatusForbidden)
			return core.AuthScope{}, false
		}
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return core.AuthScope{}, false
		}
		browser = hex.EncodeToString(raw[:])
		http.SetCookie(c.Writer, &http.Cookie{Name: browserCookieName, Value: browser, Path: "/", MaxAge: 365 * 24 * 60 * 60, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	digest := sha256.Sum256([]byte(browser))
	scope, err := core.NewBrandAuthScope(siteID, origin, hex.EncodeToString(digest[:]))
	if err != nil {
		// The host is mapped to something that is not a site ID, or the Host is
		// not a canonical authority: this host has no brand login.
		c.AbortWithStatus(http.StatusNotFound)
		return core.AuthScope{}, false
	}
	return scope, true
}

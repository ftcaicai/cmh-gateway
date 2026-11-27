package brand

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/brandweb"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Official API routes are registered BEFORE H5, as in router.SetRouter.
// This catches the Gin rule that engine.Use only attaches to later routes.
func newEngine(t *testing.T) *gin.Engine {
	t.Helper()
	previousHosts := hostMap
	previousMode := gin.Mode()
	t.Setenv("CMH_BRAND_PROTOTYPE_HOSTS", "demo.brand.test=brand-demo")
	loadHosts()
	hostsOnce.Do(func() {})
	previousTenant, previousRoutes := core.Tenant, core.Routes
	t.Cleanup(func() {
		core.Tenant, core.Routes = previousTenant, previousRoutes
		hostMap = previousHosts
		gin.SetMode(previousMode)
	})
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	core.Tenant, core.Routes = Resolver{}, Routes{}
	engine.Use(core.TenantMiddleware())
	engine.GET("/api/user/self", func(c *gin.Context) { c.String(200, "official api") })
	for _, path := range []string{"/api/user/login", "/api/user/auth/refresh", "/api/user/register", "/api/user/login/verify"} {
		engine.POST(path, func(c *gin.Context) { c.String(200, "official auth") })
	}
	core.RegisterRoutes(engine)
	engine.GET("/cmh/whoami", func(c *gin.Context) { c.String(200, "private console") })
	engine.NoRoute(func(c *gin.Context) { c.String(200, "official index") })
	return engine
}

func TestHostIsolation(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		name, method, host, path string
		status                   int
		body                     string
	}{
		{"main_index", "GET", "www.main.test", "/", 200, "official index"},
		{"main_api", "GET", "www.main.test", "/api/user/self", 200, "official api"},
		{"main_auth", "POST", "www.main.test", "/api/user/login", 200, "official auth"},
		{"main_port", "GET", "www.main.test:3000", "/", 200, "official index"},
		{"brand_home", "GET", "demo.brand.test", "/", 200, "探索全部模型"},
		{"normalized_host", "GET", "DEMO.brand.test:443", "/", 200, "探索全部模型"},
		{"brand_login", "GET", "demo.brand.test", "/login", 200, "品牌身份认证尚未开通"},
		{"brand_login_head", "HEAD", "demo.brand.test", "/login", 200, ""},
		{"brand_login_post", "POST", "demo.brand.test", "/login", 404, ""},
		{"official_login_blocked", "POST", "demo.brand.test", "/api/user/login", 404, ""},
		{"official_refresh_blocked", "POST", "demo.brand.test", "/api/user/auth/refresh", 404, ""},
		{"official_register_blocked", "POST", "demo.brand.test", "/api/user/register", 404, ""},
		{"official_mfa_blocked", "POST", "demo.brand.test", "/api/user/login/verify", 404, ""},
		{"official_api_blocked", "GET", "demo.brand.test", "/api/user/self", 404, ""},
		{"console_blocked", "GET", "demo.brand.test", "/cmh/whoami", 404, ""},
		{"brand_pricing", "GET", "demo.brand.test", "/pricing", 200, "模型价格"},
		{"brand_docs", "GET", "demo.brand.test", "/docs", 200, "接入文档"},
		{"brand_account_closed", "GET", "demo.brand.test", "/account", 404, ""},
		{"brand_unknown_page", "GET", "demo.brand.test", "/register", 404, ""},
		{"unimplemented_asset", "GET", "demo.brand.test", "/brand-assets/missing.js", 404, ""},
		{"brand_post", "POST", "demo.brand.test", "/", 404, ""},
		{"brand_head", "HEAD", "demo.brand.test", "/", 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Host = tc.host
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			if tc.status == 200 && tc.host != "www.main.test" && tc.host != "www.main.test:3000" && tc.method == "GET" {
				require.Contains(t, w.Body.String(), tc.body)
				require.NotContains(t, w.Body.String(), "official index")
				require.NotContains(t, w.Body.String(), "<form")
				require.NotContains(t, w.Body.String(), "type=\"password\"")
				require.Contains(t, w.Header().Get("Content-Security-Policy"), "form-action 'none'")
			} else {
				require.Equal(t, tc.body, w.Body.String())
			}
		})
	}
	t.Run("forwarded_host_cannot_escape_brand", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/api/user/login", nil)
		r.Host = "demo.brand.test"
		r.Header.Set("X-Forwarded-Host", "www.main.test")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		require.Equal(t, 404, w.Code)
		require.Empty(t, w.Body.String())
	})
	t.Run("forwarded_host_does_not_select_brand", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/login", nil)
		r.Host = "www.main.test"
		r.Header.Set("X-Forwarded-Host", "demo.brand.test")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		require.Equal(t, 200, w.Code)
		require.Equal(t, "official index", w.Body.String())
	})
}

func TestSelfServiceDomain(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/domains.sqlite"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}))
	oldDB, oldConfig, oldLookup, oldHosts := model.DB, domainConfig, lookupCNAME, hostMap
	hostsOnce.Do(func() {})
	model.DB, hostMap = db, map[string]string{}
	domainConfig = func() domainSettings {
		return domainSettings{root: "brands.platform.test", reserved: []string{"platform.test", "brands.platform.test"}}
	}
	records := map[string]string{}
	resolverDown := false
	lookupCNAME = func(_ context.Context, name string) (string, error) {
		if resolverDown {
			return "", &net.DNSError{Err: "timeout", IsTimeout: true}
		}
		if target, ok := records[name]; ok {
			return target, nil
		}
		return "", &net.DNSError{Err: "no such host", IsNotFound: true}
	}
	// HTTPS probes never touch the network here: a host answers with the
	// certificate expiry in tlsResults, or as unreachable.
	oldProbe := probeTLS
	tlsResults := map[string]error{}
	certExpiry := time.Now().Add(60 * 24 * time.Hour).Truncate(time.Second)
	probeTLS = func(_ context.Context, host string) (time.Time, error) {
		if err, ok := tlsResults[host]; ok {
			if err != nil {
				return time.Time{}, err
			}
			return certExpiry, nil
		}
		return time.Time{}, errors.New(tlsErrorUnreachable)
	}
	t.Cleanup(func() {
		model.DB, domainConfig, lookupCNAME, hostMap, probeTLS = oldDB, oldConfig, oldLookup, oldHosts, oldProbe
		verifiedHosts.Store(nil)
		require.NoError(t, connection.Close())
	})
	ctx, store := context.Background(), NewSiteStore(db)
	siteA, err := store.Create(ctx, 1, SiteInput{Name: "A"})
	require.NoError(t, err)
	siteB, err := store.Create(ctx, 2, SiteInput{Name: "B"})
	require.NoError(t, err)

	for _, host := range []string{"localhost", "1.2.3.4", "single", "shop.platform.test", "x.brands.platform.test", "bad_host.com", "shop.example.com:443", "shop.example.test"} {
		_, err := store.SetDomain(ctx, 1, siteA.ID, host)
		assert.ErrorIs(t, err, ErrOEMInvalid, host)
	}
	_, err = store.SetDomain(ctx, 2, siteA.ID, "shop.example.com")
	assert.ErrorIs(t, err, ErrOEMNotFound, "another owner cannot set the domain")
	_, err = store.Domain(ctx, 2, siteA.ID)
	assert.ErrorIs(t, err, ErrOEMNotFound)

	saved, err := store.SetDomain(ctx, 1, siteA.ID, " Shop.Example.com. ")
	require.NoError(t, err)
	assert.Equal(t, "shop.example.com", saved.Host)
	assert.Regexp(t, `^[0-9a-f]{32}\.brands\.platform\.test$`, saved.CNAMETarget)
	assert.False(t, saved.Verified)
	assert.Empty(t, BrandIDForHost("shop.example.com"), "an unverified domain serves nothing")

	checked, err := store.VerifyDomain(ctx, 1, siteA.ID)
	require.NoError(t, err)
	assert.Equal(t, domainErrorMissing, checked.LastError)
	records["shop.example.com."] = "other.brands.platform.test."
	checked, err = store.VerifyDomain(ctx, 1, siteA.ID)
	require.NoError(t, err)
	assert.Equal(t, domainErrorMismatch, checked.LastError)
	assert.False(t, checked.Verified)

	records["shop.example.com."] = strings.ToUpper(saved.CNAMETarget) + "."
	checked, err = store.VerifyDomain(ctx, 1, siteA.ID)
	require.NoError(t, err)
	assert.True(t, checked.Verified)
	assert.Empty(t, checked.LastError)
	assert.Equal(t, siteA.ID, BrandIDForHost("SHOP.example.com:443"))

	// A second site cannot take a host that is already served, even with a
	// CNAME that matches its own target.
	other, err := store.SetDomain(ctx, 2, siteB.ID, "shop.example.com")
	require.NoError(t, err)
	records["shop.example.com."] = other.CNAMETarget + "."
	other, err = store.VerifyDomain(ctx, 2, siteB.ID)
	require.NoError(t, err)
	assert.Equal(t, domainErrorTaken, other.LastError)
	assert.False(t, other.Verified)
	assert.Equal(t, siteA.ID, BrandIDForHost("shop.example.com"))

	// Changing the domain takes the old one out of service at once; the
	// scheduled pass then verifies the new one.
	moved, err := store.SetDomain(ctx, 1, siteA.ID, "new.example.com")
	require.NoError(t, err)
	assert.NotEqual(t, saved.CNAMETarget, moved.CNAMETarget)
	assert.Empty(t, BrandIDForHost("shop.example.com"))
	records["new.example.com."] = moved.CNAMETarget + "."
	verified, err := store.verifyPendingDomains(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, verified, "the new domain and the host site B was waiting for")
	assert.Equal(t, siteA.ID, BrandIDForHost("new.example.com"))
	assert.Equal(t, siteB.ID, BrandIDForHost("shop.example.com"))

	// HTTPS readiness: display only, probed for verified hosts. A failing host
	// is retried after ten minutes, a ready one after a day; a failure after
	// success clears readiness.
	tlsResults["new.example.com"] = nil
	tlsResults["shop.example.com"] = errors.New(tlsErrorUntrusted)
	checkedTLS, err := store.checkDueTLS(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, checkedTLS)
	viewA, err := store.Domain(ctx, 1, siteA.ID)
	require.NoError(t, err)
	assert.True(t, viewA.TLSReady)
	assert.Equal(t, certExpiry.Unix(), viewA.TLSNotAfter)
	viewB, err := store.Domain(ctx, 2, siteB.ID)
	require.NoError(t, err)
	assert.False(t, viewB.TLSReady)
	assert.Equal(t, tlsErrorUntrusted, viewB.TLSError)
	checkedTLS, err = store.checkDueTLS(ctx)
	require.NoError(t, err)
	assert.Zero(t, checkedTLS, "nothing is due right after a check")
	age := func(siteID string, by time.Duration) {
		require.NoError(t, db.Model(&migrations.OEMSiteTLSRecord{}).Where("site_id = ?", siteID).Update("checked_at", time.Now().Add(-by).Unix()).Error)
	}
	age(siteB.ID, 11*time.Minute)
	tlsResults["shop.example.com"] = nil
	checkedTLS, err = store.checkDueTLS(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, checkedTLS, "only the failing host is due after ten minutes")
	viewB, err = store.Domain(ctx, 2, siteB.ID)
	require.NoError(t, err)
	assert.True(t, viewB.TLSReady)
	age(siteA.ID, 25*time.Hour)
	tlsResults["new.example.com"] = errors.New(tlsErrorPrivate)
	_, err = store.checkDueTLS(ctx)
	require.NoError(t, err)
	viewA, err = store.Domain(ctx, 1, siteA.ID)
	require.NoError(t, err)
	assert.False(t, viewA.TLSReady)
	assert.Equal(t, tlsErrorPrivate, viewA.TLSError)
	assert.Equal(t, siteA.ID, BrandIDForHost("new.example.com"), "HTTPS readiness never gates serving")
	tlsResults["new.example.com"] = nil

	// Daily recheck of verified domains. A fresh verification is not due; an
	// old one is looked up again. A resolver failure only moves the check
	// time, a removed or redirected CNAME takes the domain out of service, and
	// restoring the CNAME verifies it again without the owner's help.
	lapsed, err := store.recheckVerifiedDomains(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, lapsed, "nothing is due yet")
	stale := time.Now().Add(-2 * domainRecheckAfter).Unix()
	require.NoError(t, db.Model(&migrations.OEMSiteDomainRecord{}).Where("site_id = ?", siteA.ID).Update("checked_at", stale).Error)
	resolverDown = true
	lapsed, err = store.recheckVerifiedDomains(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, lapsed)
	assert.Equal(t, siteA.ID, BrandIDForHost("new.example.com"), "a resolver failure keeps the domain in service")
	resolverDown = false
	require.NoError(t, db.Model(&migrations.OEMSiteDomainRecord{}).Where("site_id = ?", siteA.ID).Update("checked_at", stale).Error)
	delete(records, "new.example.com.")
	lapsed, err = store.recheckVerifiedDomains(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, lapsed)
	assert.Empty(t, BrandIDForHost("new.example.com"), "a removed CNAME takes the domain out of service")
	current, err := store.Domain(ctx, 1, siteA.ID)
	require.NoError(t, err)
	assert.False(t, current.Verified)
	assert.Equal(t, domainErrorMissing, current.LastError)
	assert.Equal(t, moved.CNAMETarget, current.CNAMETarget, "the challenge is kept")
	records["new.example.com."] = moved.CNAMETarget + "."
	verified, err = store.verifyPendingDomains(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, verified)
	assert.Equal(t, siteA.ID, BrandIDForHost("new.example.com"), "restoring the CNAME verifies it again")

	cleared, err := store.SetDomain(ctx, 1, siteA.ID, "")
	require.NoError(t, err)
	assert.Empty(t, cleared.Host)
	assert.Empty(t, BrandIDForHost("new.example.com"))
	audits, err := store.Audits(ctx, 1, siteA.ID, Page{})
	require.NoError(t, err)
	actions := map[string]int{}
	for _, audit := range audits {
		actions[audit.Action]++
	}
	assert.Equal(t, map[string]int{"site.created": 1, "site.domain_set": 2, "site.domain_verified": 3, "site.domain_lapsed": 1, "site.domain_cleared": 1}, actions)
}

// TestPublicAddress pins the guard that keeps the HTTPS probe off internal
// networks.
func TestPublicAddress(t *testing.T) {
	for address, public := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"10.0.0.1": false, "172.16.0.1": false, "192.168.1.1": false, "127.0.0.1": false,
		"169.254.169.254": false, "0.0.0.0": false, "::1": false, "fc00::1": false, "fe80::1": false,
		"::ffff:10.0.0.1": false, "224.0.0.1": false,
	} {
		assert.Equal(t, public, publicAddress(netip.MustParseAddr(address)), address)
	}
}

// TestAuthScopeMaintenance covers the fail-safe behind "turn H8 off only after
// revoking brand sessions": expired bindings go away, and while H8 is off a
// live brand-bound session is revoked while main-site sessions are kept.
func TestAuthScopeMaintenance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/maintenance.sqlite"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	oldDB, oldRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = oldDB, oldRedis
		require.NoError(t, connection.Close())
	})
	ctx := context.Background()

	result, err := maintainAuthScopes(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result["skipped"], "a database without the extension tables is skipped")

	require.NoError(t, migrations.MigrateOEM(db))
	require.NoError(t, db.AutoMigrate(&model.UserSession{}))
	now := time.Now().Unix()
	session := func(sid string) model.UserSession {
		return model.UserSession{SID: sid, UserID: 7, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("a", 64), LoginMethod: "password", ExpiresAt: now + 3600, LastActiveAt: now}
	}
	for _, sid := range []string{"main-live", "brand-live", "brand-stale"} {
		s := session(sid)
		require.NoError(t, db.Create(&s).Error)
	}
	binding := func(kind core.AuthBindingKind, id, scope string, expiresAt int64) migrations.AuthScopeBindingRecord {
		return migrations.AuthScopeBindingRecord{Kind: string(kind), RecordID: id, UserID: 7, Scope: scope, SiteID: strings.Repeat("0", 32), Origin: "https://shop.example", BrowserDigest: strings.Repeat("b", 64), ExpiresAt: expiresAt, CreatedAt: now}
	}
	for _, record := range []migrations.AuthScopeBindingRecord{
		binding(core.AuthBindingSession, "main-live", "main", now+3600),
		binding(core.AuthBindingSession, "brand-live", "brand", now+3600),
		binding(core.AuthBindingSession, "brand-stale", "brand", now-1),
		binding(core.AuthBindingFlow, strings.Repeat("c", 64), "main", now-1),
	} {
		require.NoError(t, db.Create(&record).Error)
	}

	require.False(t, authEnabled, "this test runs with H8 off")
	result, err = maintainAuthScopes(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result["expired_deleted"])
	assert.Equal(t, int64(1), result["brand_sessions_revoked"])
	var remaining int64
	require.NoError(t, db.Model(&migrations.AuthScopeBindingRecord{}).Count(&remaining).Error)
	assert.Equal(t, int64(2), remaining)
	status := map[string]string{}
	var sessions []model.UserSession
	require.NoError(t, db.Find(&sessions).Error)
	for _, s := range sessions {
		status[s.SID] = s.Status + "/" + s.RevokedReason
	}
	assert.Equal(t, map[string]string{"main-live": "active/", "brand-live": "revoked/brand_auth_disabled", "brand-stale": "active/"}, status,
		"only the live brand-bound session is revoked; the stale binding's session is left to expire on its own")

	result, err = maintainAuthScopes(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result["expired_deleted"])
	assert.Equal(t, int64(0), result["brand_sessions_revoked"], "a second pass changes nothing")
}

func TestStorefrontAppearance(t *testing.T) {
	engine := newEngine(t)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/appearance.sqlite"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, migrations.MigrateOEM(db))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		verifiedHosts.Store(nil)
		require.NoError(t, connection.Close())
	})
	ctx, store := context.Background(), NewSiteStore(db)
	site, err := store.Create(ctx, 1, SiteInput{Name: "Orange & Moon", Description: "profile description"})
	require.NoError(t, err)
	hostMap["demo.brand.test"] = site.ID
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", path, nil)
		request.Host = "demo.brand.test"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	pngOf := func(width, height int) []byte {
		var buffer bytes.Buffer
		require.NoError(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))))
		return buffer.Bytes()
	}

	// Defaults: profile name and description, default theme, indexable, no logo.
	home := get("/").Body.String()
	assert.Contains(t, home, "<title>Orange &amp; Moon</title>")
	assert.Contains(t, home, `data-theme="default"`)
	assert.Contains(t, home, `content="profile description"`)
	assert.Contains(t, home, `content="index,follow"`)
	assert.NotContains(t, home, "canonical")
	assert.NotContains(t, home, "/brand-assets/logo")
	assert.Equal(t, 404, get("/brand-assets/logo").Code)

	for name, input := range map[string]SiteAppearanceInput{
		"unknown theme":  {Theme: "neon", AllowIndexing: true},
		"long title":     {Theme: "default", SEOTitle: strings.Repeat("字", 121)},
		"control chars":  {Theme: "default", SEODescription: "line\nbreak"},
		"long keywords":  {Theme: "default", SEOKeywords: strings.Repeat("k", 201)},
		"negative token": {Theme: "default", Version: -1},
	} {
		_, err := store.UpdateAppearance(ctx, 1, site.ID, input)
		assert.ErrorIs(t, err, ErrOEMInvalid, name)
	}
	_, err = store.UpdateAppearance(ctx, 2, site.ID, SiteAppearanceInput{Theme: "technology"})
	assert.ErrorIs(t, err, ErrOEMNotFound, "another owner")
	saved, err := store.UpdateAppearance(ctx, 1, site.ID, SiteAppearanceInput{
		Theme: "technology", SEOTitle: `Best <script>alert(1)</script> "API"`, SEODescription: "seo description", SEOKeywords: "ai,api", AllowIndexing: true,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), saved.Version)
	_, err = store.UpdateAppearance(ctx, 1, site.ID, SiteAppearanceInput{Theme: "business", Version: 0})
	assert.ErrorIs(t, err, ErrOEMConflict, "a stale version must not overwrite")

	for name, data := range map[string][]byte{"not an image": []byte("<svg onload=alert(1)>"), "too wide": pngOf(2049, 1), "too large": make([]byte, logoMaxBytes+1)} {
		_, err := store.SetLogo(ctx, 1, site.ID, data)
		assert.ErrorIs(t, err, ErrOEMInvalid, name)
	}
	_, err = store.SetLogo(ctx, 2, site.ID, pngOf(8, 8))
	assert.ErrorIs(t, err, ErrOEMNotFound)
	logo := pngOf(8, 8)
	withLogo, err := store.SetLogo(ctx, 1, site.ID, logo)
	require.NoError(t, err)
	assert.True(t, withLogo.HasLogo)

	pricing := get("/pricing").Body.String()
	assert.Contains(t, pricing, "<title>模型价格 - Best &lt;script&gt;alert(1)&lt;/script&gt; &#34;API&#34;</title>")
	assert.NotContains(t, pricing, "<script>alert(1)</script>")
	assert.Contains(t, pricing, `data-theme="technology"`)
	assert.Contains(t, pricing, `content="seo description"`)
	assert.Contains(t, pricing, `content="ai,api"`)
	assert.Contains(t, pricing, `<img src="/brand-assets/logo?v=`)
	served := get("/brand-assets/logo")
	assert.Equal(t, "image/png", served.Header().Get("Content-Type"))
	assert.Equal(t, logo, served.Body.Bytes())
	assert.Contains(t, get("/login").Body.String(), `content="noindex,nofollow"`, "sign-in is never indexed")

	// Without a verified domain there is no canonical address to publish.
	assert.NotContains(t, get("/robots.txt").Body.String(), "Sitemap:")
	assert.NotContains(t, get("/sitemap.xml").Body.String(), "<loc>")
	bound := "shop.example.com"
	require.NoError(t, db.Create(&migrations.OEMSiteDomainRecord{SiteID: site.ID, OwnerUserID: 1, Host: bound, Challenge: strings.Repeat("a", 32), VerifiedHost: &bound}).Error)
	assert.Contains(t, get("/docs").Body.String(), `<link rel="canonical" href="https://shop.example.com/docs">`)
	assert.Contains(t, get("/robots.txt").Body.String(), "Sitemap: https://shop.example.com/sitemap.xml")
	sitemap := get("/sitemap.xml").Body.String()
	assert.Equal(t, 3, strings.Count(sitemap, "<loc>https://shop.example.com/"))
	assert.NotContains(t, sitemap, "/login")

	// Turning indexing off closes every door at once.
	_, err = store.UpdateAppearance(ctx, 1, site.ID, SiteAppearanceInput{Theme: "business", AllowIndexing: false, Version: 1})
	require.NoError(t, err)
	hidden := get("/").Body.String()
	assert.Contains(t, hidden, `content="noindex,nofollow"`)
	assert.NotContains(t, hidden, "canonical")
	assert.Contains(t, hidden, "<title>Orange &amp; Moon</title>", "an empty SEO title falls back to the profile name")
	assert.Equal(t, "User-agent: *\nDisallow: /\n", get("/robots.txt").Body.String())
	assert.NotContains(t, get("/sitemap.xml").Body.String(), "<loc>")

	removed, err := store.SetLogo(ctx, 1, site.ID, nil)
	require.NoError(t, err)
	assert.False(t, removed.HasLogo)
	assert.Equal(t, 404, get("/brand-assets/logo").Code)

	// Logo history: a removed or replaced logo can be restored; only the last
	// three are kept, newest first, and only the owner sees them.
	history, err := store.LogoHistory(ctx, 1, site.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	restored, err := store.RestoreLogo(ctx, 1, site.ID, history[0].ID)
	require.NoError(t, err)
	assert.True(t, restored.HasLogo)
	assert.Equal(t, logo, get("/brand-assets/logo").Body.Bytes())
	for _, size := range []int{9, 10, 11, 12} {
		_, err = store.SetLogo(ctx, 1, site.ID, pngOf(size, size))
		require.NoError(t, err)
	}
	history, err = store.LogoHistory(ctx, 1, site.ID)
	require.NoError(t, err)
	require.Len(t, history, logoHistoryKeep)
	oldest, err := store.HistoricLogo(ctx, 1, site.ID, history[logoHistoryKeep-1].ID)
	require.NoError(t, err)
	assert.Equal(t, pngOf(9, 9), oldest.Data, "the 8px original and the first restore fell off the end")
	_, err = store.LogoHistory(ctx, 2, site.ID)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	_, err = store.RestoreLogo(ctx, 2, site.ID, history[0].ID)
	assert.ErrorIs(t, err, ErrOEMNotFound)
	_, err = store.RestoreLogo(ctx, 1, site.ID, "missing")
	assert.ErrorIs(t, err, ErrOEMNotFound)

	// Preview: self-contained, scriptless, with a theme tried before saving.
	preview, err := store.Preview(ctx, 1, site.ID, "technology", "")
	require.NoError(t, err)
	assert.Contains(t, preview, `data-theme="technology"`)
	assert.Contains(t, preview, `<img src="data:image/png;base64,`)
	assert.Contains(t, preview, "<style>")
	assert.NotContains(t, preview, "<script")
	assert.NotContains(t, preview, "/brand-assets/")
	saved2, err := store.Appearance(ctx, 1, site.ID)
	require.NoError(t, err)
	assert.Equal(t, "business", saved2.Theme, "a preview never saves")
	for name, args := range map[string][2]string{"unknown theme": {"neon", "home"}, "data page": {"", "pricing"}, "account page": {"", "account"}} {
		_, err := store.Preview(ctx, 1, site.ID, args[0], args[1])
		assert.ErrorIs(t, err, ErrOEMInvalid, name)
	}
	_, err = store.Preview(ctx, 2, site.ID, "", "home")
	assert.ErrorIs(t, err, ErrOEMNotFound)

	// Pause: every page answers 403 with a branded notice, APIs a fixed JSON
	// error, assets stay reachable; resuming restores the storefront at once.
	_, err = store.SetPaused(ctx, 2, site.ID, true)
	assert.ErrorIs(t, err, ErrOEMNotFound, "another owner cannot pause")
	paused, err := store.SetPaused(ctx, 1, site.ID, true)
	require.NoError(t, err)
	assert.True(t, paused.Paused)
	again, err := store.SetPaused(ctx, 1, site.ID, true)
	require.NoError(t, err)
	assert.Equal(t, paused.PausedAt, again.PausedAt, "pausing twice keeps the first time")
	for _, path := range []string{"/", "/pricing", "/login", "/robots.txt", "/nowhere"} {
		response := get(path)
		assert.Equal(t, 403, response.Code, path)
		assert.Contains(t, response.Body.String(), "站点已暂停服务", path)
		assert.Contains(t, response.Body.String(), `content="noindex,nofollow"`, path)
		assert.NotContains(t, response.Body.String(), `href="/pricing"`, "no navigation into a paused site")
	}
	api := get("/api/ext/brand/pricing")
	assert.Equal(t, 403, api.Code)
	assert.JSONEq(t, `{"success":false,"code":"brand_site_suspended","message":"站点已暂停服务"}`, api.Body.String())
	assert.Equal(t, 200, get("/brand-assets/site.css").Code)
	assert.Equal(t, 200, get("/brand-assets/logo").Code)
	resumed, err := store.SetPaused(ctx, 1, site.ID, false)
	require.NoError(t, err)
	assert.False(t, resumed.Paused)
	assert.Equal(t, 200, get("/").Code)
	audits, err := store.Audits(ctx, 1, site.ID, Page{})
	require.NoError(t, err)
	actions := map[string]int{}
	for _, audit := range audits {
		actions[audit.Action]++
	}
	assert.Equal(t, 1, actions["site.paused"], "a repeated pause writes no second record")
	assert.Equal(t, 1, actions["site.resumed"])
	assert.Equal(t, 1, actions["site.logo_restored"])
}

// TestStorefrontExposure is the G5 check carried over from the old storefront:
// nothing a brand host serves may name an upstream vendor, the platform or its
// domains. The AGPL attribution is the one deliberate exception and must stay
// on every page.
func TestStorefrontExposure(t *testing.T) {
	engine := newEngine(t)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/exposure.sqlite"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, migrations.MigrateOEM(db))
	oldDB, oldAuth := model.DB, authEnabled
	model.DB = db
	t.Cleanup(func() {
		model.DB, authEnabled = oldDB, oldAuth
		verifiedHosts.Store(nil)
		pausedSites.Store(nil)
		require.NoError(t, connection.Close())
	})
	ctx, store := context.Background(), NewSiteStore(db)
	site, err := store.Create(ctx, 1, SiteInput{Name: "Exposure Brand"})
	require.NoError(t, err)
	hostMap["demo.brand.test"] = site.ID
	get := func(path string) string {
		request := httptest.NewRequest("GET", path, nil)
		request.Host = "demo.brand.test"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Body.String()
	}
	forbidden := []string{
		// upstream vendors, products and model families
		"火山", "方舟", "volcengine", "volces", "字节跳动", "doubao", "seedance", "seedream",
		// platform identity, domains and support desk
		"xinruan", "信软", "树数", "chinamodelhub", "antrouter", "蚂蚁", "quantumnous.com", "北京",
		"平台", "主站", "举报", "投诉", "support@", "service@",
	}
	served := map[string]string{}
	for _, theme := range brandThemes {
		_, err := store.UpdateAppearance(ctx, 1, site.ID, SiteAppearanceInput{Theme: theme, AllowIndexing: true, Version: int64(slices.Index(brandThemes, theme))})
		require.NoError(t, err)
		for _, auth := range []bool{false, true} {
			authEnabled = auth
			for _, path := range []string{"/", "/pricing", "/docs", "/login"} {
				served[theme+path] = get(path)
			}
		}
		for _, page := range previewPages {
			served["preview:"+theme+page], err = store.Preview(ctx, 1, site.ID, "", page)
			require.NoError(t, err)
		}
	}
	authEnabled = oldAuth
	_, err = store.SetPaused(ctx, 1, site.ID, true)
	require.NoError(t, err)
	served["paused"] = get("/")
	for _, asset := range []string{"site.js", "site.css"} {
		content, err := brandweb.Assets.ReadFile(asset)
		require.NoError(t, err)
		served[asset] = string(content)
	}
	endpointID := regexp.MustCompile(`(?i)\bep-[a-z0-9]{4,}\b`)
	for name, body := range served {
		lower := strings.ToLower(body)
		for _, word := range forbidden {
			assert.NotContains(t, lower, strings.ToLower(word), "%s exposes %q", name, word)
		}
		assert.False(t, endpointID.MatchString(body), "%s exposes an upstream endpoint id", name)
		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") {
			continue
		}
		assert.Equal(t, 1, strings.Count(body, "Powered by new-api · QuantumNous · AGPL-3.0"), "%s keeps the AGPL attribution exactly once", name)
	}
}

// TestStorefrontPricing checks the public price list against real published
// models and ratios: only models enabled for the default group appear, and the
// figures follow the main site's model square (ratio × 2 × group ratio per
// million tokens, per-call price × group ratio, CNY at the configured rate).
func TestStorefrontPricing(t *testing.T) {
	engine := newEngine(t)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/pricing.sqlite"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}))
	settings := operation_setting.GetGeneralSetting()
	oldDB, oldRedis, oldDisplay, oldRate := model.DB, common.RedisEnabled, settings.QuotaDisplayType, operation_setting.USDExchangeRate
	oldRatios := []string{ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()}
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = oldDB, oldRedis
		settings.QuotaDisplayType, operation_setting.USDExchangeRate = oldDisplay, oldRate
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios[0]))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldRatios[1]))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldRatios[2]))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios[3]))
		model.InvalidatePricingCache()
		require.NoError(t, connection.Close())
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"brand-token":1.5}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"brand-token":4}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"brand-call":0.05}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0.8,"vip":0.5}`))
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 1, Type: 1, Status: common.ChannelStatusEnabled, Models: "brand-token,brand-call", Group: "default"},
		{Id: 2, Type: 1, Status: common.ChannelStatusEnabled, Models: "vip-only", Group: "vip"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "brand-token", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "brand-call", ChannelId: 1, Enabled: true},
		{Group: "vip", Model: "vip-only", ChannelId: 2, Enabled: true},
	}).Error)
	model.InvalidatePricingCache()
	hostMap["demo.brand.test"] = strings.Repeat("c", 32)

	list := func() map[string]storefrontModel {
		request := httptest.NewRequest("GET", "/api/ext/brand/pricing", nil)
		request.Host = "demo.brand.test"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, 200, response.Code)
		var body struct {
			Data []storefrontModel `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
		models := map[string]storefrontModel{}
		for _, item := range body.Data {
			models[item.Name] = item
		}
		return models
	}

	settings.QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	usd := list()
	require.Len(t, usd, 2, "the vip-only model is not published to the default group")
	assert.Equal(t, "token", usd["brand-token"].Billing)
	assert.Equal(t, "$", usd["brand-token"].Currency)
	assert.InDelta(t, 2.4, usd["brand-token"].Input, 1e-9, "1.5 × 2 × 0.8 per million input tokens")
	assert.InDelta(t, 9.6, usd["brand-token"].Output, 1e-9, "input × completion ratio 4")
	assert.Equal(t, "call", usd["brand-call"].Billing)
	assert.InDelta(t, 0.04, usd["brand-call"].Call, 1e-9, "0.05 × 0.8 per call")

	settings.QuotaDisplayType, operation_setting.USDExchangeRate = operation_setting.QuotaDisplayTypeCNY, 7.3
	cny := list()
	assert.Equal(t, "¥", cny["brand-token"].Currency)
	assert.InDelta(t, 17.52, cny["brand-token"].Input, 1e-9)
	assert.InDelta(t, 70.08, cny["brand-token"].Output, 1e-9)
	assert.InDelta(t, 0.292, cny["brand-call"].Call, 1e-9)
}

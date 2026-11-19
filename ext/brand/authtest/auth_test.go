// Package authtest is the H8 brand-login regression suite. It lives in its own
// test binary because enabling H8 is a process-wide, one-way switch.
package authtest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/ext/brand"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	mainHost   = "www.main.test"
	hostA      = "a.brand.test"
	hostB      = "b.brand.test"
	siteA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	siteB      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	password   = "correct horse battery staple"
	totpSecret = "JBSWY3DPEHPK3PXP"
	badLogin   = "Username or password is incorrect, or user has been banned"
)

var fixtureTables = []any{
	&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.UserAccessToken{}, &model.TwoFA{},
	&model.TwoFABackupCode{}, &model.PasskeyCredential{}, &migrations.OEMSiteRecord{},
	&migrations.OEMCustomerRecord{}, &migrations.AuthScopeBindingRecord{},
}

type reply struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		AccessToken         string `json:"access_token"`
		RequireVerification bool   `json:"require_verification"`
		FlowToken           string `json:"flow_token"`
		Methods             []struct {
			Method string `json:"method"`
		} `json:"methods"`
		Username string `json:"username"`
		SiteName string `json:"site_name"`
	} `json:"data"`
}

// client is one browser on one host: its cookies and, for a brand, its origin.
type client struct {
	t       *testing.T
	engine  *gin.Engine
	host    string
	cookies map[string]string
}

func (c *client) do(method, path, body, bearer string) (*httptest.ResponseRecorder, reply) {
	c.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = c.host
	request.Header.Set("Origin", "https://"+c.host)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	for name, value := range c.cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	response := httptest.NewRecorder()
	c.engine.ServeHTTP(response, request)
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
			continue
		}
		c.cookies[cookie.Name] = cookie.Value
	}
	var parsed reply
	_ = common.Unmarshal(response.Body.Bytes(), &parsed)
	return response, parsed
}

func (c *client) login(username string) (*httptest.ResponseRecorder, reply) {
	c.t.Helper()
	return c.do(http.MethodPost, "/api/user/login", `{"username":"h8-`+username+`","password":"`+password+`"}`, "")
}

func openFixture(t *testing.T, engine string) *gorm.DB {
	t.Helper()
	var dialect gorm.Dialector
	databaseType := common.DatabaseTypeSQLite
	switch engine {
	case "mysql":
		dsn := os.Getenv("CMH_AUTH_TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("empty dedicated MySQL database not configured")
		}
		dialect, databaseType = mysql.Open(dsn), common.DatabaseTypeMySQL
	case "postgres":
		dsn := os.Getenv("CMH_AUTH_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("empty dedicated PostgreSQL database not configured")
		}
		dialect, databaseType = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), common.DatabaseTypePostgreSQL
	default:
		dialect = sqlite.Open(t.TempDir() + "/auth.sqlite")
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	var version string
	query := "SELECT version()"
	if engine == "sqlite" {
		query = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(query).Scan(&version).Error)
	t.Logf("real database: %s %s", engine, version)

	// The DSNs name an empty database created for this suite only.
	require.NoError(t, db.Migrator().DropTable(fixtureTables...))
	require.NoError(t, db.AutoMigrate(fixtureTables...))
	oldDB, oldLogDB, oldRedis, oldType := model.DB, model.LOG_DB, common.RedisEnabled, common.MainDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetMainDatabaseType(databaseType)
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(fixtureTables...))
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetMainDatabaseType(oldType)
		require.NoError(t, connection.Close())
	})
	return db
}

func TestBrandLogin(t *testing.T) {
	t.Setenv("CMH_BRAND_PROTOTYPE_HOSTS", hostA+"="+siteA+","+hostB+"="+siteB)
	brand.EnableAuth()
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) { runBrandLogin(t, engine) })
	}
}

func runBrandLogin(t *testing.T, engineName string) {
	db := openFixture(t, engineName)
	hash, err := common.Password2Hash(password)
	require.NoError(t, err)
	users := map[string]*model.User{}
	for _, name := range []string{"plain", "mfa", "stranger", "passkey", "outsider", "legacy"} {
		user := &model.User{Username: "h8-" + name, AffCode: "h8-" + name, Password: hash, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
		require.NoError(t, db.Create(user).Error)
		users[name] = user
	}
	now := time.Now().Unix()
	for _, site := range []string{siteA, siteB} {
		require.NoError(t, db.Create(&migrations.OEMSiteRecord{ID: site, OwnerUserID: 999, Name: "Brand " + site[:1], Version: 1, CreatedAt: now, UpdatedAt: now}).Error)
	}
	for _, name := range []string{"plain", "mfa", "passkey"} {
		require.NoError(t, db.Create(&migrations.OEMCustomerRecord{UserID: users[name].Id, SiteID: siteA, OwnerUserID: 999, InvitationID: "fixture", Active: true, JoinedAt: now}).Error)
	}
	require.NoError(t, db.Create(&model.TwoFA{UserId: users["mfa"].Id, Secret: totpSecret, IsEnabled: true}).Error)
	require.NoError(t, db.Create(&model.PasskeyCredential{UserID: users["passkey"].Id, CredentialID: "fixture-credential", PublicKey: "fixture-key"}).Error)
	require.NoError(t, db.Create(&model.PasskeyCredential{UserID: users["outsider"].Id, CredentialID: "outsider-credential", PublicKey: "fixture-key"}).Error)

	previousTenant, previousRoutes := core.Tenant, core.Routes
	t.Cleanup(func() { core.Tenant, core.Routes = previousTenant, previousRoutes })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	core.Tenant, core.Routes = brand.Resolver{}, brand.Routes{}
	engine.Use(core.TenantMiddleware())
	engine.POST("/api/user/login", controller.Login)
	engine.POST("/api/user/login/verify", controller.VerifyLogin)
	engine.POST("/api/user/login/passkey/begin", controller.LoginPasskeyBegin)
	engine.POST("/api/user/auth/refresh", controller.RefreshAuth)
	engine.POST("/api/user/auth/logout", controller.AuthLogout)
	engine.GET("/api/user/self", middleware.UserAuth(), controller.GetSelf)
	core.RegisterRoutes(engine)

	newClient := func(host string) *client {
		c := &client{t: t, engine: engine, host: host, cookies: map[string]string{}}
		if host != mainHost {
			response, _ := c.do(http.MethodGet, "/login", "", "")
			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Body.String(), `id="login-form"`)
			require.Len(t, c.cookies["__Host-cmh_browser"], 64)
		}
		return c
	}
	count := func(table any, where string, args ...any) int64 {
		var n int64
		require.NoError(t, db.Model(table).Where(where, args...).Count(&n).Error)
		return n
	}
	activeSessions := func(user string) int64 {
		return count(&model.UserSession{}, "user_id = ? AND status = ?", users[user].Id, model.UserSessionStatusActive)
	}

	t.Run("browser cookie is host-only and unreadable by scripts", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/login", nil)
		request.Host = hostA
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		cookie := response.Result().Cookies()[0]
		assert.Equal(t, "__Host-cmh_browser", cookie.Name)
		assert.True(t, cookie.Secure && cookie.HttpOnly)
		assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
		assert.Empty(t, cookie.Domain)
		assert.Equal(t, "/", cookie.Path)
		assert.Contains(t, response.Header().Get("Content-Security-Policy"), "script-src 'self'")
	})

	t.Run("Turnstile appears on the sign-in page only while the main site requires it", func(t *testing.T) {
		oldEnabled, oldKey := common.TurnstileCheckEnabled, common.TurnstileSiteKey
		t.Cleanup(func() { common.TurnstileCheckEnabled, common.TurnstileSiteKey = oldEnabled, oldKey })
		get := func(path string) *httptest.ResponseRecorder {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Host = hostA
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			return response
		}
		common.TurnstileCheckEnabled, common.TurnstileSiteKey = false, "1x00000000000000000000AA"
		off := get("/login")
		assert.NotContains(t, off.Body.String(), "challenges.cloudflare.com")
		assert.NotContains(t, off.Header().Get("Content-Security-Policy"), "cloudflare")

		common.TurnstileCheckEnabled = true
		on := get("/login")
		assert.Contains(t, on.Body.String(), `id="turnstile" class="turnstile" data-sitekey="1x00000000000000000000AA"`)
		assert.Contains(t, on.Body.String(), `src="https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=cmhTurnstileReady"`)
		csp := on.Header().Get("Content-Security-Policy")
		assert.Contains(t, csp, "script-src 'self' https://challenges.cloudflare.com;")
		assert.Contains(t, csp, "frame-src https://challenges.cloudflare.com;")
		assert.Contains(t, csp, "connect-src 'self';", "the widget never widens connect-src")
		// Other storefront pages keep the strict policy.
		home := get("/")
		assert.NotContains(t, home.Body.String(), "cloudflare")
		assert.NotContains(t, home.Header().Get("Content-Security-Policy"), "cloudflare")
	})

	main, a, b := newClient(mainHost), newClient(hostA), newClient(hostB)

	t.Run("main site keeps working with H8 on", func(t *testing.T) {
		raw, result := main.login("stranger")
		require.True(t, result.Success, raw.Body.String())
		response, _ := main.do(http.MethodGet, "/api/user/self", "", result.Data.AccessToken)
		assert.Equal(t, http.StatusOK, response.Code)
		// A main-site session is not a brand session.
		response, _ = a.do(http.MethodGet, "/api/ext/brand/self", "", result.Data.AccessToken)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.Equal(t, int64(1), count(&migrations.AuthScopeBindingRecord{}, "scope = ? AND user_id = ?", "main", users["stranger"].Id))
	})

	t.Run("session without a binding is rejected until backfilled", func(t *testing.T) {
		session := &model.UserSession{SID: "11111111-1111-4111-8111-111111111111", UserID: users["legacy"].Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("0", 64), LoginMethod: "password", CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600}
		require.NoError(t, db.Create(session).Error)
		token, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: session.UserID, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
		require.NoError(t, err)
		response, _ := main.do(http.MethodGet, "/api/user/self", "", token)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		added, err := brand.BackfillMainAuthBindings(db)
		require.NoError(t, err)
		assert.Equal(t, int64(1), added)
		added, err = brand.BackfillMainAuthBindings(db)
		require.NoError(t, err)
		assert.Zero(t, added)
		response, _ = main.do(http.MethodGet, "/api/user/self", "", token)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("non-customer and wrong password get the same answer", func(t *testing.T) {
		flows := count(&model.AuthFlow{}, "1 = 1")
		_, wrong := a.do(http.MethodPost, "/api/user/login", `{"username":"h8-plain","password":"wrong"}`, "")
		_, stranger := a.login("stranger")
		_, otherBrand := b.login("plain")
		for _, result := range []reply{wrong, stranger, otherBrand} {
			assert.False(t, result.Success)
			assert.Equal(t, badLogin, result.Message)
		}
		assert.Equal(t, int64(1), activeSessions("stranger"), "only the earlier main-site session")
		assert.Zero(t, activeSessions("plain"))
		assert.Equal(t, flows, count(&model.AuthFlow{}, "1 = 1"))
		assert.Empty(t, a.cookies[service.RefreshCookieName])
	})

	t.Run("brand POST requires the exact origin and the browser cookie", func(t *testing.T) {
		for _, origin := range []string{"", "https://" + mainHost, "http://" + hostA} {
			request := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`{"username":"h8-plain","password":"`+password+`"}`))
			request.Host = hostA
			if origin != "" {
				request.Header.Set("Origin", origin)
			}
			request.AddCookie(&http.Cookie{Name: "__Host-cmh_browser", Value: a.cookies["__Host-cmh_browser"]})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, http.StatusForbidden, response.Code, origin)
		}
		noCookie := &client{t: t, engine: engine, host: hostA, cookies: map[string]string{}}
		response, _ := noCookie.login("plain")
		assert.Equal(t, http.StatusForbidden, response.Code)
		response, _ = a.do(http.MethodPost, "/api/user/register", `{}`, "")
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Zero(t, activeSessions("plain"))
	})

	var plainToken string
	t.Run("customer signs in and the session stays on its site and browser", func(t *testing.T) {
		_, result := a.login("plain")
		require.True(t, result.Success, result.Message)
		plainToken = result.Data.AccessToken
		require.NotEmpty(t, a.cookies[service.RefreshCookieName])
		response, self := a.do(http.MethodGet, "/api/ext/brand/self", "", plainToken)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "h8-plain", self.Data.Username)
		assert.Equal(t, "Brand a", self.Data.SiteName)
		assert.NotContains(t, response.Body.String(), "quota")

		otherBrowser := newClient(hostA)
		for name, attempt := range map[string]func() *httptest.ResponseRecorder{
			"main site": func() *httptest.ResponseRecorder {
				r, _ := main.do(http.MethodGet, "/api/user/self", "", plainToken)
				return r
			},
			"other brand": func() *httptest.ResponseRecorder {
				r, _ := b.do(http.MethodGet, "/api/ext/brand/self", "", plainToken)
				return r
			},
			"other browser": func() *httptest.ResponseRecorder {
				r, _ := otherBrowser.do(http.MethodGet, "/api/ext/brand/self", "", plainToken)
				return r
			},
		} {
			assert.Equal(t, http.StatusUnauthorized, attempt().Code, name)
		}
		response, _ = a.do(http.MethodGet, "/api/user/self", "", plainToken)
		assert.Equal(t, http.StatusNotFound, response.Code, "official profile API stays closed on a brand host")
	})

	t.Run("access tokens are not accepted on a brand host", func(t *testing.T) {
		raw := model.AccessTokenPrefix + "h8-brand-rejects-pat"
		pat := &model.UserAccessToken{Name: "fixture", TokenHash: model.AccessTokenFingerprint(raw), TokenHint: model.AccessTokenHint(raw)}
		require.NoError(t, pat.SetScopes([]string{"profile:read"}))
		require.NoError(t, model.CreateUserAccessToken(users["plain"].Id, pat, 10))
		response, _ := a.do(http.MethodGet, "/api/ext/brand/self", "", raw)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("refresh and logout from another scope change nothing", func(t *testing.T) {
		var before model.UserSession
		require.NoError(t, db.Where("user_id = ?", users["plain"].Id).First(&before).Error)
		thief := &client{t: t, engine: engine, host: mainHost, cookies: map[string]string{service.RefreshCookieName: a.cookies[service.RefreshCookieName]}}
		response, _ := thief.do(http.MethodPost, "/api/user/auth/refresh", "", "")
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.NotEmpty(t, thief.cookies[service.RefreshCookieName], "a wrong-scope refresh does not clear the cookie")
		response, _ = thief.do(http.MethodPost, "/api/user/auth/logout", "", "")
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		response, _ = main.do(http.MethodPost, "/api/user/auth/logout", "", plainToken)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		var after model.UserSession
		require.NoError(t, db.Where("sid = ?", before.SID).First(&after).Error)
		assert.Equal(t, model.UserSessionStatusActive, after.Status)
		assert.Equal(t, before.RefreshHash, after.RefreshHash)

		response, refreshed := a.do(http.MethodPost, "/api/user/auth/refresh", "", "")
		require.Equal(t, http.StatusOK, response.Code)
		plainToken = refreshed.Data.AccessToken
		response, _ = a.do(http.MethodGet, "/api/ext/brand/self", "", plainToken)
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("revoking the customer relationship ends the brand session", func(t *testing.T) {
		require.NoError(t, db.Model(&migrations.OEMCustomerRecord{}).Where("user_id = ?", users["plain"].Id).Update("active", false).Error)
		response, _ := a.do(http.MethodGet, "/api/ext/brand/self", "", plainToken)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		response, _ = a.do(http.MethodPost, "/api/user/auth/refresh", "", "")
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		_, result := newClient(hostA).login("plain")
		assert.Equal(t, badLogin, result.Message)
		require.NoError(t, db.Model(&migrations.OEMCustomerRecord{}).Where("user_id = ?", users["plain"].Id).Update("active", true).Error)
		response, _ = a.do(http.MethodPost, "/api/user/auth/logout", "", plainToken)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Zero(t, activeSessions("plain"))
	})

	t.Run("TOTP is required and the flow stays on its site and browser", func(t *testing.T) {
		mfa := newClient(hostA)
		_, challenge := mfa.login("mfa")
		require.True(t, challenge.Success && challenge.Data.RequireVerification, challenge.Message)
		require.Len(t, challenge.Data.Methods, 1)
		assert.Equal(t, service.VerificationMethodTwoFA, challenge.Data.Methods[0].Method)
		assert.Zero(t, activeSessions("mfa"))
		code, err := totp.GenerateCode(totpSecret, time.Now())
		require.NoError(t, err)
		body := `{"flow_token":"` + challenge.Data.FlowToken + `","method":"2fa","code":"` + code + `"}`

		// A valid code presented from the wrong place must not be consumed.
		for name, wrong := range map[string]*client{"main site": main, "other brand": b, "other browser": newClient(hostA)} {
			_, result := wrong.do(http.MethodPost, "/api/user/login/verify", body, "")
			assert.False(t, result.Success, name)
		}
		_, passkey := mfa.do(http.MethodPost, "/api/user/login/passkey/begin", `{"flow_token":"`+challenge.Data.FlowToken+`"}`, "")
		assert.False(t, passkey.Success)
		var factor model.TwoFA
		require.NoError(t, db.Where("user_id = ?", users["mfa"].Id).First(&factor).Error)
		assert.Nil(t, factor.LastUsedAt)
		assert.Zero(t, factor.FailedAttempts)
		assert.Zero(t, activeSessions("mfa"))

		// A binding failure rolls back the flow consumption and the session.
		failBinding := errors.New("injected binding failure")
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("h8:fail-session-binding", func(tx *gorm.DB) {
			if record, ok := tx.Statement.Dest.(*migrations.AuthScopeBindingRecord); ok && record.Kind == string(core.AuthBindingSession) {
				_ = tx.AddError(failBinding)
			}
		}))
		_, failed := mfa.do(http.MethodPost, "/api/user/login/verify", body, "")
		require.NoError(t, db.Callback().Create().Remove("h8:fail-session-binding"))
		assert.False(t, failed.Success)
		assert.Zero(t, count(&model.UserSession{}, "user_id = ?", users["mfa"].Id))
		assert.Equal(t, int64(1), count(&model.AuthFlow{}, "user_id = ? AND consumed_at IS NULL", users["mfa"].Id))
		assert.Empty(t, mfa.cookies[service.RefreshCookieName])

		// The factor was used by the failed attempt above, so a fresh login and
		// the next code are needed; that limit is documented in the RFC.
		require.NoError(t, db.Model(&model.TwoFA{}).Where("user_id = ?", users["mfa"].Id).Update("last_used_at", nil).Error)
		_, completed := mfa.do(http.MethodPost, "/api/user/login/verify", body, "")
		require.True(t, completed.Success, completed.Message)
		response, _ := mfa.do(http.MethodGet, "/api/ext/brand/self", "", completed.Data.AccessToken)
		assert.Equal(t, http.StatusOK, response.Code)
		_, replay := mfa.do(http.MethodPost, "/api/user/login/verify", body, "")
		assert.False(t, replay.Success)
		assert.Equal(t, int64(1), activeSessions("mfa"))
	})

	t.Run("an account that needs a Passkey cannot sign in on a brand", func(t *testing.T) {
		// A non-customer in the same state must still look like a wrong password.
		_, outsider := newClient(hostA).login("outsider")
		assert.Equal(t, badLogin, outsider.Message)
		_, result := newClient(hostA).login("passkey")
		assert.False(t, result.Success)
		assert.False(t, result.Data.RequireVerification)
		assert.Zero(t, activeSessions("passkey"))
		assert.Zero(t, count(&model.AuthFlow{}, "user_id = ?", users["passkey"].Id))
	})
}

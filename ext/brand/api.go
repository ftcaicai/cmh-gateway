package brand

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/MoeForge/moego/ojson"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

var OEMRead = authz.Permission{Resource: "cmh_oem", Action: "read"}
var OEMWrite = authz.Permission{Resource: "cmh_oem", Action: "write"}

func init() {
	authz.RegisterResource(authz.ResourceDefinition{Resource: "cmh_oem", LabelKey: "OEM workspace", Actions: []authz.ActionDefinition{
		{Action: "read", LabelKey: "Read OEM workspace", DescriptionKey: "Read owned brand profiles and operation records.", DefaultRoles: []string{authz.BuiltInRoleAdmin}},
		{Action: "write", LabelKey: "Edit OEM workspace", DescriptionKey: "Create and edit owned brand profiles.", DefaultRoles: []string{authz.BuiltInRoleAdmin}},
	}})
}

// MountOEM reuses dashboard authentication and Casbin. Neither a relay key nor
// knowledge of a brand ID grants access. No schema changes happen on requests.
func MountOEM(engine *gin.Engine) {
	group := engine.Group("/api/ext/oem", middleware.DisableCache(), middleware.UserAuth())
	group.Use(func(c *gin.Context) {
		if _, ok := middleware.GetSessionAuthIdentity(c); !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	})
	MountOEMCatalog(group)
	mountOEMCustomers(group)
	mountOEMTeam(group)
	handler := siteHTTP{store: func() *SiteStore { return NewSiteStore(model.DB) }}
	group.GET("/sites", middleware.RequirePermission(OEMRead), handler.list)
	group.GET("/sites/:id/audits", middleware.RequirePermission(OEMRead), handler.audits)
	group.GET("/sites/:id", middleware.RequirePermission(OEMRead), handler.get)
	group.POST("/sites", middleware.RequirePermission(OEMWrite), handler.create)
	group.PUT("/sites/:id", middleware.RequirePermission(OEMWrite), handler.update)
	group.GET("/sites/:id/appearance", middleware.RequirePermission(OEMRead), handler.appearance)
	group.PUT("/sites/:id/appearance", middleware.RequirePermission(OEMWrite), handler.updateAppearance)
	group.GET("/sites/:id/logo", middleware.RequirePermission(OEMRead), handler.logo)
	group.PUT("/sites/:id/logo", middleware.RequirePermission(OEMWrite), middleware.UserCriticalRateLimit("cmh-oem-logo"), handler.setLogo)
	group.GET("/sites/:id/domain", middleware.RequirePermission(OEMRead), handler.domain)
	group.PUT("/sites/:id/domain", middleware.RequirePermission(OEMWrite), middleware.UserCriticalRateLimit("cmh-oem-domain"), handler.setDomain)
	group.POST("/sites/:id/domain/verify", middleware.RequirePermission(OEMWrite), middleware.UserCriticalRateLimit("cmh-oem-domain"), handler.verifyDomain)
}

type siteHTTP struct{ store func() *SiteStore }

func oemResponse(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
		return
	}
	status, code := http.StatusServiceUnavailable, "OEM_UNAVAILABLE"
	switch {
	case errors.Is(err, ErrOEMInvalid):
		status, code = http.StatusBadRequest, "OEM_INVALID_INPUT"
	case errors.Is(err, ErrOEMNotFound):
		status, code = http.StatusNotFound, "OEM_NOT_FOUND"
	case errors.Is(err, ErrOEMConflict):
		status, code = http.StatusConflict, "OEM_VERSION_CONFLICT"
	}
	c.JSON(status, gin.H{"success": false, "code": code})
}

// pageQuery reads ?offset=&limit= for the list endpoints. Absent or malformed
// values fall back to the first page of the default size rather than failing,
// so older console builds keep working.
func pageQuery(c *gin.Context) Page {
	offset, _ := strconv.Atoi(c.Query("offset"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	return Page{Offset: offset, Limit: limit}
}

func (h siteHTTP) list(c *gin.Context) {
	value, err := h.store().ReadableSites(c.Request.Context(), c.GetInt("id"))
	oemResponse(c, value, err)
}
func (h siteHTTP) get(c *gin.Context) {
	store := h.store()
	owner, access, err := store.ReadSiteOwner(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	if err != nil {
		oemResponse(c, nil, err)
		return
	}
	value, err := store.Get(c.Request.Context(), owner, c.Param("id"))
	oemResponse(c, ReadableSite{Site: value, Access: access}, err)
}

func (h siteHTTP) audits(c *gin.Context) {
	store := h.store()
	owner, _, err := store.ReadSiteOwner(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	if err != nil {
		oemResponse(c, nil, err)
		return
	}
	value, err := store.Audits(c.Request.Context(), owner, c.Param("id"), pageQuery(c))
	oemResponse(c, value, err)
}

func decodeOEMInput(c *gin.Context, target any) bool {
	if c.ContentType() != "application/json" {
		oemResponse(c, nil, ErrOEMInvalid)
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	if err != nil || !utf8.Valid(data) || !ojson.ValidBytes(data) {
		oemResponse(c, nil, ErrOEMInvalid)
		return false
	}
	root := ojson.ParseBytes(data)
	valid := root.IsObject()
	seen := make(map[string]bool)
	root.ForEach(func(key, value ojson.Result) bool {
		name := key.String()
		if seen[name] {
			valid = false
		}
		seen[name] = true
		switch name {
		case "name", "support_email", "description":
			valid = valid && value.Type == ojson.String
		case "version":
			valid = valid && c.Request.Method == http.MethodPut && value.Type == ojson.Number
		default:
			valid = false
		}
		return valid
	})
	if !valid || common.Unmarshal(data, target) != nil {
		oemResponse(c, nil, ErrOEMInvalid)
		return false
	}
	return true
}
func (h siteHTTP) create(c *gin.Context) {
	var input SiteInput
	if !decodeOEMInput(c, &input) {
		return
	}
	value, err := h.store().Create(c.Request.Context(), c.GetInt("id"), input)
	oemResponse(c, value, err)
}
func (h siteHTTP) update(c *gin.Context) {
	var input struct {
		SiteInput
		Version int64 `json:"version"`
	}
	if !decodeOEMInput(c, &input) {
		return
	}
	value, err := h.store().Update(c.Request.Context(), c.GetInt("id"), c.Param("id"), input.Version, input.SiteInput)
	oemResponse(c, value, err)
}

// The domain endpoints are owner-only: a shared read-only collaborator cannot
// see or change where a brand is served.
func (h siteHTTP) domain(c *gin.Context) {
	value, err := h.store().Domain(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, value, err)
}

func (h siteHTTP) setDomain(c *gin.Context) {
	var input struct {
		Host string `json:"host"`
	}
	if !decodeCustomerInput(c, "host", &input) {
		return
	}
	value, err := h.store().SetDomain(c.Request.Context(), c.GetInt("id"), c.Param("id"), input.Host)
	oemResponse(c, value, err)
}

func (h siteHTTP) verifyDomain(c *gin.Context) {
	value, err := h.store().VerifyDomain(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, value, err)
}

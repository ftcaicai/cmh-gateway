package brand

import (
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/MoeForge/moego/ojson"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func mountOEMCustomers(group *gin.RouterGroup) {
	h := customerHTTP{}
	group.GET("/sites/:id/customer-audits", middleware.RequirePermission(OEMRead), h.audits)
	group.GET("/sites/:id/customers/:user_id/notes", middleware.RequirePermission(OEMRead), h.note)
	group.PUT("/sites/:id/customers/:user_id/notes", middleware.RequirePermission(OEMWrite), h.updateNote)
	group.GET("/sites/:id/customers", middleware.RequirePermission(OEMRead), h.list)
	group.GET("/sites/:id/customer-invitations", middleware.RequirePermission(OEMRead), h.invitations)
	group.POST("/sites/:id/customer-invitations", middleware.RequirePermission(OEMWrite), middleware.UserCriticalRateLimit("cmh-oem-customer-invite"), h.invite)
	group.POST("/sites/:id/customers/:user_id/revoke", middleware.RequirePermission(OEMWrite), h.revoke)
	// These still inherit the strict official browser-session middleware, but no
	// merchant permission: acceptance never grants cmh_oem administration rights.
	group.GET("/customer-membership", h.self)
	group.POST("/customer-invitations/accept", middleware.UserCriticalRateLimit("cmh-oem-customer-accept"), h.accept)
}

type customerHTTP struct{}

func (customerHTTP) audits(c *gin.Context) {
	value, err := NewSiteStore(model.DB).CustomerAudits(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, value, err)
}
func (customerHTTP) list(c *gin.Context) {
	value, err := NewSiteStore(model.DB).Customers(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, value, err)
}
func (customerHTTP) invitations(c *gin.Context) {
	value, err := NewSiteStore(model.DB).CustomerInvitations(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, value, err)
}
func (customerHTTP) self(c *gin.Context) {
	value, err := NewSiteStore(model.DB).CustomerMembership(c.Request.Context(), c.GetInt("id"))
	oemResponse(c, gin.H{"membership": value}, err)
}

// Only one canonical member is accepted. No alternate identity field, case-fold
// alias, unknown member or duplicate decoded key is interpreted by Unmarshal.
func decodeCustomerInput(c *gin.Context, key string, target any) bool {
	if c.ContentType() != "application/json" {
		oemResponse(c, nil, ErrOEMInvalid)
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
	if err != nil || !utf8.Valid(data) || !ojson.ValidBytes(data) {
		oemResponse(c, nil, ErrOEMInvalid)
		return false
	}
	root := ojson.ParseBytes(data)
	count := 0
	valid := root.IsObject()
	root.ForEach(func(name, value ojson.Result) bool {
		count++
		valid = valid && name.String() == key
		if key == "token" {
			valid = valid && value.Type == ojson.String
		} else {
			valid = valid && value.Type == ojson.Number
		}
		return valid
	})
	if !valid || count != 1 || common.Unmarshal(data, target) != nil {
		oemResponse(c, nil, ErrOEMInvalid)
		return false
	}
	return true
}
func (customerHTTP) invite(c *gin.Context) {
	var input struct {
		TargetUserID int `json:"target_user_id"`
	}
	if !decodeCustomerInput(c, "target_user_id", &input) {
		return
	}
	value, err := NewSiteStore(model.DB).InviteCustomer(c.Request.Context(), c.GetInt("id"), c.Param("id"), input.TargetUserID)
	oemResponse(c, value, err)
}
func (customerHTTP) accept(c *gin.Context) {
	var input struct {
		Token string `json:"token"`
	}
	if !decodeCustomerInput(c, "token", &input) {
		return
	}
	value, err := NewSiteStore(model.DB).AcceptCustomerInvitation(c.Request.Context(), c.GetInt("id"), input.Token)
	oemResponse(c, value, err)
}
func (customerHTTP) note(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	value, err := NewSiteStore(model.DB).CustomerNote(c.Request.Context(), c.GetInt("id"), c.Param("id"), userID)
	oemResponse(c, value, err)
}
func (customerHTTP) updateNote(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 || c.ContentType() != "application/json" {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	if err != nil || !utf8.Valid(data) || !ojson.ValidBytes(data) {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	root := ojson.ParseBytes(data)
	valid := root.IsObject()
	seen := map[string]bool{}
	root.ForEach(func(name, value ojson.Result) bool {
		key := name.String()
		if seen[key] {
			valid = false
			return false
		}
		seen[key] = true
		valid = valid && ((key == "note" && value.Type == ojson.String) || (key == "version" && value.Type == ojson.Number))
		return valid
	})
	var input struct {
		Note    string `json:"note"`
		Version int64  `json:"version"`
	}
	if !valid || len(seen) != 2 || !seen["note"] || !seen["version"] || common.Unmarshal(data, &input) != nil {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	value, err := NewSiteStore(model.DB).UpdateCustomerNote(c.Request.Context(), c.GetInt("id"), c.Param("id"), userID, input.Note, input.Version)
	oemResponse(c, value, err)
}

func (customerHTTP) revoke(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	value, err := NewSiteStore(model.DB).RevokeCustomer(c.Request.Context(), c.GetInt("id"), c.Param("id"), userID)
	oemResponse(c, value, err)
}

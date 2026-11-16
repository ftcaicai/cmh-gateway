package brand

import (
	"strconv"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func mountOEMTeam(group *gin.RouterGroup) {
	h := teamHTTP{}
	group.GET("/sites/:id/team", middleware.RequirePermission(OEMRead), h.members)
	group.GET("/sites/:id/team-invitations", middleware.RequirePermission(OEMRead), h.invitations)
	group.GET("/sites/:id/team-audits", middleware.RequirePermission(OEMRead), h.audits)
	group.POST("/sites/:id/team-invitations", middleware.RequirePermission(OEMWrite), middleware.UserCriticalRateLimit("cmh-oem-team-invite"), h.invite)
	group.POST("/sites/:id/team/:user_id/revoke", middleware.RequirePermission(OEMWrite), h.revoke)
	group.POST("/team-invitations/accept", middleware.RequirePermission(OEMRead), middleware.UserCriticalRateLimit("cmh-oem-team-accept"), h.accept)
}

type teamHTTP struct{}

func (teamHTTP) members(c *gin.Context) {
	v, e := NewSiteStore(model.DB).TeamMembers(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, v, e)
}
func (teamHTTP) invitations(c *gin.Context) {
	v, e := NewSiteStore(model.DB).TeamInvitations(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, v, e)
}
func (teamHTTP) audits(c *gin.Context) {
	v, e := NewSiteStore(model.DB).TeamAudits(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, v, e)
}
func (teamHTTP) invite(c *gin.Context) {
	var input struct {
		TargetUserID int `json:"target_user_id"`
	}
	if !decodeCustomerInput(c, "target_user_id", &input) {
		return
	}
	v, e := NewSiteStore(model.DB).InviteTeamMember(c.Request.Context(), c.GetInt("id"), c.Param("id"), input.TargetUserID)
	oemResponse(c, v, e)
}
func (teamHTTP) accept(c *gin.Context) {
	var input struct {
		Token string `json:"token"`
	}
	if !decodeCustomerInput(c, "token", &input) {
		return
	}
	v, e := NewSiteStore(model.DB).AcceptTeamInvitation(c.Request.Context(), c.GetInt("id"), input.Token)
	oemResponse(c, v, e)
}
func (teamHTTP) revoke(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	var input struct {
		Version int64 `json:"version"`
	}
	if !decodeCustomerInput(c, "version", &input) {
		return
	}
	v, e := NewSiteStore(model.DB).RevokeTeamMember(c.Request.Context(), c.GetInt("id"), c.Param("id"), userID, input.Version)
	oemResponse(c, v, e)
}

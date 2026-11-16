package brand

import (
	"context"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// OEMCatalog is a read-only publication observation. It is neither an executor
// permission nor a brand-tenant policy, and never exposes channel identities.
type OEMCatalog struct {
	Models                   []string `json:"models"`
	ModelPolicyEnabled       bool     `json:"model_policy_enabled"`
	DestinationPolicyEnabled bool     `json:"destination_policy_enabled"`
}

// MountOEMCatalog requires the caller's official-session-only OEM route group.
// Policy registration is immutable at startup; this endpoint does not edit it.
func MountOEMCatalog(group *gin.RouterGroup) {
	group.GET("/catalog", middleware.RequirePermission(OEMRead), func(c *gin.Context) {
		value, err := readOEMCatalog(c.Request.Context(), c.GetInt("id"))
		oemResponse(c, value, err)
	})
}

func readOEMCatalog(ctx context.Context, userID int) (OEMCatalog, error) {
	result := OEMCatalog{
		Models:                   []string{},
		ModelPolicyEnabled:       core.ModelAdmissionEnabled(),
		DestinationPolicyEnabled: core.DestinationAdmissionEnabled(),
	}
	if model.DB == nil || userID <= 0 || ctx.Err() != nil {
		return result, ErrOEMUnavailable
	}
	// Exported DB readers do not accept a context. The boundary checks before
	// and after reads prevent publishing after cancellation, not abort DB I/O.
	user, err := model.GetUserById(userID, false)
	if err != nil || user == nil || user.Status != common.UserStatusEnabled || user.Group == "" {
		return result, ErrOEMUnavailable
	}
	group := user.Group
	// GetGroupEnabledModels intentionally swallows database errors. Use the
	// exported error-returning ability snapshot and the host's own admission
	// projection instead, so a failed query is not misreported as an empty list.
	abilities, err := model.GetAllEnableAbilityWithChannels()
	if err != nil {
		return result, ErrOEMUnavailable
	}
	channels := make(map[int]*model.Channel)
	visible := make(map[string]bool)
	for _, ability := range abilities {
		if ability.Group != group {
			continue
		}
		channel, loaded := channels[ability.ChannelId]
		if !loaded {
			channel, err = model.GetChannelById(ability.ChannelId, false)
			if err != nil || channel == nil {
				return result, ErrOEMUnavailable
			}
			channels[ability.ChannelId] = channel
		}
		if channel.Status != common.ChannelStatusEnabled || visible[ability.Model] {
			continue
		}
		if !result.ModelPolicyEnabled || core.Models.Visible(ctx, model.ChannelModelAdmission(channel, ability.Model, core.AdmissionCandidate)) {
			visible[ability.Model] = true
			result.Models = append(result.Models, ability.Model)
		}
	}
	if ctx.Err() != nil {
		return OEMCatalog{}, ErrOEMUnavailable
	}
	slices.Sort(result.Models)
	return result, nil
}

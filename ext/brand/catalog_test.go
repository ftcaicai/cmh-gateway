package brand

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type catalogPublicationGate struct{}

func (catalogPublicationGate) Check(context.Context, core.Admission) error { return nil }
func (catalogPublicationGate) ValidateChannels(context.Context, []core.ChannelSpec) error {
	return nil
}
func (catalogPublicationGate) Visible(_ context.Context, value core.Admission) bool {
	return value.Model.Public == "public-allowed" && value.Phase == core.AdmissionCandidate
}

func TestOEMCatalogOfficialGroupPublication(t *testing.T) {
	oldDB, oldRedis, oldGate := model.DB, common.RedisEnabled, core.Models
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/catalog.sqlite"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, core.Models = oldDB, oldRedis, oldGate
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))
	user := model.User{Username: "catalog-owner", AffCode: "catalog-owner", Group: "catalog-own"}
	require.NoError(t, db.Create(&user).Error)
	channels := []model.Channel{
		{Id: 1, Type: 1, Status: common.ChannelStatusEnabled, Models: "public-allowed,policy-denied", Group: "catalog-own", Key: "synthetic-not-for-response"},
		{Id: 2, Type: 1, Status: common.ChannelStatusEnabled, Models: "other-only", Group: "catalog-other"},
		{Id: 3, Type: 1, Status: common.ChannelStatusManuallyDisabled, Models: "disabled-channel", Group: "catalog-own"},
	}
	require.NoError(t, db.Create(&channels).Error)
	abilities := []model.Ability{
		{Group: "catalog-own", Model: "public-allowed", ChannelId: 1, Enabled: true},
		{Group: "catalog-own", Model: "policy-denied", ChannelId: 1, Enabled: true},
		{Group: "catalog-other", Model: "other-only", ChannelId: 2, Enabled: true},
		{Group: "catalog-own", Model: "disabled-channel", ChannelId: 3, Enabled: true},
		{Group: "catalog-own", Model: "disabled-ability", ChannelId: 1, Enabled: false},
	}
	require.NoError(t, db.Create(&abilities).Error)
	// The upstream-compatible mode still publishes only this user's official
	// group, not a group selected through query parameters or brand membership.
	value, err := readOEMCatalog(context.Background(), user.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-denied", "public-allowed"}, value.Models)
	assert.False(t, value.ModelPolicyEnabled)
	// Ability rows are the official publication authority. This read-only view
	// does not invent a second catalog from channel.Models/group strings.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"models": "changed", "group": "changed"}).Error)
	value, err = readOEMCatalog(context.Background(), user.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-denied", "public-allowed"}, value.Models)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"models": "public-allowed,policy-denied", "group": "catalog-own"}).Error)

	core.Models = catalogPublicationGate{}
	value, err = readOEMCatalog(context.Background(), user.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"public-allowed"}, value.Models)
	assert.True(t, value.ModelPolicyEnabled)
	assert.Equal(t, core.DestinationAdmissionEnabled(), value.DestinationPolicyEnabled)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, common.Unmarshal(encoded, &wire))
	assert.Len(t, wire, 3)
	assert.NotContains(t, string(encoded), "synthetic-not-for-response")
	assert.NotContains(t, string(encoded), "catalog-own")

	// A cancellation or missing required table is unavailable, never a
	// successful empty catalog. Exercise the actual safe HTTP error envelope.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = readOEMCatalog(ctx, user.Id)
	assert.ErrorIs(t, err, ErrOEMUnavailable)
	_, err = readOEMCatalog(context.Background(), 9999)
	assert.ErrorIs(t, err, ErrOEMUnavailable)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error)
	_, err = readOEMCatalog(context.Background(), user.Id)
	assert.ErrorIs(t, err, ErrOEMUnavailable)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusEnabled).Error)
	require.NoError(t, db.Migrator().DropTable(&model.Ability{}))
	_, err = readOEMCatalog(context.Background(), user.Id)
	assert.ErrorIs(t, err, ErrOEMUnavailable)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	oemResponse(c, nil, err)
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"success":false,"code":"OEM_UNAVAILABLE"}`, response.Body.String())
	model.DB = nil
	_, err = readOEMCatalog(context.Background(), user.Id)
	assert.ErrorIs(t, err, ErrOEMUnavailable)
}

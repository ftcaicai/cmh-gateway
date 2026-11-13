package relay

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// CheckRequestAdmission checks a selected request against current persisted channel
// identity. It does not apply model mapping or mutate the request/billing state.
func CheckRequestAdmission(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if !core.ModelAdmissionEnabled() {
		return nil
	}
	rejected := types.NewErrorWithStatusCode(errors.New("request rejected"), types.ErrorCodeInvalidRequest, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	if c == nil || info == nil || info.ChannelMeta == nil || info.OriginModelName == "" || info.ChannelId <= 0 {
		return rejected
	}
	if info.ChannelId != common.GetContextKeyInt(c, constant.ContextKeyChannelId) ||
		info.ChannelType != common.GetContextKeyInt(c, constant.ContextKeyChannelType) ||
		info.OriginModelName != common.GetContextKeyString(c, constant.ContextKeyOriginalModel) {
		return rejected
	}
	if model.DB == nil {
		return rejected
	}
	channel, err := model.GetChannelById(info.ChannelId, false)
	if err != nil || channel == nil || channel.Id != info.ChannelId || channel.Type != info.ChannelType || channel.Status != common.ChannelStatusEnabled {
		return rejected
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "auto" {
		group = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	}
	if group != "" && !slices.Contains(strings.Split(channel.Group, ","), group) {
		return rejected
	}
	filters := slices.Clone(service.GetChannelConstraints(c).Filters)
	if value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin); exists {
		pinned, ok := value.(jsplugin.PinnedPlugin)
		if !ok || pinned.Plugin == nil || pinned.Plugin.Meta.Key == "" {
			return rejected
		}
		// A selected legacy task executor must not become builtin merely because
		// the current row or its model declaration no longer names the plugin.
		filters = append(filters, hostdto.ChannelFilter{Kind: hostdto.FilterTaskPluginIdentity, TaskPluginKey: pinned.Plugin.Meta.Key})
	}
	admission := model.ChannelModelAdmission(channel, info.OriginModelName, core.AdmissionAttempt, filters...)
	// The handler consumes the selected context mapping, not this fresh DB row.
	// A policy witness for a repaired row must not authorize an older context
	// that would still send a different upstream model.
	var selectedMapping map[string]string
	if raw := common.GetContextKeyString(c, constant.ContextKeyChannelModelMapping); raw != "" && raw != "{}" {
		if common.UnmarshalJsonStr(raw, &selectedMapping) != nil {
			return rejected
		}
	}
	selectedUpstream, mappingErr := model.ResolveChannelModel(info.OriginModelName, selectedMapping)
	if mappingErr != nil || selectedUpstream != admission.Model.Upstream {
		return rejected
	}
	var ctx context.Context = c
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	if err = core.Models.Check(ctx, admission); err != nil {
		return rejected
	}
	return nil
}

// PrepareRequestBilling estimates and reserves one request's charge. Transports
// provide the current request body through BodyStorage or BillingRequestInput;
// channel retries retain the resulting billing session and pricing snapshot.
func PrepareRequestBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if core.ModelAdmissionEnabled() {
		admissionInfo := info
		if c != nil && info != nil && info.ChannelMeta == nil {
			// Initial HTTP/WS billing precedes the handler's InitChannelMeta.
			// Project selection only; do not initialize or rewrite the request.
			snapshot := *info
			snapshot.ChannelMeta = &relaycommon.ChannelMeta{
				ChannelId:   common.GetContextKeyInt(c, constant.ContextKeyChannelId),
				ChannelType: common.GetContextKeyInt(c, constant.ContextKeyChannelType),
			}
			admissionInfo = &snapshot
		}
		if apiErr := CheckRequestAdmission(c, admissionInfo); apiErr != nil {
			return apiErr
		}
	}
	meta := &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
	if info.Request != nil && constant.CountToken {
		meta = info.Request.GetTokenCountMeta()
	} else {
		// Avoid building CombineText when only the pricing quantities are needed.
		switch request := info.Request.(type) {
		case *dto.GeneralOpenAIRequest:
			meta.MaxTokens = int(max(lo.FromPtr(request.MaxTokens), lo.FromPtr(request.MaxCompletionTokens)))
		case *dto.OpenAIResponsesRequest:
			meta.MaxTokens = int(lo.FromPtr(request.MaxOutputTokens))
		case *dto.ClaudeRequest:
			meta.MaxTokens = int(lo.FromPtr(request.MaxTokens))
		case *dto.ImageRequest:
			meta = request.GetTokenCountMeta()
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, info)
	if err != nil {
		return types.NewError(err, types.ErrorCodeCountTokenFailed)
	}
	info.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, info, tokens, meta)
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
	}
	if priceData.FreeModel {
		logger.LogInfo(c, common.LogText("model %s is free, skipping pre-consume", info.OriginModelName))
		return nil
	}
	return service.PreConsumeBilling(c, priceData.QuotaToPreConsume, info)
}

// RefundFailedRequestBilling applies the common final-failure policy after all
// eligible attempts have ended. A settled BillingSession never refunds again.
func RefundFailedRequestBilling(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) *types.NewAPIError {
	if apiErr == nil {
		return nil
	}
	apiErr = service.NormalizeViolationFeeError(apiErr)
	if info.Billing != nil {
		info.Billing.Refund(c)
	}
	service.ChargeViolationFeeIfNeeded(c, info, apiErr)
	return apiErr
}

package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// CheckRealtimeAdmission checks the fixed handshake identity against current
// channel authority without remapping or changing session/accounting state.
// A nil message checks the handshake; event model fields must name that exact
// fixed upstream identity, not a public alias requiring another mapping.
func CheckRealtimeAdmission(c *gin.Context, info *relaycommon.RelayInfo, message []byte) *types.NewAPIError {
	if !core.ModelAdmissionEnabled() {
		return nil
	}
	rejected := types.NewErrorWithStatusCode(errors.New("request rejected"), types.ErrorCodeInvalidRequest, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	if c == nil || info == nil || info.ChannelMeta == nil || info.OriginModelName == "" || info.UpstreamModelName == "" || info.ChannelId <= 0 || model.DB == nil {
		return rejected
	}
	if info.ChannelId != common.GetContextKeyInt(c, constant.ContextKeyChannelId) ||
		info.ChannelType != common.GetContextKeyInt(c, constant.ContextKeyChannelType) ||
		info.OriginModelName != common.GetContextKeyString(c, constant.ContextKeyOriginalModel) {
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
	if group == "" || !slices.Contains(strings.Split(channel.Group, ","), group) || info.UsingGroup != "" && info.UsingGroup != "auto" && info.UsingGroup != group {
		return rejected
	}
	admission := model.ChannelModelAdmission(channel, info.OriginModelName, core.AdmissionDispatch, service.GetChannelConstraints(c).Filters...)
	var selectedMapping map[string]string
	if raw := common.GetContextKeyString(c, constant.ContextKeyChannelModelMapping); raw != "" && raw != "{}" {
		if common.UnmarshalJsonStr(raw, &selectedMapping) != nil {
			return rejected
		}
	}
	selectedUpstream, err := model.ResolveChannelModel(info.OriginModelName, selectedMapping)
	// Realtime handshakes need not run ModelMappedHelper: a fresh mapped row
	// cannot authorize a socket actually opened with the original model.
	if err != nil || selectedUpstream != admission.Model.Upstream || selectedUpstream != info.UpstreamModelName {
		return rejected
	}
	taskValue, taskPinned := c.Get(jsplugin.ContextKeyPinnedPlugin)
	providerValue, providerPinned := common.GetContextKey(c, constant.ContextKeyProviderPlugin)
	planValue, requestPlan := c.Get("request_plugin_plan")
	if taskPinned && taskValue != nil || providerPinned && providerValue != nil || requestPlan && planValue != nil || len(info.ChannelSetting.RequestPlugins) > 0 {
		// A retained plan may outlive a settings change; current settings alone
		// cannot prove this session executes builtin-only work.
		admission.Executor = core.ExecutorIdentity{}
	}
	var ctx context.Context = c
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	if message != nil {
		var event struct {
			Type     string                     `json:"type"`
			Session  map[string]json.RawMessage `json:"session"`
			Response map[string]json.RawMessage `json:"response"`
		}
		if common.Unmarshal(message, &event) != nil {
			return rejected
		}
		var members map[string]json.RawMessage
		switch event.Type {
		case dto.RealtimeEventTypeSessionUpdate:
			members = event.Session
		case dto.RealtimeEventTypeResponseCreate:
			members = event.Response
		}
		if event.Type == dto.RealtimeEventTypeSessionUpdate {
			// Transcription chooses another model, not the fixed realtime model.
			// Authorize that explicit target separately under the same channel and
			// executor policy; never let it inherit the main socket's approval.
			var nested struct {
				Session struct {
					Legacy struct {
						Model string `json:"model"`
					} `json:"input_audio_transcription"`
					Audio struct {
						Input struct {
							Transcription struct {
								Model string `json:"model"`
							} `json:"transcription"`
						} `json:"input"`
					} `json:"audio"`
				} `json:"session"`
			}
			if common.Unmarshal(message, &nested) != nil {
				return rejected
			}
			for _, target := range []string{nested.Session.Legacy.Model, nested.Session.Audio.Input.Transcription.Model} {
				if target == "" {
					continue
				}
				transcription := admission
				transcription.Model = core.ModelIdentity{Public: target, Upstream: target}
				if core.Models.Check(ctx, transcription) != nil {
					return rejected
				}
			}
		}
		if raw, present := members["model"]; present {
			var name string
			if common.Unmarshal(raw, &name) != nil || name != info.UpstreamModelName {
				return rejected
			}
		}
	}
	if core.Models.Check(ctx, admission) != nil {
		return rejected
	}
	return nil
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	// The request plugins answer the client and cancel upstream responses
	// from either reader, so each socket takes one writer at a time.
	var clientWriteMu, targetWriteMu sync.Mutex
	writeClient := func(message []byte) error {
		clientWriteMu.Lock()
		defer clientWriteMu.Unlock()
		return helper.WssString(c, clientConn, string(message))
	}
	writeTarget := func(message []byte) error {
		targetWriteMu.Lock()
		defer targetWriteMu.Unlock()
		return helper.WssString(c, targetConn, string(message))
	}
	if apiErr := CheckRealtimeAdmission(c, info, nil); apiErr != nil {
		return apiErr, nil
	}
	admissionDenied := func(message []byte) bool {
		if CheckRealtimeAdmission(c, info, message) == nil {
			return false
		}
		var event struct {
			EventID string `json:"event_id"`
		}
		_ = common.Unmarshal(message, &event)
		response, _ := common.Marshal(gin.H{
			"type": "error", "event_id": "event_" + common.GetUUID(),
			"error": gin.H{"type": "invalid_request_error", "code": "request_rejected", "message": "request rejected", "event_id": event.EventID},
		})
		if err := writeClient(response); err != nil {
			select {
			case errChan <- fmt.Errorf("error writing to client: %v", err):
			default:
			}
		}
		return true
	}
	plugins := requestplugin.NewRealtime(c, info, func(message []byte) {
		if err := writeClient(message); err != nil {
			select {
			case errChan <- fmt.Errorf("error writing to client: %v", err):
			default:
			}
		}
	}, func(message []byte) {
		if err := writeTarget(message); err != nil {
			select {
			case errChan <- fmt.Errorf("error writing to target: %v", err):
			default:
			}
		}
	})
	if plugins != nil {
		// The consume log reads what the plugins did once the session ends.
		defer plugins.Close(info)
	}

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				if admissionDenied(message) {
					continue
				}
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}
				if plugins != nil {
					patched, dropped := plugins.ClientEvent(realtimeEvent.Type, message)
					if dropped {
						continue
					}
					if patched != nil {
						message, realtimeEvent = patched, &dto.RealtimeEvent{}
						if err = common.Unmarshal(message, realtimeEvent); err != nil {
							errChan <- fmt.Errorf("error unmarshalling message: %v", err)
							return
						}
					}
				}

				if admissionDenied(message) {
					continue
				}
				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
					if realtimeEvent.Session != nil {
						if realtimeEvent.Session.Tools != nil {
							info.RealtimeTools = realtimeEvent.Session.Tools
						}
					}
				}

				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err != nil {
					errChan <- fmt.Errorf("error counting text token: %v", err)
					return
				}
				logger.LogInfo(c, common.LogText("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken

				err = writeTarget(message)
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				// quota is what a response.done was charged.
				quota := 0
				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					realtimeUsage := realtimeEvent.Response.Usage
					if realtimeUsage != nil {
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						quota, err = preConsumeUsage(c, info, usage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						usage = &dto.RealtimeUsage{}

						localUsage = &dto.RealtimeUsage{}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							errChan <- fmt.Errorf("error counting text token: %v", err)
							return
						}
						logger.LogInfo(c, common.LogText("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						quota, err = preConsumeUsage(c, info, localUsage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						localUsage = &dto.RealtimeUsage{}
						// print now usage
					}
					logger.LogInfo(c, common.LogText("realtime streaming sumUsage: %v", sumUsage))
					logger.LogInfo(c, common.LogText("realtime streaming localUsage: %v", localUsage))
					logger.LogInfo(c, common.LogText("realtime streaming localUsage: %v", localUsage))

				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					realtimeSession := realtimeEvent.Session
					if realtimeSession != nil {
						// update audio format
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						errChan <- fmt.Errorf("error counting text token: %v", err)
						return
					}
					logger.LogInfo(c, common.LogText("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
				}

				if plugins != nil {
					var responseUsage *dto.RealtimeUsage
					if realtimeEvent.Response != nil {
						responseUsage = realtimeEvent.Response.Usage
					}
					plugins.ServerEvent(realtimeEvent.Type, message, responseUsage, quota)
				} else if err = writeClient(message); err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		//return service.OpenAIErrorWrapper(err, "realtime_error", http.StatusInternalServerError), nil
		logger.LogError(c, common.LogText("realtime error: %s", err.Error()))
	case <-c.Done():
	}

	if usage.TotalTokens != 0 {
		_, _ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_, _ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	// check usage total tokens, if 0, use local usage

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) (int, error) {
	if usage == nil || totalUsage == nil {
		return 0, fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens
	// clear usage
	return service.PreWssConsumeQuota(ctx, info, usage)
}

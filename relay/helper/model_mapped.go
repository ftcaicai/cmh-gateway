package helper

import (
	"fmt"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]string)
		err := rootcommon.Unmarshal([]byte(modelMapping), &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		currentModel, err := model.ResolveChannelModel(info.OriginModelName, modelMap)
		if currentModel != info.OriginModelName {
			info.IsModelMapped = true
		}
		if err != nil {
			return err
		}
		if currentModel == info.OriginModelName {
			// The original helper returns before touching the request for an
			// initial self-map, including one found through the base-name key.
			mapped, exists := modelMap[currentModel]
			base := hostreasoning.BaseModelName(currentModel)
			if (!exists || mapped == "") && base != currentModel {
				mapped, exists = modelMap[base]
			}
			if exists && mapped != "" && mapped == currentModel {
				info.IsModelMapped = false
				return nil
			}
		}
		if info.IsModelMapped {
			info.UpstreamModelName = currentModel
		}
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}

package model

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/setting/request_plugin_setting"
)

// ResolveChannelModel follows the host's model mapping, including base-name
// fallback at every hop. A self-map terminates the chain; any other cycle fails.
// On a cycle, the returned name is the last successfully resolved hop.
func ResolveChannelModel(public string, mapping map[string]string) (string, error) {
	current := public
	visited := map[string]bool{current: true}
	for {
		mapped, exists := mapping[current]
		base := reasoning.BaseModelName(current)
		if (!exists || mapped == "") && base != current {
			mapped, exists = mapping[base]
		}
		if !exists || mapped == "" {
			return current, nil
		}
		if visited[mapped] {
			if mapped == current {
				return current, nil
			}
			return current, errors.New("model_mapping_contains_cycle")
		}
		visited[mapped] = true
		current = mapped
	}
}

// ChannelModelAdmission projects only identities proven by the host. This first
// subset does not resolve plugin execution plans or their approved artifacts;
// those candidates deliberately retain an unknown executor, never "builtin".
func ChannelModelAdmission(ch *Channel, public string, phase core.AdmissionPhase, filters ...dto.ChannelFilter) core.Admission {
	admission := core.Admission{Phase: phase, Model: core.ModelIdentity{Public: public}}
	if ch == nil {
		return admission
	}
	admission.Channel = core.ChannelIdentity{Known: true, ID: ch.Id, Type: ch.Type}
	if phase != core.AdmissionChannelSave && ch.Status != common.ChannelStatusEnabled {
		return admission
	}
	if phase != core.AdmissionChannelSave {
		names := ch.GetModels()
		if !slices.Contains(names, public) && !slices.Contains(names, ratio_setting.RoutingMatchModelName(public)) {
			return admission
		}
	}
	var mapping map[string]string
	if raw := ch.GetModelMapping(); raw != "" && raw != "{}" {
		if common.UnmarshalJsonStr(raw, &mapping) != nil {
			return admission
		}
	}
	upstream, err := ResolveChannelModel(public, mapping)
	if err != nil {
		return admission
	}
	admission.Model.Upstream = upstream
	// Request constraints can pin a legacy plugin even for an undeclared model.
	// This subset cannot attest that executor, so never label it builtin.
	for _, filter := range filters {
		if filter.Kind == dto.FilterTaskPluginIdentity && filter.TaskPluginKey != "" ||
			filter.Kind == dto.FilterTaskPluginEndpoint && !filter.TaskPluginRelayOnly && len(filter.TaskPluginKeys) > 0 {
			return admission
		}
	}

	// GetSetting repairs malformed settings in the database. Admission is a
	// read-only projection, so decode directly and withhold an unknown snapshot.
	var setting kitdto.ChannelSettings
	if ch.Setting != nil && *ch.Setting != "" {
		if common.UnmarshalJsonStr(*ch.Setting, &setting) != nil {
			return admission
		}
	}
	if ch.Type == constant.ChannelTypeTaskPlugin || setting.TaskPluginKey != "" || len(setting.TaskExtendPluginKeys) > 0 || len(setting.RequestPlugins) > 0 {
		return admission
	}
	if plugin, owned := jsplugin.DefaultRegistry.Generation().GetByChannelType(ch.Type); owned {
		for _, declared := range plugin.Meta.Models {
			if strings.EqualFold(declared, public) || strings.EqualFold(declared, upstream) || strings.EqualFold(declared, reasoning.BaseModelName(public)) || strings.EqualFold(declared, reasoning.BaseModelName(upstream)) {
				return admission
			}
		}
	}
	// Global/group request plugins are not encoded in channel settings. Until
	// the execution-plan boundary is wired, do not claim builtin-only execution
	// in a process with enabled request plugin instances, even for a narrow scope.
	for _, instance := range request_plugin_setting.GetInstances() {
		if instance.Enabled {
			return admission
		}
	}
	admission.Executor.Kind = "builtin"
	return admission
}

var filterEvalOrder = []dto.ChannelFilterKind{
	dto.FilterRequestPath,
	dto.FilterTaskPluginEndpoint,
	dto.FilterTaskPluginIdentity,
	dto.FilterResponsesWebSocket,
}

// ChannelSatisfiesFilters reports whether ch passes every filter.
// On false, it returns the first violated filter in filterEvalOrder for error
// attribution.
func ChannelSatisfiesFilters(ch *Channel, modelName string, filters []dto.ChannelFilter) (bool, dto.ChannelFilterKind) {
	if ch == nil {
		return false, ""
	}
	if core.ModelAdmissionEnabled() && core.Models.Check(context.Background(), ChannelModelAdmission(ch, modelName, core.AdmissionCandidate, filters...)) != nil {
		return false, ""
	}
	for _, kind := range filterEvalOrder {
		for _, filter := range filters {
			if filter.Kind != kind {
				continue
			}
			if !channelMatchesFilter(ch, modelName, filter) {
				return false, kind
			}
		}
	}
	return true, ""
}

// filterCandidateIDs applies filters to a cached candidate id list.
// Caller must hold channelSyncLock (read lock). The input slice is never mutated.
// A missing id in channelsIDM is kept for request_path (downstream consistency
// error) and dropped for the other filters. Active admission always drops it
// because no authoritative candidate identity can be projected.
func filterCandidateIDs(ids []int, modelName string, filters []dto.ChannelFilter) (kept []int, emptiedBy dto.ChannelFilterKind) {
	if len(ids) == 0 {
		return ids, ""
	}
	kept = ids
	if core.ModelAdmissionEnabled() {
		kept = make([]int, 0, len(ids))
		for _, id := range ids {
			channel := channelsIDM[id]
			if channel != nil && core.Models.Check(context.Background(), ChannelModelAdmission(channel, modelName, core.AdmissionCandidate, filters...)) == nil {
				kept = append(kept, id)
			}
		}
	}
	for _, kind := range filterEvalOrder {
		kindFilters := filtersByKind(filters, kind)
		if len(kindFilters) == 0 {
			continue
		}
		next := make([]int, 0, len(kept))
		for _, id := range kept {
			channel, exists := channelsIDM[id]
			if candidatePassesKindFilters(channel, exists, modelName, kind, kindFilters) {
				next = append(next, id)
			}
		}
		if len(kept) > 0 && len(next) == 0 {
			return next, kind
		}
		kept = next
	}
	return kept, ""
}

func filtersByKind(filters []dto.ChannelFilter, kind dto.ChannelFilterKind) []dto.ChannelFilter {
	var matched []dto.ChannelFilter
	for _, filter := range filters {
		if filter.Kind == kind {
			matched = append(matched, filter)
		}
	}
	return matched
}

func candidatePassesKindFilters(ch *Channel, exists bool, modelName string, kind dto.ChannelFilterKind, filters []dto.ChannelFilter) bool {
	if kind == dto.FilterRequestPath && !exists {
		return true
	}
	if !exists || ch == nil {
		return false
	}
	for _, filter := range filters {
		if !channelMatchesFilter(ch, modelName, filter) {
			return false
		}
	}
	return true
}

func channelMatchesFilter(ch *Channel, modelName string, filter dto.ChannelFilter) bool {
	switch filter.Kind {
	case dto.FilterRequestPath:
		if filter.RequestPath == "" {
			return true
		}
		if !constant.IsAdvancedCustomChannel(ch.Type) {
			return true
		}
		config := ch.GetOtherSettings().AdvancedCustom
		return config != nil && config.SupportsPathForModel(filter.RequestPath, modelName)
	case dto.FilterTaskPluginIdentity:
		if filter.TaskPluginKey == "" {
			return ch.Type != constant.ChannelTypeTaskPlugin || providerChannelServes(ch, filter.RequestPath, modelName)
		}
		if ch.Type == constant.ChannelTypeTaskPlugin || ch.Type == constant.ChannelTypeNewAPI {
			// A New API channel serves every plugin it is extended with; the
			// pinned plugin or any shared-model candidate may execute there.
			setting := ch.GetSetting()
			return setting.BindsTaskPlugin(filter.TaskPluginKey) || slices.ContainsFunc(filter.TaskPluginKeys, setting.BindsTaskPlugin)
		}
		return slices.Contains(filter.TaskPluginChannelTypes, ch.Type)
	case dto.FilterTaskPluginEndpoint:
		pluginChannel := slices.Contains(filter.TaskPluginChannelTypes, ch.Type)
		if ch.Type == constant.ChannelTypeTaskPlugin || ch.Type == constant.ChannelTypeNewAPI {
			pluginChannel = slices.ContainsFunc(filter.TaskPluginKeys, ch.GetSetting().BindsTaskPlugin)
		}
		relayChannel := ch.Type != constant.ChannelTypeTaskPlugin || providerChannelServes(ch, filter.RequestPath, modelName)
		if filter.TaskPluginRelayOnly {
			return relayChannel && !pluginChannel
		}
		// Ordinary channels and matching plugin channels compete using the same
		// group, priority, weight, affinity, and pin rules. Unrelated type-61
		// plugins cannot execute this endpoint unless their relay routes do.
		return relayChannel || pluginChannel
	case dto.FilterResponsesWebSocket:
		if !ch.GetSetting().ResponsesWebSocketEnabled {
			return false
		}
		switch ch.Type {
		case constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeCodex, constant.ChannelTypeSub2API, constant.ChannelTypeNewAPI:
			return true
		case constant.ChannelTypeAdvancedCustom:
			// The session forwards native Responses events without protocol
			// conversion, so only a converter-free /v1/responses route qualifies.
			route, ok := ch.GetOtherSettings().AdvancedCustom.MatchPathForModel("/v1/responses", modelName)
			return ok && route.IsNative()
		case constant.ChannelTypeTaskPlugin:
			// The same rule on the relay routes of the channel's plugin.
			plugin, ok := jsplugin.DefaultRegistry.Get(ch.GetSetting().TaskPluginKey)
			return ok && plugin.Meta.ServesResponsesWebSocket(modelName)
		default:
			return false
		}
	default:
		return true
	}
}

// providerChannelServes reports a task plugin channel whose plugin relays
// requestPath for modelName through its relay routes, as an Advanced Custom
// channel with those routes would.
func providerChannelServes(ch *Channel, requestPath, modelName string) bool {
	if ch.Type != constant.ChannelTypeTaskPlugin || requestPath == "" {
		return false
	}
	plugin, ok := jsplugin.DefaultRegistry.Get(ch.GetSetting().TaskPluginKey)
	if !ok || !plugin.Meta.HasProviderPart() {
		return false
	}
	_, ok = plugin.Meta.RelayRoute(requestPath, modelName)
	return ok
}

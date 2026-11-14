package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strconv"
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

// channelModelProjection resolves only authoritative channel/model/mapping values.
// Executor identity is supplied by the caller's explicit publication or runtime role.
func channelModelProjection(ch *Channel, public string, phase core.AdmissionPhase) core.Admission {
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
	return admission
}

// pluginCandidateView retains the actual generation object. Neither its number
// nor a later registry lookup may substitute a different execution artifact.
type pluginCandidateView struct {
	generation *jsplugin.RoutingGeneration
}

func NewPluginCandidateView(generation *jsplugin.RoutingGeneration) dto.PluginCandidateView {
	return &pluginCandidateView{generation: generation}
}

func (view *pluginCandidateView) SameGeneration(other dto.PluginCandidateView) bool {
	candidate, ok := other.(*pluginCandidateView)
	return ok && view != nil && candidate != nil && view.generation != nil && view.generation == candidate.generation
}

func candidateSnapshot(ch *Channel) dto.ChannelCandidateSnapshot {
	return dto.ChannelCandidateSnapshot{
		ID: ch.Id, Type: ch.Type, Status: ch.Status, Models: ch.Models, Group: ch.Group,
		BaseURL: stringPointerValue(ch.BaseURL), BaseURLPresent: ch.BaseURL != nil, Other: ch.Other,
		Setting: stringPointerValue(ch.Setting), OtherSettings: ch.OtherSettings,
		ModelMapping: stringPointerValue(ch.ModelMapping), ParamOverride: stringPointerValue(ch.ParamOverride), HeaderOverride: stringPointerValue(ch.HeaderOverride),
	}
}

func channelFromCandidate(snapshot dto.ChannelCandidateSnapshot) *Channel {
	ch := &Channel{Id: snapshot.ID, Type: snapshot.Type, Status: snapshot.Status, Models: snapshot.Models, Group: snapshot.Group,
		Other: snapshot.Other, Setting: &snapshot.Setting, OtherSettings: snapshot.OtherSettings,
		ModelMapping: &snapshot.ModelMapping, ParamOverride: &snapshot.ParamOverride, HeaderOverride: &snapshot.HeaderOverride}
	if snapshot.BaseURLPresent {
		ch.BaseURL = &snapshot.BaseURL
	}
	return ch
}

func candidatePluginRole(setting kitdto.ChannelSettings, filter dto.ChannelFilter) string {
	if filter.Kind == dto.FilterTaskPluginIdentity && filter.TaskPluginKey == "" {
		return "provider"
	}
	if filter.Kind == dto.FilterTaskPluginIdentity && filter.TaskPluginKey == setting.TaskPluginKey ||
		filter.Kind == dto.FilterTaskPluginEndpoint && !filter.TaskPluginRelayOnly && slices.Contains(filter.TaskPluginKeys, setting.TaskPluginKey) {
		return "task"
	}
	return ""
}

func (view *pluginCandidateView) Admission(snapshot dto.ChannelCandidateSnapshot, public string, phase core.AdmissionPhase, filter dto.ChannelFilter) (core.Admission, error) {
	ch := channelFromCandidate(snapshot)
	admission := channelModelProjection(ch, public, phase)
	if view == nil || view.generation == nil || ch.Type != constant.ChannelTypeTaskPlugin || (phase != core.AdmissionCandidate && phase != core.AdmissionAttempt) {
		return admission, ErrChannelAdmissionDenied
	}
	var setting kitdto.ChannelSettings
	if common.UnmarshalJsonStr(snapshot.Setting, &setting) != nil {
		return admission, ErrChannelAdmissionDenied
	}
	role := candidatePluginRole(setting, filter)
	loaded, found := view.generation.Get(setting.TaskPluginKey)
	if !found || role == "" || filter.RequestPath == "" {
		return admission, ErrChannelAdmissionDenied
	}
	return ChannelPluginAdmission(ch, public, phase, jsplugin.PinnedPlugin{Generation: view.generation, Plugin: loaded}, role, filter.RequestPath)
}

func (view *pluginCandidateView) Matches(snapshot dto.ChannelCandidateSnapshot, public string, filter dto.ChannelFilter) bool {
	if view == nil || view.generation == nil || snapshot.Type != constant.ChannelTypeTaskPlugin {
		return false
	}
	var setting kitdto.ChannelSettings
	if common.UnmarshalJsonStr(snapshot.Setting, &setting) != nil {
		return false
	}
	if _, found := view.generation.Get(setting.TaskPluginKey); !found {
		return false
	}
	if snapshot.OtherSettings != "" {
		var other kitdto.ChannelOtherSettings
		if common.UnmarshalJsonStr(snapshot.OtherSettings, &other) != nil {
			return false
		}
	}
	filter.PluginView = nil
	return channelMatchesFilterInGeneration(channelFromCandidate(snapshot), public, filter, view.generation)
}

// coherentCandidateView rejects mixed snapshots even when a later filter would
// otherwise be ignored. Nil filters inherit a supplied view, never current code.
func coherentCandidateView(filters []dto.ChannelFilter) (dto.PluginCandidateView, bool) {
	var view dto.PluginCandidateView
	for _, filter := range filters {
		if filter.PluginView == nil {
			continue
		}
		if view == nil {
			view = filter.PluginView
		}
		if !view.SameGeneration(filter.PluginView) || !filter.PluginView.SameGeneration(view) {
			return nil, false
		}
	}
	return view, true
}

// ChannelModelAdmission projects an explicit executor role when the host has a
// witness. Publication approval is distinct from permission to execute any route.
func ChannelModelAdmission(ch *Channel, public string, phase core.AdmissionPhase, filters ...dto.ChannelFilter) core.Admission {
	admission := channelModelProjection(ch, public, phase)
	if ch == nil || admission.Model.Upstream == "" {
		return admission
	}
	upstream := admission.Model.Upstream
	if ch.Type == constant.ChannelTypeTaskPlugin && phase != core.AdmissionChannelSave && len(filters) > 0 {
		view, coherent := coherentCandidateView(filters)
		if !coherent {
			return admission
		}
		var setting kitdto.ChannelSettings
		if ch.Setting != nil && common.UnmarshalJsonStr(*ch.Setting, &setting) == nil {
			if view != nil {
				for _, filter := range filters {
					if candidatePluginRole(setting, filter) == "" || filter.RequestPath == "" {
						continue
					}
					projected, err := view.Admission(candidateSnapshot(ch), public, phase, filter)
					if err == nil {
						return projected
					}
					return admission
				}
				return admission
			}
			// No pinned view was supplied by this caller. Preserve the current
			// generation projection; an explicit view must never fall back here.
			generation := jsplugin.DefaultRegistry.Generation()
			if loaded, ok := generation.Get(setting.TaskPluginKey); ok {
				for _, filter := range filters {
					role := candidatePluginRole(setting, filter)
					if role != "" && filter.RequestPath != "" {
						projected, err := ChannelPluginAdmission(ch, public, phase, jsplugin.PinnedPlugin{Generation: generation, Plugin: loaded}, role, filter.RequestPath)
						if err == nil {
							return projected
						}
						return admission
					}
				}
			}
		}
	}
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
	if ch.Type == constant.ChannelTypeTaskPlugin && len(filters) == 0 && (phase == core.AdmissionChannelSave || phase == core.AdmissionCandidate) {
		generation := jsplugin.DefaultRegistry.Generation()
		loaded, found := generation.Get(setting.TaskPluginKey)
		if !found {
			return admission
		}
		var declared string
		if loaded.Meta.HasTaskPart() {
			for _, name := range []string{public, upstream, reasoning.BaseModelName(public), reasoning.BaseModelName(upstream)} {
				canonical, ok := generation.CanonicalModel(name)
				if ok && slices.Contains(loaded.Meta.Models, canonical) {
					declared = canonical
					break
				}
			}
		}
		if declared == "" && !loaded.Meta.RelaysModel(public) {
			return admission
		}
		executor, err := PluginExecutorIdentity(ch, loaded, generation.Number, "publication", "/v1/models")
		if err != nil {
			return admission
		}
		admission.Model.Declared, admission.Executor = declared, executor
		return admission
	}
	if ch.Type == constant.ChannelTypeTaskPlugin || setting.TaskPluginKey != "" || len(setting.TaskExtendPluginKeys) > 0 || len(setting.RequestPlugins) > 0 {
		return admission
	}
	// Legacy task resolution is not limited to the owner's declared models.
	// Until an exact legacy execution envelope exists, even dual-use ordinary
	// relay channels must not claim builtin identity while a current owner exists.
	generation := jsplugin.DefaultRegistry.Generation()
	if _, owned := generation.GetByChannelType(ch.Type); owned {
		return admission
	}
	if _, owned := jsplugin.ResolveTaskPluginForPlatform(generation, constant.TaskPlatform(strconv.Itoa(ch.Type))); owned {
		return admission
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

// PluginExecutorIdentity attests a bound type-61 artifact and its routing controls.
// Only the digest escapes: the envelope may contain private configuration.
func PluginExecutorIdentity(ch *Channel, loaded *jsplugin.LoadedPlugin, generation uint64, role, pathScope string) (core.ExecutorIdentity, error) {
	denied := errors.New("model admission denied")
	if ch == nil || ch.Type != constant.ChannelTypeTaskPlugin || loaded == nil || loaded.Engine == nil ||
		loaded.SourceSHA256() == "" || loaded.SourceSHA256() != loaded.Engine.SourceSHA256() ||
		(role != "task" && role != "provider" && (role != "publication" || pathScope != "/v1/models")) || pathScope == "" || !strings.HasPrefix(pathScope, "/") || strings.ContainsAny(pathScope, "?#\\\\\r\n") {
		return core.ExecutorIdentity{}, denied
	}
	var setting kitdto.ChannelSettings
	var other kitdto.ChannelOtherSettings
	if ch.Setting == nil || common.UnmarshalJsonStr(*ch.Setting, &setting) != nil || setting.TaskPluginKey != loaded.Meta.Key || len(setting.TaskExtendPluginKeys) != 0 || len(setting.RequestPlugins) != 0 ||
		(ch.OtherSettings != "" && common.UnmarshalJsonStr(ch.OtherSettings, &other) != nil) {
		return core.ExecutorIdentity{}, denied
	}
	if ch.OtherSettings != "" {
		var object map[string]common.RawMessage
		if common.UnmarshalJsonStr(ch.OtherSettings, &object) != nil || object == nil {
			return core.ExecutorIdentity{}, denied
		}
	}
	for _, instance := range request_plugin_setting.GetInstances() {
		if instance.Enabled {
			return core.ExecutorIdentity{}, denied
		}
	}
	config := other.PluginConfig
	if len(config) == 0 {
		config = common.RawMessage(`{}`)
	}
	var object map[string]common.RawMessage
	if common.Unmarshal(config, &object) != nil || object == nil {
		return core.ExecutorIdentity{}, denied
	}
	// Provider routes are derived from this exact source and effective config,
	// not the stored advanced-custom override. Keep the runtime envelope equal.
	if role == "provider" {
		other.AdvancedCustom = nil
	}
	other.PluginConfig = nil
	controls := make([]map[string]any, 3)
	for i, raw := range []string{ch.GetModelMapping(), stringPointerValue(ch.ParamOverride), stringPointerValue(ch.HeaderOverride)} {
		controls[i] = make(map[string]any)
		if raw != "" && common.UnmarshalJsonStr(raw, &controls[i]) != nil {
			return core.ExecutorIdentity{}, denied
		}
		if controls[i] == nil {
			controls[i] = make(map[string]any)
		}
	}
	envelope := struct {
		Version                                int
		Role, PathScope, BaseURL, ChannelOther string
		ChannelType                            int
		Binding                                kitdto.ChannelSettings
		Other                                  kitdto.ChannelOtherSettings
		EffectiveConfig                        common.RawMessage
		Controls                               []map[string]any
	}{1, role, pathScope, ch.GetBaseURL(), ch.Other, ch.Type, setting, other, loaded.ChannelConfigFor(config), controls}
	encoded, err := common.Marshal(envelope)
	if err != nil {
		return core.ExecutorIdentity{}, denied
	}
	digest := sha256.Sum256(encoded)
	return core.ExecutorIdentity{Kind: "plugin", Key: loaded.Meta.Key, SourceSHA256: loaded.Engine.SourceSHA256(), ConfigFingerprint: fmt.Sprintf("%x", digest), Generation: int64(generation)}, nil
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ChannelPluginAdmission requires the exact pinned artifact, never a metadata owner.
func ChannelPluginAdmission(ch *Channel, public string, phase core.AdmissionPhase, pinned jsplugin.PinnedPlugin, role, pathScope string) (core.Admission, error) {
	admission := channelModelProjection(ch, public, phase)
	if (role != "task" && role != "provider") || pinned.Generation == nil || pinned.Plugin == nil {
		return admission, errors.New("model admission denied")
	}
	published, ok := pinned.Generation.Get(pinned.Plugin.Meta.Key)
	if !ok || published != pinned.Plugin || admission.Model.Upstream == "" {
		return admission, errors.New("model admission denied")
	}
	executor, err := PluginExecutorIdentity(ch, pinned.Plugin, pinned.Generation.Number, role, pathScope)
	if err != nil {
		return admission, err
	}
	if role == "provider" {
		if _, ok := pinned.Plugin.Meta.RelayRoute(pathScope, public); !ok {
			return admission, errors.New("model admission denied")
		}
	} else {
		for _, name := range []string{public, admission.Model.Upstream, reasoning.BaseModelName(public), reasoning.BaseModelName(admission.Model.Upstream)} {
			canonical, found := pinned.Generation.CanonicalModel(name)
			if found && slices.Contains(pinned.Plugin.Meta.Models, canonical) {
				admission.Model.Declared = canonical
				break
			}
		}
		if admission.Model.Declared == "" {
			return admission, errors.New("model admission denied")
		}
	}
	admission.Executor = executor
	return admission, nil
}

// CheckPluginExecution checks runtime-owned settings, not a fresh row pretending
// to be the selected snapshot. Source/config approval does not attest opaque wire bodies.
func CheckPluginExecution(ctx context.Context, selected *Channel, pinned jsplugin.PinnedPlugin, public, upstream, role, pathScope string) error {
	if !core.ModelAdmissionEnabled() {
		return nil
	}
	if selected == nil || DB == nil || upstream == "" {
		return ErrChannelAdmissionDenied
	}
	fresh, err := GetChannelById(selected.Id, false)
	if err != nil || fresh == nil || fresh.Type != selected.Type || selected.Group == "" || !slices.Contains(strings.Split(fresh.Group, ","), selected.Group) {
		return ErrChannelAdmissionDenied
	}
	base, err := ChannelPluginAdmission(fresh, public, core.AdmissionDispatch, pinned, role, pathScope)
	if err != nil || core.Models.Check(ctx, base) != nil {
		return ErrChannelAdmissionDenied
	}
	actual, err := ChannelPluginAdmission(selected, public, core.AdmissionDispatch, pinned, role, pathScope)
	if err != nil || actual.Model.Upstream != upstream {
		return ErrChannelAdmissionDenied
	}
	// Only parameter overrides may differ from the current approved base.
	// This attests the final effective envelope, not its affinity-rule origin.
	effective := *fresh
	effective.ParamOverride = selected.ParamOverride
	expected, err := ChannelPluginAdmission(&effective, public, core.AdmissionDispatch, pinned, role, pathScope)
	if err != nil || expected.Model != actual.Model || expected.Executor != actual.Executor || core.Models.Check(ctx, actual) != nil {
		return ErrChannelAdmissionDenied
	}
	return nil
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
	return ChannelSatisfiesHostFilters(ch, modelName, filters)
}

// ChannelSatisfiesHostFilters applies protocol constraints after an exact pinned
// executor has already passed admission. It grants no model-policy authority.
func ChannelSatisfiesHostFilters(ch *Channel, modelName string, filters []dto.ChannelFilter) (bool, dto.ChannelFilterKind) {
	if ch == nil {
		return false, ""
	}
	view, coherent := coherentCandidateView(filters)
	if ch.Type == constant.ChannelTypeTaskPlugin && !coherent {
		return false, ""
	}
	for _, kind := range filterEvalOrder {
		for _, filter := range filters {
			if filter.PluginView == nil {
				filter.PluginView = view
			}
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
	view, coherent := coherentCandidateView(filters)
	if view != nil && coherent {
		filters = slices.Clone(filters)
		for i := range filters {
			if filters[i].PluginView == nil {
				filters[i].PluginView = view
			}
		}
	} else if !coherent {
		kept = make([]int, 0, len(ids))
		for _, id := range ids {
			if ch := channelsIDM[id]; ch == nil || ch.Type != constant.ChannelTypeTaskPlugin {
				kept = append(kept, id)
			}
		}
	}
	if core.ModelAdmissionEnabled() {
		admitted := make([]int, 0, len(kept))
		for _, id := range kept {
			channel := channelsIDM[id]
			if channel != nil && core.Models.Check(context.Background(), ChannelModelAdmission(channel, modelName, core.AdmissionCandidate, filters...)) == nil {
				admitted = append(admitted, id)
			}
		}
		kept = admitted
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
	if ch.Type == constant.ChannelTypeTaskPlugin && filter.PluginView != nil {
		return filter.PluginView.Matches(candidateSnapshot(ch), modelName, filter)
	}
	return channelMatchesFilterInGeneration(ch, modelName, filter, nil)
}

func channelMatchesFilterInGeneration(ch *Channel, modelName string, filter dto.ChannelFilter, generation *jsplugin.RoutingGeneration) bool {
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
			return ch.Type != constant.ChannelTypeTaskPlugin || providerChannelServesInGeneration(ch, filter.RequestPath, modelName, generation)
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
		relayChannel := ch.Type != constant.ChannelTypeTaskPlugin || providerChannelServesInGeneration(ch, filter.RequestPath, modelName, generation)
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
			// A supplied view checks its captured source, not a later registry.
			if generation != nil {
				plugin, ok := generation.Get(ch.GetSetting().TaskPluginKey)
				return ok && plugin.Meta.ServesResponsesWebSocket(modelName)
			}
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
func providerChannelServesInGeneration(ch *Channel, requestPath, modelName string, generation *jsplugin.RoutingGeneration) bool {
	if ch.Type != constant.ChannelTypeTaskPlugin || requestPath == "" {
		return false
	}
	var plugin *jsplugin.LoadedPlugin
	var ok bool
	if generation != nil {
		plugin, ok = generation.Get(ch.GetSetting().TaskPluginKey)
	} else {
		plugin, ok = jsplugin.DefaultRegistry.Get(ch.GetSetting().TaskPluginKey)
	}
	if !ok || !plugin.Meta.HasProviderPart() {
		return false
	}
	_, ok = plugin.Meta.RelayRoute(requestPath, modelName)
	return ok
}

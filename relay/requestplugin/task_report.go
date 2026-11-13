package requestplugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/request_plugin_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Limits of a task report, part of the published plugin contract
// (docs/plugin-api/v1.md).
const (
	// maxTaskRequestBytes is the largest request a task keeps for its report.
	maxTaskRequestBytes = 64 << 10
	// maxTaskDataBytes is the largest task result the report reads.
	maxTaskDataBytes       = 8 << 20
	maxTaskURLs            = 32
	maxTaskTexts           = 256
	maxFrozenReportBytes   = 4 << 20
	maxFrozenReportTargets = 16
	// maxTaskTextBytes bounds the result texts, as the response capture is
	// bounded.
	maxTaskTextBytes = maxReportResponseBytes
)

type reportTask struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// FinishedAt is in Unix milliseconds.
	FinishedAt int64            `json:"finishedAt"`
	Result     reportTaskResult `json:"result"`
}

type reportTaskResult struct {
	URLs      []string         `json:"urls"`
	Texts     []reportTaskText `json:"texts"`
	Truncated bool             `json:"truncated"`
}

type reportTaskText struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

type taskReportRuntime struct {
	Version  int
	Instance request_plugin_setting.Instance
}

// freezeTaskReports is best-effort after acceptance, never a billing barrier.
// References are attached only when every target was durably saved.
func freezeTaskReports(c *gin.Context, info *relaycommon.RelayInfo, plan *Plan, instances []*request_plugin_setting.Instance, stored *model.TaskRequestPlugins) {
	store := core.TaskContinuations
	if store == nil {
		return
	}
	stored.Frozen = true
	if c == nil || c.Request == nil || info.TaskRelayInfo == nil || info.PublicTaskID == "" || info.UserId <= 0 || info.ChannelMeta == nil || info.ChannelId <= 0 || len(instances) > 256 {
		common.SysError("task report continuation unavailable")
		return
	}
	refs := make([]model.TaskReportContinuation, 0, len(instances))
	seen := make(map[string]bool, len(instances))
	for _, instance := range instances {
		plugin := plan.plugins[instance.Plugin]
		if instance.ID == "" || seen[instance.ID] || plugin == nil || plugin.Engine == nil || plugin.Engine.SourceSHA256() != plugin.SourceSHA256() || plugin.Meta.Key != instance.Plugin {
			common.SysError("task report continuation unavailable")
			return
		}
		seen[instance.ID] = true
		source := plugin.SourceBytes()
		runtime, err := common.Marshal(taskReportRuntime{Version: 1, Instance: *instance})
		if err != nil || len(runtime) > 256<<10 || !utf8.Valid(runtime) || len(source) == 0 || len(source) > 2<<20 || !utf8.Valid(source) || fmt.Sprintf("%x", sha256.Sum256(source)) != plugin.SourceSHA256() {
			common.SysError("task report continuation unavailable")
			return
		}
		ref, err := store.Save(c.Request.Context(), core.ContinuationBinding{TaskID: info.PublicTaskID, UserID: info.UserId, ChannelID: info.ChannelId}, core.ContinuationArtifact{PluginKey: plugin.Meta.Key, SourceSHA256: plugin.SourceSHA256(), Source: source, RuntimeJSON: runtime})
		if err != nil || ref.ID == "" || ref.Digest == "" {
			common.SysError("task report continuation unavailable")
			return
		}
		refs = append(refs, model.TaskReportContinuation{InstanceID: instance.ID, Ref: ref})
	}
	stored.Continuations = refs
}

// TaskReport is what a task submission keeps for the report at the task's
// final state: the report instances in scope, the request as the record
// shows it and the submission's record. It is nil when no instance may
// report the task. Call it once the submission was accepted, before the task
// is stored.
func TaskReport(c *gin.Context, info *relaycommon.RelayInfo) *model.TaskRequestPlugins {
	plan := planOf(c)
	if plan == nil || !plan.reports || plan.submission == nil || info == nil {
		return nil
	}
	call := attemptCall(c, info)
	var ids []string
	var targets []*request_plugin_setting.Instance
	for _, instance := range plan.scope(jsplugin.ReportHook, call.Group, call.Model, call.Bound) {
		if plan.plugins[instance.Plugin] != nil {
			ids = append(ids, instance.ID)
			targets = append(targets, instance)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	started := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if started.IsZero() {
		started = time.Now()
	}
	stored := &model.TaskRequestPlugins{
		Instances: ids,
		Format:    call.Format,
		Attempt:   call.Attempt,
		Username:  call.User.Username,
		StartedAt: started.UnixMilli(),
		Record:    relaycommon.CapRequestPluginRecord(slices.Clone(info.RequestPluginRecord)),
	}
	if call.Channel != nil {
		stored.ChannelType = call.Channel.Type
	}
	if call.Token != nil {
		stored.TokenName = call.Token.Name
	}
	freezeTaskReports(c, info, plan, targets, stored)
	if !jsonRequest(c) {
		return stored
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return stored
	}
	body, err := storage.Bytes()
	if err != nil || !gjson.ValidBytes(body) {
		return stored
	}
	view, media := scanMedia(body)
	if len(view) > maxTaskRequestBytes {
		return stored
	}
	stored.Request = slices.Clone(view)
	for index, item := range media {
		stored.Media = append(stored.Media, jsplugin.TransformMedia{Index: index, Path: item.path, MimeType: item.mime, Bytes: item.size()})
	}
	return stored
}

// TaskCreated marks a submission whose task is stored: the task reports at
// its final state, so the submission does not report.
func TaskCreated(c *gin.Context) {
	if plan := planOf(c); plan != nil && plan.submission != nil {
		plan.submission.stored = true
	}
}

// frozenTaskReportTargets restores every target before any report can be queued.
// Compilation is unpublished; normal worker source/executor/target gates remain.
func frozenTaskReportTargets(task *model.Task, stored *model.TaskRequestPlugins) ([]reportTarget, error) {
	if len(stored.Continuations) == 0 {
		return nil, nil
	}
	store := core.TaskContinuations
	if store == nil || task.TaskID == "" || task.UserId <= 0 || task.ChannelId <= 0 || len(stored.Continuations) > maxFrozenReportTargets || len(stored.Continuations) != len(stored.Instances) {
		return nil, core.ErrContinuationUnavailable
	}
	currentInstances := request_plugin_setting.GetInstances()
	for _, ref := range stored.Continuations {
		index := slices.IndexFunc(currentInstances, func(i request_plugin_setting.Instance) bool { return i.ID == ref.InstanceID })
		if index < 0 || !currentInstances[index].Enabled {
			return nil, core.ErrContinuationUnavailable
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	targets := make([]reportTarget, 0, len(stored.Continuations))
	seen := make(map[string]bool, len(stored.Continuations))
	retained := 0
	for index, continuation := range stored.Continuations {
		if continuation.InstanceID == "" || continuation.InstanceID != stored.Instances[index] || continuation.Ref.ID == "" || continuation.Ref.Digest == "" || seen[continuation.InstanceID] {
			return nil, core.ErrContinuationUnavailable
		}
		seen[continuation.InstanceID] = true
		artifact, err := store.Load(ctx, core.ContinuationBinding{TaskID: task.TaskID, UserID: task.UserId, ChannelID: task.ChannelId}, continuation.Ref, core.ContinuationReport)
		if err != nil || ctx.Err() != nil || len(artifact.Source) == 0 || len(artifact.Source) > 2<<20 || !utf8.Valid(artifact.Source) || fmt.Sprintf("%x", sha256.Sum256(artifact.Source)) != artifact.SourceSHA256 || len(artifact.RuntimeJSON) == 0 || len(artifact.RuntimeJSON) > 256<<10 || !utf8.Valid(artifact.RuntimeJSON) {
			return nil, core.ErrContinuationUnavailable
		}
		retained += len(artifact.Source) + len(artifact.RuntimeJSON)
		if retained > maxFrozenReportBytes {
			return nil, core.ErrContinuationUnavailable
		}
		var runtime taskReportRuntime
		if common.Unmarshal(artifact.RuntimeJSON, &runtime) != nil || runtime.Version != 1 || runtime.Instance.ID != continuation.InstanceID || runtime.Instance.Plugin != artifact.PluginKey || !runtime.Instance.Enabled {
			return nil, core.ErrContinuationUnavailable
		}
		currentIndex := slices.IndexFunc(currentInstances, func(i request_plugin_setting.Instance) bool { return i.ID == continuation.InstanceID })
		if currentIndex < 0 || currentInstances[currentIndex].Plugin != runtime.Instance.Plugin {
			return nil, core.ErrContinuationUnavailable
		}
		encoded, err := common.Marshal(runtime)
		if err != nil || !bytes.Equal(encoded, artifact.RuntimeJSON) {
			return nil, core.ErrContinuationUnavailable
		}
		plugin, err := jsplugin.CompilePlugin(string(artifact.Source), jsplugin.Options{})
		if err != nil || plugin.Meta.Key != artifact.PluginKey || plugin.SourceSHA256() != artifact.SourceSHA256 || plugin.Engine.SourceSHA256() != artifact.SourceSHA256 || !plugin.Meta.HasTransformPart() || !slices.Contains(plugin.TransformHooks(), jsplugin.ReportHook) {
			return nil, core.ErrContinuationUnavailable
		}
		targets = append(targets, reportTarget{plugin: plugin, instance: &runtime.Instance})
	}
	return targets, nil
}

// ReportTask restores the accepted report targets of a frozen task; legacy tasks
// keep the current enabled-instance lookup. It reads only the task, so it runs on
// whichever node finished it, once per task after settlement and refund.
func ReportTask(task *model.Task) {
	if task == nil {
		return
	}
	stored := task.PrivateData.RequestPlugins
	if stored == nil {
		return
	}
	data := task.Data
	if len(data) > maxTaskDataBytes {
		data = nil
	}
	size := int64(len(stored.Request) + len(data))
	frozen := stored.Frozen || len(stored.Continuations) > 0
	reserved := false
	if frozen {
		size += maxFrozenReportBytes
		if !reports.admit(size) {
			return
		}
		reserved = true
		defer func() {
			if reserved {
				reports.bytes.Add(-size)
			}
		}()
	}
	var targets []reportTarget
	if frozen {
		var err error
		targets, err = frozenTaskReportTargets(task, stored)
		if err != nil {
			common.SysError("task report continuation unavailable")
			return
		}
	} else {
		instances := request_plugin_setting.GetInstances()
		for _, id := range stored.Instances {
			index := slices.IndexFunc(instances, func(instance request_plugin_setting.Instance) bool { return instance.ID == id })
			if index < 0 || !instances[index].Enabled {
				continue
			}
			plugin, _ := jsplugin.DefaultRegistry.GetIgnoringMasterSwitch(instances[index].Plugin)
			if plugin == nil || !plugin.Meta.HasTransformPart() || !slices.Contains(plugin.TransformHooks(), jsplugin.ReportHook) {
				continue
			}
			targets = append(targets, reportTarget{plugin: plugin, instance: &instances[index]})
		}
	}
	if len(targets) == 0 {
		return
	}
	if !frozen && !reports.admit(size) {
		return
	}
	call := jsplugin.TransformContext{
		Format:        stored.Format,
		Model:         task.Properties.OriginModelName,
		UpstreamModel: task.Properties.UpstreamModelName,
		Group:         task.Group,
		Attempt:       stored.Attempt,
		User:          jsplugin.TransformUser{ID: task.UserId, Username: stored.Username},
	}
	var requestID string
	if execution := task.PrivateData.Execution; execution != nil {
		requestID, call.Path = execution.RequestID, execution.RequestPath
	}
	if task.ChannelId > 0 {
		call.Channel = &jsplugin.TransformChannel{ID: task.ChannelId, Type: stored.ChannelType}
	}
	token := reportToken{ID: task.PrivateData.TokenId, Name: stored.TokenName}
	if token.ID > 0 {
		call.Token = &jsplugin.TransformToken{ID: token.ID, Name: token.Name}
	}
	job := &reportJob{
		targets:  targets,
		tctx:     call,
		size:     size,
		taskData: data,
		record: reportRecord{
			RequestID:      requestID,
			User:           reportUser{ID: task.UserId, Username: stored.Username},
			Token:          token,
			StartedAt:      stored.StartedAt,
			DurationMs:     time.Now().UnixMilli() - stored.StartedAt,
			Request:        reportRequest{Body: stored.Request, Media: stored.Media},
			Task:           &reportTask{ID: task.TaskID, Status: string(task.Status), FinishedAt: task.FinishTime * 1000},
			Cost:           reportCost{Quota: task.Quota, USD: float64(task.Quota) / common.QuotaPerUnit},
			RequestPlugins: stored.Record,
		},
	}
	if task.Status == model.TaskStatusFailure {
		job.record.Task.Reason = task.FailReason
	}
	if url := task.PrivateData.ResultURL; isHTTPURL(url) {
		job.taskURL = url
	}
	reserved = false
	reports.push(job)
}

// taskResult reads the result of a task for its report: the http and https
// URLs, then the other strings as texts with their dotted paths. Data URLs
// and base64 data are left out. Past a bound the rest is left out and
// Truncated is set.
func taskResult(data []byte, resultURL string) reportTaskResult {
	result := reportTaskResult{URLs: []string{}, Texts: []reportTaskText{}}
	if resultURL != "" {
		result.URLs = append(result.URLs, resultURL)
	}
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return result
	}
	textBytes := 0
	var walk func(value gjson.Result, path string) bool
	walk = func(value gjson.Result, path string) bool {
		switch {
		case value.IsObject() || value.IsArray():
			index := 0
			value.ForEach(func(key, member gjson.Result) bool {
				name := key.String()
				if value.IsArray() {
					name = strconv.Itoa(index)
				}
				index++
				if path != "" {
					name = path + "." + name
				}
				return walk(member, name)
			})
			return !result.Truncated
		case value.Type != gjson.String || value.Str == "" || strings.HasPrefix(value.Str, "data:") || base64Data(value.Str):
			return true
		case isHTTPURL(value.Str):
			if slices.Contains(result.URLs, value.Str) {
				return true
			}
			if len(result.URLs) == maxTaskURLs {
				result.Truncated = true
				return false
			}
			result.URLs = append(result.URLs, value.Str)
		default:
			if len(result.Texts) == maxTaskTexts {
				result.Truncated = true
				return false
			}
			text := value.Str
			if room := maxTaskTextBytes - textBytes; len(text) > room {
				// The last text is cut at a character.
				text = strings.ToValidUTF8(text[:room], "")
				result.Truncated = true
			}
			textBytes += len(text)
			result.Texts = append(result.Texts, reportTaskText{Path: path, Text: text})
			return !result.Truncated
		}
		return true
	}
	walk(gjson.ParseBytes(data), "")
	return result
}

func isHTTPURL(text string) bool {
	return strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://")
}

// base64Data reports a long string of base64 characters only: encoded
// media, not text.
func base64Data(text string) bool {
	if len(text) < 1024 {
		return false
	}
	return !strings.ContainsFunc(text, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=' || r == '-' || r == '_')
	})
}

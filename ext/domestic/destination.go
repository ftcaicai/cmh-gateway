package domestic

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/ext/core"
)

// DestinationPolicy approves exact channel/model/method/transport/address/Host
// relationships. It never discovers or automatically trusts a channel Base URL,
// plugin allowedHosts, model aliases or already-approved executor sources.
type DestinationPolicy struct {
	approved map[core.DestinationAdmission]struct{}
}

var _ core.DestinationGate = (*DestinationPolicy)(nil)

func NewDestinationPolicy(approvals []core.DestinationAdmission) (*DestinationPolicy, error) {
	if len(approvals) > 4096 {
		return nil, errGateConfiguration
	}
	policy := &DestinationPolicy{approved: make(map[core.DestinationAdmission]struct{}, len(approvals))}
	for _, approval := range approvals {
		if !validDestination(approval) {
			return nil, errGateConfiguration
		}
		if _, duplicate := policy.approved[approval]; duplicate {
			return nil, errGateConfiguration
		}
		policy.approved[approval] = struct{}{}
	}
	return policy, nil
}

func validDestination(admission core.DestinationAdmission) bool {
	if admission.Kind == "plugin_fetch" {
		if admission.Channel != (core.ChannelIdentity{}) || admission.Model != (core.ModelIdentity{}) || admission.Executor.Kind != "plugin" || admission.Hook == "" || len(admission.Hook) > 128 || !utf8.ValidString(admission.Hook) || strings.ContainsFunc(admission.Hook, unicode.IsControl) {
			return false
		}
		if _, valid := approvedExecutorIdentity(admission.Executor); !valid {
			return false
		}
		if admission.Method != "GET" && admission.Method != "HEAD" && admission.Method != "POST" {
			return false
		}
	} else {
		if admission.Executor != (core.ExecutorIdentity{}) || admission.Hook != "" || !admission.Channel.Known || admission.Channel.ID <= 0 || admission.Channel.Type <= 0 {
			return false
		}
		switch admission.Kind {
		case "relay_http":
			switch admission.Method {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
			default:
				return false
			}
		case "relay_websocket":
			if admission.Method != "GET" {
				return false
			}
		default:
			return false
		}
		for _, name := range []string{admission.Model.Public, admission.Model.Upstream} {
			if name == "" || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
				return false
			}
		}
	}
	if len(admission.Model.Declared) > 255 || !utf8.ValidString(admission.Model.Declared) || strings.ContainsFunc(admission.Model.Declared, unicode.IsControl) {
		return false
	}
	for _, digest := range []string{admission.AddressSHA256, admission.HostSHA256} {
		if len(digest) != 64 {
			return false
		}
		for _, c := range digest {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}

func (policy *DestinationPolicy) CheckDestination(ctx context.Context, admission core.DestinationAdmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if policy == nil || !validDestination(admission) {
		return ErrAdmissionDenied
	}
	if _, approved := policy.approved[admission]; !approved {
		return ErrAdmissionDenied
	}
	return nil
}

package domestic

import (
	"errors"

	"github.com/QuantumNous/new-api/ext/core"
)

// InstallPolicyOnly is startup-only, before any consumer. All validation finishes
// before publication; it is not atomic against concurrent readers or a reload API.
// All-absent configuration preserves defaults. Archive/grants are never enabled.
func InstallPolicyOnly(environment map[string]string) error {
	policies, err := LoadOperatorPolicies(environment)
	if err != nil {
		return errors.New("extension policy startup rejected")
	}
	if policies == nil {
		return nil
	}
	if len(policies.Continuations.grants) != 0 {
		return errors.New("extension policy startup rejected")
	}
	core.Models, core.PluginHooks, core.Executors = policies.Models, policies.Models, policies.Models
	core.Destinations = policies.Destinations
	core.TaskContinuations, core.ContinuationGrants = nil, nil
	return nil
}

// Package ext is the single entry point of the extension zone.
//
// Wiring: main.go blank-imports this package (`_ ".../ext"`), the only place
// the official zone names it, and that import is registered in
// ext/docs/HOOKS.md under H2. init() then installs the real hook
// implementations into ext/core. Official packages other than main never
// import ext or ext/<module>; they only call ext/core (ext/AGENTS.md §2.2).
package ext

import (
	"github.com/QuantumNous/new-api/ext/brand"
	"github.com/QuantumNous/new-api/ext/core"
)

func init() { Register() }

// Register installs every extension hook implementation into ext/core.
// Idempotent; call order is the delivery order of the phases.
func Register() {
	core.Tenant = brand.Resolver{} // H2
	core.Routes = brand.Routes{}   // H5
}

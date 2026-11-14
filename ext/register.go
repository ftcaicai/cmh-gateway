// Package ext is the single entry point of the extension zone.
//
// Wiring: main.go blank-imports this package (`_ ".../ext"`), the only place
// the official zone names it, and that import is registered in
// ext/docs/HOOKS.md under H2. init() then installs the real hook
// implementations into ext/core. Official packages other than main never
// import ext or ext/<module>; they only call ext/core (ext/AGENTS.md §2.2).
package ext

import (
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/ext/brand"
	"github.com/QuantumNous/new-api/ext/console"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/domestic"
)

func init() { Register() }

// Register installs every extension hook implementation into ext/core.
// Idempotent; call order is the delivery order of the phases.
func Register() {
	core.InitializePolicies = domestic.InstallPolicyOnly // H1, explicit host startup only
	core.Tenant = brand.Resolver{}                       // H2
	core.Routes = routes{}                               // H5
}

// routes is the single H5 registrar; each module mounts itself here in order.
type routes struct{}

func (routes) Register(engine *gin.Engine) {
	brand.Routes{}.Register(engine) // brand hosts first: they must never reach official fallbacks
	console.Mount(engine)           // /cmh/* operations console
}

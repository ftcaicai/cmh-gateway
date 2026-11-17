// Package ext is the single entry point of the extension zone.
//
// Wiring: main.go blank-imports this package (`_ ".../ext"`), the only place
// the official zone names it, and that import is registered in
// ext/docs/HOOKS.md under H2. init() then installs the real hook
// implementations into ext/core. Official packages other than main never
// import ext or ext/<module>; they only call ext/core (ext/AGENTS.md §2.2).
package ext

import (
	"os"

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
	brand.RegisterDomains()                              // T2.5: verified-domain sync topic and DNS check task
	// H8 is off unless the operator opts in at startup. Before the first start
	// with it on, run `oem-migrate -backfill-main-auth` with the gateway
	// stopped; see ext/docs/OEM_V1_OPERATOR_GUIDE.md.
	if os.Getenv("CMH_BRAND_AUTH") == "1" {
		brand.EnableAuth()
	}
}

// routes is the single H5 registrar; each module mounts itself here in order.
type routes struct{}

func (routes) Register(engine *gin.Engine) {
	brand.Routes{}.Register(engine) // brand hosts first: they must never reach official fallbacks
	brand.MountOEM(engine)          // official-session OEM management API
	console.Mount(engine)           // /cmh/* operations console
}

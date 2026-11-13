// Package ext is the single entry point of the extension zone.
//
// Official code never imports this package (ext/AGENTS.md §2.2). The only
// official-side reference is the H5 call in router/main.go, added later via a
// registered hook; until then this package is compiled but not wired.
package ext

// Register installs every extension hook implementation into ext/core and
// mounts extension routes. It is a no-op in P0; each later phase adds its
// registrations here, in the order the phases are delivered.
func Register() {}

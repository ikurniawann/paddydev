package app

import (
	"nuhabit/backend/internal/modules/identity"
	"nuhabit/backend/internal/platform/module"
)

// identity: login, logout, change-password, impersonation, me, scope and the
// active stall. The stall switcher reads warehouses through identityStalls.
func init() {
	Register(identity.Name, func(d module.Deps) module.Module {
		return identity.New(d, identityStalls{db: d.DB}, identityEmployeePhotos{db: d.DB})
	})
}

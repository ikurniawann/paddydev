package identity

import (
	"context"

	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/storage"
)

// Name is the MODULES key.
const Name = "identity"

type mod struct{ handlers }

func (mod) Name() string { return Name }

// New builds the module. stalls is the warehouse port and photos the
// employee photo mirror (EPIC-087, may be nil) internal/app adapts.
func New(deps module.Deps, stalls Stalls, photos EmployeePhotos) module.Module {
	warn := func(ctx context.Context, msg string, err error) { deps.Log.WarnContext(ctx, msg, "error", err) }
	svc := NewService(NewPostgres(deps.DB), stalls, deps.Auth, deps.Now, warn)
	svc.files, svc.photos = storage.FromEnv(), photos
	return mod{handlers{svc: svc, log: deps.Log}}
}

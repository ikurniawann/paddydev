package hris

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/platform/httpx"
)

// retryableLockError turns a deadlock (40P01) or lock timeout (55P03) into a
// 409 the user can retry (EPIC-105 R2); every other error is unchanged.
func retryableLockError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "40P01" || pg.Code == "55P03") {
		return httpx.Conflict("Data sedang diubah proses lain — coba lagi sebentar")
	}
	return err
}

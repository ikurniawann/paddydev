package hris

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"nuhabit/backend/internal/platform/httpx"
)

// EPIC-105 R2 / Review Focus 5: only deadlock and lock timeout become a
// retryable 409; every other error passes through unchanged.
func TestRetryableLockError(t *testing.T) {
	for _, code := range []string{"40P01", "55P03"} {
		for name, err := range map[string]error{
			"retryable": retryableLockError(&pgconn.PgError{Code: code}),
			"placement": placementError(&pgconn.PgError{Code: code}),
		} {
			var he *httpx.Error
			if !errors.As(err, &he) || he.Status != 409 || he.Message != "Data sedang diubah proses lain — coba lagi sebentar" {
				t.Errorf("%s %s = %v", name, code, err)
			}
		}
	}
	unique := &pgconn.PgError{Code: "23505"}
	if got := retryableLockError(unique); got != unique {
		t.Errorf("23505 changed: %v", got)
	}
	plain := errors.New("x")
	if got := retryableLockError(plain); got != plain {
		t.Errorf("plain changed: %v", got)
	}
	if retryableLockError(nil) != nil {
		t.Error("nil changed")
	}
}

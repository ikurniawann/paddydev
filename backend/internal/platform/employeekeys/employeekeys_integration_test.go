package employeekeys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-105 R1 / Review Focus 1: two new employees in different months
// cannot take the same main sequence number. The second check waits for
// the first transaction and is then refused.
func TestCheckNewNIPConcurrentSeries(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	l, err := lastFor(ctx, db, "2090-01-05")
	if err != nil {
		t.Fatal(err)
	}
	seq := l.Main + 1
	if seq < 500 {
		seq = 500
	}
	jan, feb := fmt.Sprintf("209001%03d", seq), fmt.Sprintf("209002%03d", seq)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM hris.employees WHERE nip IN ($1, $2)`, jan, feb)
	})

	tx1, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx)
	if err := CheckNewNIP(ctx, tx1, jan, "2090-01-05", "permanent"); err != nil {
		t.Fatalf("first check: %v", err)
	}
	if _, err := tx1.Exec(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('QA NIP Jan', $1, 'qa-nip-jan@invalid.local', '0800000001', '2090-01-05')`, jan); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	pid := make(chan int, 1)
	go func() {
		tx2, err := db.Begin(ctx)
		if err != nil {
			pid <- 0
			done <- err
			return
		}
		defer tx2.Rollback(ctx)
		var p int
		_ = tx2.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&p)
		pid <- p
		done <- CheckNewNIP(ctx, tx2, feb, "2090-02-05", "permanent")
	}()
	tx2pid := <-pid
	waiting := false
	for i := 0; i < 60 && !waiting && tx2pid != 0; i++ {
		var n int
		_ = db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE pid = $1 AND wait_event_type = 'Lock'`, tx2pid).Scan(&n)
		waiting = n > 0
		if !waiting {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !waiting {
		<-done
		t.Fatal("second NIP check did not wait for the first transaction")
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	err = <-done
	var he *httpx.Error
	want := fmt.Sprintf("NIP %s: nomor urut terakhir karyawan %03d", feb, seq)
	if !errors.As(err, &he) || he.Status != 400 || !strings.HasPrefix(he.Message, want) {
		t.Fatalf("second check = %v, want 400 %q…", err, want)
	}
}

#!/usr/bin/env bash
# Tes runner cleanup DM-01 (EPIC-083): tools/ops/dm01-cleanup.sh.
#
# Hanya memakai DB uji lokal dan volume docker sementara:
#   - database sementara dm01_runner_test, dibuat dari template DB uji (createdb -T) di container
#     DB uji, lalu dihapus di akhir (DB uji bersama tidak disentuh);
#   - volume docker sementara dm01-runner-test-<pid>, dihapus di akhir;
#   - folder backup sementara (mktemp), dihapus di akhir.
# TIDAK PERNAH memakai paddydev-db atau paddydev-storage.
#
# Pemakaian: bash tools/ops/dm01-cleanup.test.sh
#   DM01_TEST_CONTAINER (bawaan paddydev-sdd-testdb), DM01_TEST_TEMPLATE (bawaan paddy_test_e071).
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RUNNER="$ROOT/tools/ops/dm01-cleanup.sh"
C=${DM01_TEST_CONTAINER:-paddydev-sdd-testdb}
TEMPLATE=${DM01_TEST_TEMPLATE:-paddy_test_e071}
DB=dm01_runner_test
RESTORE_DB=dm01_runner_restore
VOL=dm01-runner-test-$$
WORK=$(mktemp -d)
BK="$WORK/backups"
IMAGE=$(docker inspect -f '{{.Config.Image}}' "$C")

case "$C" in paddydev-db|*prod*) echo "TOLAK: tes tidak boleh memakai $C" >&2; exit 1 ;; esac

A=aaaaaaaa-0000-4000-8000-00000000000a # karyawan pemilik berkas
B=bbbbbbbb-0000-4000-8000-00000000000b # karyawan lain
S=cccccccc-0000-4000-8000-00000000000c # foldernya symlink ke luar private/

# search_path sama dengan sesi aplikasi: fungsi trigger memakai nama tabel tanpa schema.
SP='public,iam,configuration,hris,performance,recruitment,item,purchasing,inventory,manufacturing,pos,crm,accounting,auth'
pg() { docker exec -i -e PGOPTIONS="-c search_path=$SP" "$C" psql -X -q -U postgres -d "$DB" -v ON_ERROR_STOP=1 -At "$@"; }
vol() { docker run --rm -i --network none --entrypoint sh -v "$VOL:/s" "$IMAGE" -c "$1"; }
run() {
  env DM01_DB_CONTAINER="$C" DM01_DB_NAME="$DB" DM01_DB_USER=postgres DM01_VOLUME="$VOL" \
    DM01_BACKUP_DIR="$BK" bash "$RUNNER" "$@"
}
setuju() { DM01_APPROVED=yes run "$@"; }

cleanup() {
  docker exec "$C" dropdb -U postgres --if-exists "$DB" >/dev/null 2>&1 || true
  docker exec "$C" dropdb -U postgres --if-exists "$RESTORE_DB" >/dev/null 2>&1 || true
  docker volume rm -f "$VOL" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

GAGAL=0
ok() { echo "  ok   $1"; }
salah() { echo "  GAGAL $1" >&2; GAGAL=1; }
cek() { if eval "$2"; then ok "$1"; else salah "$1"; fi; }
# harus_gagal <label> <pola pesan> <perintah...>: perintah keluar non-nol dan pesannya memuat pola.
harus_gagal() {
  local label=$1 pola=$2 out rc=0
  shift 2
  out=$("$@" 2>&1) || rc=$?
  if [ "$rc" -ne 0 ] && grep -qiE "$pola" <<<"$out"; then ok "$label"; else
    salah "$label (rc=$rc)"; echo "$out" | sed 's/^/      | /' >&2
  fi
}
ada() { vol "test -f /s/private/$1"; }

echo "== Persiapan: DB sementara $DB dari $TEMPLATE, volume $VOL"
docker exec "$C" dropdb -U postgres --if-exists "$DB" >/dev/null
docker exec "$C" dropdb -U postgres --if-exists "$RESTORE_DB" >/dev/null
for i in 1 2 3 4 5 6 7 8 9 10; do
  docker exec "$C" createdb -U postgres -T "$TEMPLATE" "$DB" 2>/dev/null && break
  [ "$i" = 10 ] && { echo "createdb -T $TEMPLATE gagal (template sedang dipakai sesi lain?)" >&2; exit 1; }
  sleep 1
done
docker volume create "$VOL" >/dev/null

pg <<SQL
INSERT INTO hris.employees (id, full_name, nip, email, phone, join_date) VALUES
  ('$A', 'Karyawan Uji A', 'DM01-A', 'dm01-a@uji.local', '0800000001', '2025-01-01'),
  ('$B', 'Karyawan Uji B', 'DM01-B', 'dm01-b@uji.local', '0800000002', '2025-01-01'),
  ('$S', 'Karyawan Uji S', 'DM01-S', 'dm01-s@uji.local', '0800000003', '2025-01-01');
-- dihapus, berkas milik baris: FILE
INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, attachment_url)
  VALUES ('$A', 'annual', '2026-10-01', '2026-10-03', 3, 'uji runner', 'approved', 'leave-attachments/$A/hapus-1.pdf');
INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, reason, attachments)
  VALUES ('$A', '2026-10-05', '18:00', '20:00', 2, 'uji runner',
          ARRAY['overtime-attachments/$A/hapus-2.pdf', 'overtime-attachments/$A/hilang.pdf']);
-- dihapus, berkas di folder karyawan lain: FILE_LAIN (tidak dihapus otomatis)
INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, attachment_url)
  VALUES ('$A', 'annual', '2026-10-10', '2026-10-10', 1, 'uji runner', 'rejected', 'leave-attachments/$B/lain.pdf');
-- dihapus, FILE yang foldernya symlink ke luar private/: dilewati, tidak dihapus
INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, attachment_url)
  VALUES ('$S', 'annual', '2026-10-11', '2026-10-11', 1, 'uji runner', 'approved', 'leave-attachments/$S/luar.pdf');
-- tetap (> 20 Okt): berkasnya tetap
INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, attachment_url)
  VALUES ('$A', 'annual', '2026-10-25', '2026-10-25', 1, 'uji runner', 'approved', 'leave-attachments/$A/tetap.pdf');
SQL

vol "set -e; mkdir -p /s/private/leave-attachments/$A /s/private/leave-attachments/$B /s/private/overtime-attachments/$A
  echo satu > /s/private/leave-attachments/$A/hapus-1.pdf
  echo dua > /s/private/overtime-attachments/$A/hapus-2.pdf
  echo tetap > /s/private/leave-attachments/$A/tetap.pdf
  echo lain > /s/private/leave-attachments/$B/lain.pdf
  echo tak-dirujuk > /s/private/leave-attachments/$A/tak-dirujuk.pdf
  mkdir -p /s/luar && echo luar > /s/luar/luar.pdf && ln -s /s/luar /s/private/leave-attachments/$S"

unset DM01_APPROVED

echo "== Penolakan sebelum pratinjau"
harus_gagal "execute tanpa DM01_APPROVED=yes ditolak" "DM01_APPROVED" run execute
harus_gagal "execute tanpa pratinjau hari ini ditolak" "pratinjau" setuju execute
harus_gagal "files tanpa DM01_APPROVED=yes ditolak" "DM01_APPROVED" run files
harus_gagal "mode tak dikenal ditolak" "pemakaian|mode" run hapus-semua

echo "== Pratinjau"
out=$(run preview 2>&1) || { echo "$out" >&2; salah "preview berhasil"; exit 1; }
ok "preview berhasil"
echo "$out" | sed 's/^/      | /'
PDIR=$(ls -1d "$BK"/dm01-*-pratinjau* | tail -1)
P="$PDIR/pratinjau.txt"
cek "pratinjau.txt ada" "[ -s '$P' ]"
cek "HAPUS leaves = 3" "grep -qx \$'HAPUS\tleaves\t3' '$P'"
cek "HAPUS overtime_requests = 1" "grep -qx \$'HAPUS\tovertime_requests\t1' '$P'"
cek "FILE_LAIN dicatat" "grep -qx \$'FILE_LAIN\tleave-attachments/$B/lain.pdf' '$P'"
cek "inventaris FK attendance_requests → attendance" "grep -q \$'^FK\thris.attendance_requests\tattendance_requests_attendance_id_fkey' '$P'"
cek "inventaris trigger notifikasi absensi" "grep -q \$'^TRIGGER\thris.attendance\ttrg_notify_attendance_changes\tO' '$P'"
cek "files.txt = 4 path FILE (tanpa FILE_LAIN dan berkas baris tetap)" \
  "[ \"\$(sort '$PDIR/files.txt' | tr '\n' ' ')\" = 'leave-attachments/$A/hapus-1.pdf leave-attachments/$S/luar.pdf overtime-attachments/$A/hapus-2.pdf overtime-attachments/$A/hilang.pdf ' ]"
cek "berkas pratinjau mode 600" "[ \"\$(stat -c %a '$P')\" = 600 ]"
harus_gagal "files sebelum execute ditolak" "eksekusi" setuju files
harus_gagal "execute ditolak bila pratinjau dibuat untuk target lain" "target lain" \
  env DM01_APPROVED=yes DM01_DB_CONTAINER="$C" DM01_DB_NAME="$TEMPLATE" DM01_DB_USER=postgres DM01_VOLUME="$VOL" \
  DM01_BACKUP_DIR="$BK" bash "$RUNNER" execute

echo "== Penolakan payroll draft yang mencakup ≤ 20 Okt"
pg -c "INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status) VALUES ('Uji Oktober', 10, 2026, 'draft')"
harus_gagal "execute ditolak bila payroll draft periode 2026-10" "payroll" setuju execute
pg -c "DELETE FROM hris.payroll_runs WHERE run_name = 'Uji Oktober'"
pg <<SQL
INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date)
  VALUES (2026, 11, '2026-10-16', '2026-11-15', '2026-11-18');
INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status) VALUES ('Uji November', 11, 2026, 'draft');
SQL
harus_gagal "execute ditolak bila periode payroll draft mulai ≤ 20 Okt (attendance_periods)" "payroll" \
  setuju execute
pg -c "DELETE FROM hris.attendance_periods WHERE period_year = 2026 AND period_month = 11"
cek "DB belum berubah setelah penolakan" "[ \"\$(pg -c \"SELECT count(*) FROM hris.leaves WHERE employee_id = '$A'\")\" = 3 ]"
cek "belum ada dump setelah penolakan" "! ls '$BK'/dm01-*-eksekusi*/db.dump >/dev/null 2>&1"

echo "== Penolakan bila data berubah sejak pratinjau"
pg -c "INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status) VALUES ('$A', 'annual', '2026-10-12', '2026-10-12', 1, 'uji berubah', 'pending')"
harus_gagal "execute ditolak bila angka beda dari pratinjau" "berubah" setuju execute
pg -c "DELETE FROM hris.leaves WHERE reason = 'uji berubah'"

echo "== Eksekusi (payroll draft November tanpa periode khusus tidak menghalangi)"
out=$(DM01_APPROVED=yes run execute 2>&1) || { echo "$out" >&2; salah "execute berhasil"; exit 1; }
ok "execute berhasil"
echo "$out" | sed 's/^/      | /'
EDIR=$(ls -1d "$BK"/dm01-*-eksekusi* | tail -1)
cek "execute.ok ada" "[ -f '$EDIR/execute.ok' ]"
cek "db.dump ada, mode 600" "[ -s '$EDIR/db.dump' ] && [ \"\$(stat -c %a '$EDIR/db.dump')\" = 600 ]"
cek "DB cek restore sementara sudah dihapus" \
  "[ -z \"\$(docker exec $C psql -X -U postgres -Atc \"SELECT 1 FROM pg_database WHERE datname = 'paddydev_dm01_check'\")\" ]"
cek "cuti ≤ 20 Okt terhapus, cuti 25 Okt tetap" \
  "[ \"\$(pg -c \"SELECT string_agg(end_date::text, ',') FROM hris.leaves WHERE employee_id = '$A'\")\" = 2026-10-25 ]"
cek "lembur ≤ 20 Okt terhapus" "[ \"\$(pg -c \"SELECT count(*) FROM hris.overtime_requests WHERE date <= '2026-10-20'\")\" = 0 ]"
cek "satu baris audit data_cleanup.dm01" "[ \"\$(pg -c \"SELECT count(*) FROM audit.audit_log WHERE action = 'data_cleanup.dm01'\")\" = 1 ]"
cek "payroll draft November masih ada" "[ \"\$(pg -c \"SELECT count(*) FROM hris.payroll_runs WHERE run_name = 'Uji November'\")\" = 1 ]"

docker exec "$C" createdb -U postgres -T template0 "$RESTORE_DB"
if docker exec -i "$C" pg_restore -U postgres -d "$RESTORE_DB" --no-owner --no-acl --exit-on-error <"$EDIR/db.dump"; then
  ok "db.dump bisa di-restore"
else salah "db.dump bisa di-restore"; fi
cek "restore berisi data sebelum eksekusi (3 cuti uji)" \
  "[ \"\$(docker exec $C psql -X -U postgres -d $RESTORE_DB -Atc \"SELECT count(*) FROM hris.leaves WHERE employee_id = '$A'\")\" = 3 ]"
harus_gagal "execute kedua tanpa pratinjau baru tetap butuh angka yang sama" "berubah|pratinjau" \
  setuju execute

echo "== Hapus berkas"
cek "berkas belum dihapus sebelum mode files" "ada leave-attachments/$A/hapus-1.pdf"
out=$(DM01_APPROVED=yes run files 2>&1) || { echo "$out" >&2; salah "files berhasil"; exit 1; }
ok "files berhasil"
echo "$out" | sed 's/^/      | /'
cek "keluaran: dihapus 2" "grep -qE 'dihapus *: *2\$' <<<\"\$out\""
cek "keluaran: tidak ada 1" "grep -qE 'sudah tidak ada *: *1\$' <<<\"\$out\""
cek "keluaran: dilewati 1 (folder symlink)" "grep -qE 'dilewati \\(symlink\\) *: *1\$' <<<\"\$out\""
cek "berkas di luar private/ lewat symlink tidak disentuh" "vol 'test -f /s/luar/luar.pdf'"
cek "hapus-1.pdf terhapus" "! ada leave-attachments/$A/hapus-1.pdf"
cek "hapus-2.pdf terhapus" "! ada overtime-attachments/$A/hapus-2.pdf"
cek "berkas baris tetap masih ada" "ada leave-attachments/$A/tetap.pdf"
cek "berkas FILE_LAIN masih ada" "ada leave-attachments/$B/lain.pdf"
cek "berkas tak dirujuk masih ada" "ada leave-attachments/$A/tak-dirujuk.pdf"
TGZ=$(ls -1 "$EDIR"/files-*.tgz | tail -1)
cek "tar berisi tepat 2 path yang dihapus" \
  "[ \"\$(tar -tzf '$TGZ' | sort | tr '\n' ' ')\" = 'leave-attachments/$A/hapus-1.pdf overtime-attachments/$A/hapus-2.pdf ' ]"
cek "isi tar utuh" "[ \"\$(tar -xzOf '$TGZ' leave-attachments/$A/hapus-1.pdf)\" = satu ]"
cek "tar mode 600" "[ \"\$(stat -c %a '$TGZ')\" = 600 ]"
harus_gagal "files kedua kali ditolak" "sudah" setuju files

if [ "$GAGAL" = 0 ]; then echo PASS; else echo FAIL; exit 1; fi

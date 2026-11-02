#!/usr/bin/env bash
# Tes runner cleanup DM-01 (EPIC-083): tools/ops/dm01-cleanup.sh.
#
# Hanya memakai DB uji lokal dan volume docker sementara:
#   - database sementara dm01_runner_test_<pid>, dibuat dari template DB uji (createdb -T) di
#     container DB uji, lalu dihapus di akhir (DB uji bersama tidak disentuh);
#   - volume docker sementara dm01-runner-test-<pid>, dihapus di akhir;
#   - folder backup sementara (mktemp), dihapus di akhir;
#   - kegagalan disimulasikan dengan pembungkus `docker` dan `df` di PATH (hanya di tes; runner tidak
#     punya kait khusus tes).
# TIDAK PERNAH memakai paddydev-db atau paddydev-storage.
#
# Pemakaian: bash tools/ops/dm01-cleanup.test.sh
#   DM01_TEST_CONTAINER (bawaan paddydev-sdd-testdb), DM01_TEST_TEMPLATE (bawaan paddy_test_e071).
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RUNNER="$ROOT/tools/ops/dm01-cleanup.sh"
C=${DM01_TEST_CONTAINER:-paddydev-sdd-testdb}
TEMPLATE=${DM01_TEST_TEMPLATE:-paddy_test_e071}
DB=dm01_runner_test_$$
RESTORE_DB=dm01_runner_restore_$$
VOL=dm01-runner-test-$$
WORK=""

case "$C" in paddydev-db|*prod*) echo "TOLAK: tes tidak boleh memakai $C" >&2; exit 1 ;; esac

cleanup() {
  docker exec "$C" dropdb -U postgres --if-exists "$DB" >/dev/null 2>&1 || true
  docker exec "$C" dropdb -U postgres --if-exists "$RESTORE_DB" >/dev/null 2>&1 || true
  docker exec "$C" dropdb -U postgres --if-exists paddydev_dm01_check >/dev/null 2>&1 || true
  docker exec "$C" dropdb -U postgres --if-exists paddydev_dm01_restore >/dev/null 2>&1 || true
  docker volume rm -f "$VOL" >/dev/null 2>&1 || true
  if [ -n "$WORK" ]; then rm -rf -- "$WORK"; fi
}
trap cleanup EXIT

WORK=$(mktemp -d)
BK="$WORK/backups"
IMAGE=$(docker inspect -f '{{.Config.Image}}' "$C")

# Pembungkus untuk simulasi kegagalan (DM01_TEST_GAGAL):
#   inventaris  query inventaris FK/trigger gagal
#   pg_dump     pg_dump gagal
#   disk_backup df folder backup melaporkan hampir penuh
#   disk_data   df volume data Postgres (di container) hampir penuh mulai panggilan ke-DM01_TEST_KE
# Selain itu diteruskan apa adanya ke program asli.
mkdir -p "$WORK/bin"
cat >"$WORK/bin/docker" <<'SH'
#!/usr/bin/env bash
real=$DM01_TEST_DOCKER_ASLI
case "${DM01_TEST_GAGAL:-}" in
  pg_dump)
    if [[ " $* " == *" pg_dump "* ]]; then echo "pg_dump: error: simulasi gagal" >&2; exit 1; fi ;;
  inventaris)
    if [[ " $* " == *" psql "* && " $* " == *" -f - "* ]]; then
      t=$(mktemp); cat >"$t"
      if grep -q pg_constraint "$t"; then rm -f "$t"; echo "ERROR:  simulasi inventaris gagal" >&2; exit 1; fi
      rc=0; "$real" "$@" <"$t" || rc=$?; rm -f "$t"; exit "$rc"
    fi ;;
  disk_data)
    if [[ "$*" == *"df -Pk"* ]]; then
      n=$(( $(cat "$DM01_TEST_HITUNG" 2>/dev/null || echo 0) + 1 )); echo "$n" >"$DM01_TEST_HITUNG"
      if [ "$n" -ge "${DM01_TEST_KE:-1}" ]; then echo "/dev/uji 1000 990 10 99% /var/lib/postgresql/data"; exit 0; fi
    fi ;;
esac
exec "$real" "$@"
SH
cat >"$WORK/bin/df" <<'SH'
#!/usr/bin/env bash
if [ "${DM01_TEST_GAGAL:-}" = disk_backup ]; then
  echo "Filesystem 1024-blocks Used Available Capacity Mounted on"
  echo "/dev/uji 1000 990 10 99% /"
  exit 0
fi
exec "$DM01_TEST_DF_ASLI" "$@"
SH
chmod +x "$WORK/bin/docker" "$WORK/bin/df"
DOCKER_ASLI=$(command -v docker)
DF_ASLI=$(command -v df)
gagal_di() { # <inventaris|pg_dump|disk_backup|disk_data> <perintah…>
  local apa=$1
  shift
  rm -f "$WORK/hitung"
  DM01_TEST_GAGAL=$apa DM01_TEST_DOCKER_ASLI=$DOCKER_ASLI DM01_TEST_DF_ASLI=$DF_ASLI DM01_TEST_HITUNG="$WORK/hitung" \
    PATH="$WORK/bin:$PATH" "$@"
}

A=aaaaaaaa-0000-4000-8000-00000000000a   # karyawan pemilik berkas
B=bbbbbbbb-0000-4000-8000-00000000000b   # karyawan lain
S=cccccccc-0000-4000-8000-00000000000c   # foldernya symlink ke luar private/
ATT=dddddddd-0000-4000-8000-00000000000d # absensi ≤ 20 Okt yang dirujuk pengajuan 22 Okt
REQ=eeeeeeee-0000-4000-8000-00000000000e # pengajuan absensi 22 Okt (tetap) → ATT

# search_path sama dengan sesi aplikasi: fungsi trigger memakai nama tabel tanpa schema.
SP='public,iam,configuration,hris,performance,recruitment,item,purchasing,inventory,manufacturing,pos,crm,accounting,auth'
pg() { docker exec -i -e PGOPTIONS="-c search_path=$SP" "$C" psql -X -q -U postgres -d "$DB" -v ON_ERROR_STOP=1 -At "$@"; }
vol() { docker run --rm -i --network none --entrypoint sh -v "$VOL:/s" "$IMAGE" -c "$1"; }
run() {
  env DM01_DB_CONTAINER="$C" DM01_DB_NAME="$DB" DM01_DB_USER=postgres DM01_VOLUME="$VOL" \
    DM01_BACKUP_DIR="$BK" bash "$RUNNER" "$@"
}
PDIR=""
EDIR=""
eksekusi() { DM01_APPROVED=yes DM01_PREVIEW_DIR="$PDIR" run execute; }
eksekusi_ack() { DM01_PAYROLL_ACK=yes eksekusi; }
berkas() { DM01_APPROVED=yes DM01_EXEC_DIR="$EDIR" run files; }
pulihkan() { DM01_RESTORE_APPROVED=yes run restore "$EDIR"; }

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
harus_lolos() { # <label> <perintah…>; keluaran di $out
  local label=$1
  shift
  if out=$("$@" 2>&1); then ok "$label"; echo "$out" | sed 's/^/      | /'; else
    salah "$label"; echo "$out" | sed 's/^/      | /' >&2; echo FAIL; exit 1
  fi
}
ada() { vol "test -f /s/private/$1"; }
# Isi baris uji ≤ 20 Okt per tabel (untuk membandingkan sebelum eksekusi dan sesudah pemulihan).
jejak() {
  pg -c "SELECT md5(string_agg(x, '|' ORDER BY x)) FROM (
    SELECT a::text AS x FROM hris.attendance a WHERE employee_id IN ('$A', '$S') AND date <= '2026-10-20'
    UNION ALL SELECT l::text FROM hris.leaves l WHERE employee_id IN ('$A', '$S') AND end_date <= '2026-10-20'
      AND reason <> 'baru setelah cleanup'
    UNION ALL SELECT o::text FROM hris.overtime_requests o WHERE employee_id = '$A' AND date <= '2026-10-20') t"
}
stamp_lalu() { TZ=Asia/Jakarta date -d "$1" +%Y%m%d-%H%M%S; } # <berapa lalu>

echo "== Persiapan: DB sementara $DB dari $TEMPLATE, volume $VOL"
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
-- gaji aktif tanpa perusahaan: gerbang payroll butuh run final Oktober 2026
INSERT INTO hris.employee_salary (employee_id, base_salary, effective_date, is_active) VALUES ('$A', 5000000, '2025-01-01', true);
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
-- absensi 15 Okt (dihapus) dirujuk pengajuan 22 Okt (tetap): ON DELETE SET NULL, dipulihkan restore
INSERT INTO hris.attendance (id, employee_id, date, clock_in, clock_out, status)
  VALUES ('$ATT', '$A', '2026-10-15', '2026-10-15 08:00+07', '2026-10-15 17:00+07', 'present');
INSERT INTO hris.attendance_requests (id, employee_id, date, kind, reason, clock_in, attendance_id)
  VALUES ('$REQ', '$A', '2026-10-22', 'correction', 'uji tautan absensi', '08:00', '$ATT');
SQL

vol "set -e; mkdir -p /s/private/leave-attachments/$A /s/private/leave-attachments/$B /s/private/overtime-attachments/$A
  echo satu > /s/private/leave-attachments/$A/hapus-1.pdf
  echo dua > /s/private/overtime-attachments/$A/hapus-2.pdf
  echo tetap > /s/private/leave-attachments/$A/tetap.pdf
  echo lain > /s/private/leave-attachments/$B/lain.pdf
  echo tak-dirujuk > /s/private/leave-attachments/$A/tak-dirujuk.pdf
  mkdir -p /s/luar && echo luar > /s/luar/luar.pdf && ln -s /s/luar /s/private/leave-attachments/$S"

unset DM01_APPROVED DM01_PREVIEW_DIR DM01_PREVIEW_SHA256 DM01_PAYROLL_ACK DM01_EXEC_DIR DM01_RESTORE_APPROVED

echo "== Penolakan sebelum pratinjau"
harus_gagal "execute tanpa DM01_APPROVED=yes ditolak" "DM01_APPROVED" run execute
harus_gagal "execute tanpa DM01_PREVIEW_DIR ditolak" "DM01_PREVIEW_DIR wajib" eksekusi
harus_gagal "files tanpa DM01_APPROVED=yes ditolak" "DM01_APPROVED" run files
harus_gagal "files tanpa DM01_EXEC_DIR ditolak" "DM01_EXEC_DIR wajib" berkas
harus_gagal "restore tanpa DM01_RESTORE_APPROVED=yes ditolak" "DM01_RESTORE_APPROVED" run restore "$BK"
harus_gagal "mode tak dikenal ditolak" "pemakaian|mode" run hapus-semua

echo "== Pratinjau"
harus_gagal "preview gagal bila query inventaris FK/trigger gagal" "inventaris" gagal_di inventaris run preview
cek "preview gagal tidak meninggalkan folder pratinjau" "! ls -d '$BK'/dm01-*-pratinjau* >/dev/null 2>&1"
harus_lolos "preview berhasil" run preview
PDIR=$(ls -1d "$BK"/dm01-*-pratinjau* | tail -1)
P="$PDIR/pratinjau.txt"
SHA=$(sha256sum "$P" | awk '{print $1}')
cek "pratinjau.txt ada" "[ -s '$P' ]"
cek "SHA-256 pratinjau dicetak" "grep -q 'SHA-256 pratinjau: $SHA' <<<\"\$out\""
cek "target.txt memuat host" "grep -q '^host=$(hostname) ' '$PDIR/target.txt'"
cek "HAPUS leaves = 3" "grep -qx \$'HAPUS\tleaves\t3' '$P'"
cek "HAPUS overtime_requests = 1" "grep -qx \$'HAPUS\tovertime_requests\t1' '$P'"
cek "HAPUS attendance = 1" "grep -qx \$'HAPUS\tattendance\t1' '$P'"
cek "FILE_LAIN dicatat" "grep -qx \$'FILE_LAIN\tleave-attachments/$B/lain.pdf' '$P'"
cek "inventaris FK attendance_requests → attendance" "grep -q \$'^FK\thris.attendance_requests\tattendance_requests_attendance_id_fkey' '$P'"
cek "inventaris trigger notifikasi absensi" "grep -q \$'^TRIGGER\thris.attendance\ttrg_notify_attendance_changes\tO' '$P'"
cek "pratinjau: payroll Oktober belum final" "grep -q \$'^PAYROLL_BELUM\t(tanpa perusahaan)\t2026-10\t' '$P'"
cek "files.txt = 4 path FILE (tanpa FILE_LAIN dan berkas baris tetap)" \
  "[ \"\$(sort '$PDIR/files.txt' | tr '\n' ' ')\" = 'leave-attachments/$A/hapus-1.pdf leave-attachments/$S/luar.pdf overtime-attachments/$A/hapus-2.pdf overtime-attachments/$A/hilang.pdf ' ]"
cek "berkas pratinjau mode 600" "[ \"\$(stat -c %a '$P')\" = 600 ]"
harus_gagal "execute ditolak bila pratinjau dibuat untuk target lain" "target lain" \
  env DM01_APPROVED=yes DM01_PREVIEW_DIR="$PDIR" DM01_DB_CONTAINER="$C" DM01_DB_NAME="$TEMPLATE" DM01_DB_USER=postgres \
  DM01_VOLUME="$VOL" DM01_BACKUP_DIR="$BK" bash "$RUNNER" execute

echo "== Umur dan SHA-256 pratinjau"
TUA="$BK/dm01-$(stamp_lalu '37 hours ago')-pratinjau"
cp -a -- "$PDIR" "$TUA"
tua() { DM01_APPROVED=yes DM01_PREVIEW_DIR="$TUA" DM01_PAYROLL_ACK=yes run execute; }
harus_gagal "execute ditolak bila pratinjau > 36 jam" "36 jam" tua
KEMARIN="$BK/dm01-$(stamp_lalu '24 hours ago')-pratinjau"
cp -a -- "$PDIR" "$KEMARIN"
sha_salah() { DM01_PREVIEW_SHA256=0000 eksekusi_ack; }
harus_gagal "execute ditolak bila SHA-256 beda dari DM01_PREVIEW_SHA256" "SHA-256" sha_salah

echo "== Payroll draft yang mencakup ≤ 20 Okt"
pg -c "INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status) VALUES ('Uji Oktober', 10, 2026, 'draft')"
harus_gagal "execute ditolak bila payroll draft periode 2026-10 (ACK tidak berlaku)" "draft" eksekusi_ack
pg -c "DELETE FROM hris.payroll_runs WHERE run_name = 'Uji Oktober'"
pg <<SQL
INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date)
  VALUES (2026, 11, '2026-10-16', '2026-11-15', '2026-11-18');
INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status) VALUES ('Uji November', 11, 2026, 'draft');
SQL
harus_gagal "execute ditolak bila periode payroll draft mulai ≤ 20 Okt (attendance_periods)" "draft" eksekusi_ack
pg -c "DELETE FROM hris.attendance_periods WHERE period_year = 2026 AND period_month = 11"

echo "== Gerbang payroll belum final / periode absensi terbuka"
harus_gagal "execute ditolak bila payroll Oktober belum final" "payroll belum final.*2026-10" eksekusi
pg -c "INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status) VALUES ('Uji Oktober final', 10, 2026, 'completed')"
pg -c "INSERT INTO hris.attendance_periods (period_year, period_month, start_date, end_date, closing_date, status)
  VALUES (2026, 10, '2026-10-01', '2026-10-31', '2026-11-03', 'open')"
harus_gagal "execute ditolak bila periode absensi ≤ 20 Okt belum ditutup" "periode absensi belum ditutup: 2026-10" eksekusi
cek "DB belum berubah setelah penolakan" "[ \"\$(pg -c \"SELECT count(*) FROM hris.leaves WHERE employee_id = '$A'\")\" = 3 ]"
cek "belum ada folder eksekusi setelah penolakan" "! ls -d '$BK'/dm01-*-eksekusi* >/dev/null 2>&1"

echo "== Penolakan bila data berubah sejak pratinjau"
pg -c "INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status) VALUES ('$A', 'annual', '2026-10-12', '2026-10-12', 1, 'uji berubah', 'pending')"
harus_gagal "execute ditolak bila angka beda dari pratinjau" "berubah" eksekusi_ack
pg -c "DELETE FROM hris.leaves WHERE reason = 'uji berubah'"

echo "== Ruang disk"
harus_gagal "execute ditolak bila folder backup < ukuran DB + 2 GiB" "folder backup .* kurang" gagal_di disk_backup eksekusi_ack
harus_gagal "execute ditolak bila volume data < 1,2 × DB + 2 GiB (sebelum dump)" "volume data Postgres .* kurang.*sebelum dump" \
  gagal_di disk_data eksekusi_ack
cek "belum ada dump setelah penolakan disk" "! ls '$BK'/dm01-*-eksekusi*/db.dump >/dev/null 2>&1"
dua_kali() { DM01_TEST_KE=2 gagal_di disk_data eksekusi_ack; }
harus_gagal "execute berhenti bila volume data penuh sesudah dump" "volume data Postgres .* kurang.*sesudah dump" dua_kali
cek "dump sudah ada tetapi core tidak jalan" \
  "ls '$BK'/dm01-*-eksekusi*/db.dump >/dev/null 2>&1 && [ \"\$(pg -c \"SELECT count(*) FROM audit.audit_log WHERE action = 'data_cleanup.dm01'\")\" = 0 ]"

echo "== pg_dump gagal: berhenti sebelum core"
harus_gagal "execute berhenti bila pg_dump gagal" "pg_dump gagal" gagal_di pg_dump eksekusi_ack
cek "data utuh setelah pg_dump gagal" "[ \"\$(pg -c \"SELECT count(*) FROM hris.leaves WHERE employee_id = '$A'\")\" = 3 ]"
cek "tidak ada audit cleanup setelah pg_dump gagal" "[ \"\$(pg -c \"SELECT count(*) FROM audit.audit_log WHERE action = 'data_cleanup.dm01'\")\" = 0 ]"
cek "tidak ada execute.ok setelah penolakan" "! ls '$BK'/dm01-*-eksekusi*/execute.ok >/dev/null 2>&1"

echo "== Eksekusi: pratinjau 24 jam lalu (< 36 jam), periode terbuka di-ACK, draft November tidak menghalangi"
JEJAK_AWAL=$(jejak)
PDIR=$KEMARIN
harus_lolos "execute berhasil" eksekusi_ack
EDIR=$(ls -1td "$BK"/dm01-*-eksekusi* | head -1)
cek "peringatan ACK tercetak dan dicatat" "grep -q 'DM01_PAYROLL_ACK=yes — gerbang' <<<\"\$out\" && grep -q PERIODE_TERBUKA '$EDIR/payroll-ack.txt'"
cek "gladi core di DB sementara lolos dengan durasi" "grep -qE 'Gladi core lolos dalam [0-9]+ ms' <<<\"\$out\""
cek "angka yang disetujui diteruskan ke core (psql -v → dm01.expect_*)" \
  "grep -qF \"set_config('dm01.expect_leaves', :'expect_leaves', true)\" '$EDIR/core-pembungkus.sql' && grep -A2 -w expect_leaves '$EDIR/core.log' | grep -qx ' 3'"
cek "execute.ok ada" "[ -f '$EDIR/execute.ok' ]"
cek "notifikasi Dihapus selama core = 0" "grep -q ': 0\$' '$EDIR/notifikasi.txt'"
cek "db.dump ada, mode 600" "[ -s '$EDIR/db.dump' ] && [ \"\$(stat -c %a '$EDIR/db.dump')\" = 600 ]"
cek "DB sementara sudah dihapus" \
  "[ -z \"\$(docker exec $C psql -X -U postgres -Atc \"SELECT 1 FROM pg_database WHERE datname = 'paddydev_dm01_check'\")\" ]"
cek "cuti ≤ 20 Okt terhapus, cuti 25 Okt tetap" \
  "[ \"\$(pg -c \"SELECT string_agg(end_date::text, ',') FROM hris.leaves WHERE employee_id = '$A'\")\" = 2026-10-25 ]"
cek "lembur ≤ 20 Okt terhapus" "[ \"\$(pg -c \"SELECT count(*) FROM hris.overtime_requests WHERE date <= '2026-10-20'\")\" = 0 ]"
cek "tautan pengajuan 22 Okt dikosongkan (SET NULL)" "[ \"\$(pg -c \"SELECT attendance_id IS NULL FROM hris.attendance_requests WHERE id = '$REQ'\")\" = t ]"
cek "satu baris audit data_cleanup.dm01" "[ \"\$(pg -c \"SELECT count(*) FROM audit.audit_log WHERE action = 'data_cleanup.dm01'\")\" = 1 ]"
cek "payroll draft November masih ada" "[ \"\$(pg -c \"SELECT count(*) FROM hris.payroll_runs WHERE run_name = 'Uji November'\")\" = 1 ]"

docker exec "$C" createdb -U postgres -T template0 "$RESTORE_DB"
if docker exec -i "$C" pg_restore -U postgres -d "$RESTORE_DB" --no-owner --no-acl --exit-on-error <"$EDIR/db.dump"; then
  ok "db.dump bisa di-restore"
else salah "db.dump bisa di-restore"; fi
cek "restore berisi data sebelum eksekusi (3 cuti uji)" \
  "[ \"\$(docker exec $C psql -X -U postgres -d $RESTORE_DB -Atc \"SELECT count(*) FROM hris.leaves WHERE employee_id = '$A'\")\" = 3 ]"
harus_gagal "execute kedua dengan pratinjau yang sama ditolak (angka berubah)" "berubah" eksekusi_ack

echo "== Hapus berkas"
harus_gagal "files ditolak untuk folder eksekusi yang belum berhasil" "execute.ok" \
  env DM01_APPROVED=yes DM01_EXEC_DIR="$(ls -1d "$BK"/dm01-*-eksekusi* | grep -vx "$EDIR" | head -1)" DM01_DB_CONTAINER="$C" \
  DM01_DB_NAME="$DB" DM01_DB_USER=postgres DM01_VOLUME="$VOL" DM01_BACKUP_DIR="$BK" bash "$RUNNER" files
cp -- "$EDIR/files.txt" "$WORK/files.txt.asli"
echo 'leave-attachments/../../etc/passwd' >>"$EDIR/files.txt"
harus_gagal "files ditolak bila files.txt memuat path di luar pola" "pola" berkas
cek "tidak ada berkas terhapus setelah penolakan pola" "ada leave-attachments/$A/hapus-1.pdf && ada overtime-attachments/$A/hapus-2.pdf"
cek "tidak ada arsip/files.ok setelah penolakan pola" "! ls '$EDIR'/files-*.tgz '$EDIR'/files.ok >/dev/null 2>&1"
cp -- "$WORK/files.txt.asli" "$EDIR/files.txt"
harus_lolos "files berhasil" berkas
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
harus_gagal "files kedua kali ditolak" "sudah" berkas

echo "== Pemulihan (restore)"
# Sesudah cleanup: cuti baru ≤ 20 Okt (id baru) dan perubahan pada cuti 25 Okt yang tetap.
pg -c "UPDATE hris.leaves SET reason = 'diubah setelah cleanup' WHERE employee_id = '$A' AND end_date = '2026-10-25'"
pg -c "INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
  VALUES ('$A', 'annual', '2026-10-14', '2026-10-14', 1, 'baru setelah cleanup', 'pending')"
BARU_AWAL=$(pg -c "SELECT md5(l::text) FROM hris.leaves l WHERE reason = 'baru setelah cleanup'")
TRIGGER_AWAL=$(pg -c "SELECT string_agg(tgrelid::regclass || '.' || tgname || '=' || tgenabled::text, ',' ORDER BY tgrelid::regclass::text, tgname) FROM pg_trigger
  WHERE NOT tgisinternal AND tgrelid IN ('hris.attendance'::regclass, 'hris.leaves'::regclass, 'hris.attendance_requests'::regclass)")
NOTIF_SEBELUM=$(pg -c "SELECT count(*) FROM public.notifications")
harus_lolos "restore berhasil" pulihkan
cek "baris yang dihapus kembali persis (isi baris sama dengan sebelum eksekusi)" "[ \"\$(jejak)\" = '$JEJAK_AWAL' ]"
cek "tautan pengajuan 22 Okt → absensi 15 Okt pulih" "[ \"\$(pg -c \"SELECT attendance_id FROM hris.attendance_requests WHERE id = '$REQ'\")\" = '$ATT' ]"
cek "cuti baru sesudah cleanup tidak disentuh" "[ \"\$(pg -c \"SELECT md5(l::text) FROM hris.leaves l WHERE reason = 'baru setelah cleanup'\")\" = '$BARU_AWAL' ]"
cek "cuti 25 Okt yang diubah sesudah cleanup tidak dikembalikan" \
  "[ \"\$(pg -c \"SELECT reason FROM hris.leaves WHERE employee_id = '$A' AND end_date = '2026-10-25'\")\" = 'diubah setelah cleanup' ]"
cek "trigger kembali seperti semula" "[ \"\$(pg -c \"SELECT string_agg(tgrelid::regclass || '.' || tgname || '=' || tgenabled::text, ',' ORDER BY tgrelid::regclass::text, tgname) FROM pg_trigger
  WHERE NOT tgisinternal AND tgrelid IN ('hris.attendance'::regclass, 'hris.leaves'::regclass, 'hris.attendance_requests'::regclass)\")\" = '$TRIGGER_AWAL' ]"
cek "pemulihan tidak membuat notifikasi" "[ \"\$(pg -c 'SELECT count(*) FROM public.notifications')\" = '$NOTIF_SEBELUM' ]"
cek "baris audit data_cleanup.dm01_restore" "[ \"\$(pg -c \"SELECT count(*) FROM audit.audit_log WHERE action = 'data_cleanup.dm01_restore'\")\" = 1 ]"
cek "berkas hapus-1/hapus-2 kembali dengan isi semula" \
  "[ \"\$(vol \"cat /s/private/leave-attachments/$A/hapus-1.pdf /s/private/overtime-attachments/$A/hapus-2.pdf\")\" = \$'satu\ndua' ]"
cek "berkas yang dikembalikan milik 1001:1001 mode 644" \
  "[ \"\$(vol \"stat -c '%u:%g %a' /s/private/leave-attachments/$A/hapus-1.pdf /s/private/overtime-attachments/$A/hapus-2.pdf\" | sort -u)\" = '1001:1001 644' ]"
cek "berkas lain tidak disentuh (tetap.pdf masih milik root)" "[ \"\$(vol \"stat -c '%u' /s/private/leave-attachments/$A/tetap.pdf\")\" = 0 ]"
cek "restore.ok ada" "[ -f '$EDIR/restore.ok' ]"
cek "tidak ada skrip pemulihan berisi data yang tertinggal" "! ls '$EDIR'/restore-*.sql >/dev/null 2>&1"
cek "DB sementara pemulihan sudah dihapus" \
  "[ -z \"\$(docker exec $C psql -X -U postgres -Atc \"SELECT 1 FROM pg_database WHERE datname = 'paddydev_dm01_restore'\")\" ]"
harus_gagal "restore kedua kali ditolak" "sudah selesai" pulihkan

echo "== Cek pasca-eksekusi: notifikasi Dihapus selama core"
# Trigger uji yang membuat notifikasi "Dihapus" saat cuti dihapus (seperti trigger notifikasi yang lolos).
pg <<'SQL'
CREATE FUNCTION public.dm01_uji_notif() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO public.notifications (user_id, title, message, type)
  SELECT id, 'Data Uji Dihapus', 'uji DM-01', 'alert' FROM configuration.users LIMIT 1;
  RETURN OLD;
END $$;
CREATE TRIGGER dm01_uji_notif AFTER DELETE ON hris.leaves FOR EACH ROW EXECUTE FUNCTION public.dm01_uji_notif();
SQL
harus_lolos "preview kedua berhasil" run preview
PDIR=$(ls -1td "$BK"/dm01-*-pratinjau* | head -1)
harus_gagal "execute gagal di cek pasca-eksekusi bila ada notifikasi Dihapus" "notifikasi .*Dihapus.*seharusnya 0" eksekusi_ack
EDIR2=$(ls -1td "$BK"/dm01-*-eksekusi* | head -1)
cek "tanpa execute.ok bila cek pasca-eksekusi gagal" "[ ! -f '$EDIR2/execute.ok' ] && [ -f '$EDIR2/core.log' ]"

if [ "$GAGAL" = 0 ]; then echo PASS; else echo FAIL; exit 1; fi

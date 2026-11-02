#!/usr/bin/env bash
# EPIC-083 / DM-01 — runner cleanup data HRIS bertanggal ≤ 20 Oktober 2026.
#
# Dasar: MOM 29 Oktober 2026 §2 No. 6, DM-01 ("database cleanup per tanggal 20 Oktober 2026 …
# didahului backup") dan K-22; ruling R1–R8 di docs/epics/EPIC-083-cleanup-data-20-oktober.md.
# RUNBOOK (perintah persis staging → production, contoh keluaran, tabel kegagalan, pemulihan):
#   docs/hris/runbook-dm01-cleanup.md
# SQL-nya: backend/database/sql/fixes/2026-11-03_dm01_preview.sql (baca-saja) dan
# 2026-11-03_dm01_cleanup_core.sql (hapus permanen, satu transaksi, jumlah harus = angka disetujui).
#
# Jalankan di host tempat container DB (paddydev-db) dan volume storage (paddydev-storage) berada.
#
# LANGKAH DAN PERSETUJUAN (R8) — urut, jangan dilompati:
#   1. tools/ops/dm01-cleanup.sh preview
#        Baca-saja. Menulis ~/backups/dm01-<stamp>-pratinjau/pratinjau.txt (jumlah per tabel,
#        rincian status, pengecualian, checksum leave_balances, berkas, inventaris FK/trigger,
#        payroll draft, payroll belum final, periode absensi belum ditutup) + SHA-256-nya, dan
#        files.txt (daftar FILE yang akan dihapus).
#   2. Kirim angka pratinjau + SHA-256 ke HR (Manager HR) dan tunggu PERSETUJUAN TERTULIS (K-22).
#      Di production: juga izin eksplisit user untuk jam eksekusi (jam sepi).
#   3. DM01_APPROVED=yes DM01_PREVIEW_DIR=<folder pratinjau yang disetujui> tools/ops/dm01-cleanup.sh execute
#        Menolak bila: DM01_APPROVED bukan "yes"; DM01_PREVIEW_DIR kosong; pratinjau lebih tua dari
#        36 jam; SHA-256 beda dari DM01_PREVIEW_SHA256 (bila diisi); target beda; ada payroll run
#        draft yang mencakup ≤ 20 Okt; ada perusahaan bergaji aktif tanpa payroll final atau periode
#        absensi ≤ 20 Okt yang belum ditutup (kecuali DM01_PAYROLL_ACK=yes); angka saat ini beda
#        dari pratinjau; ruang disk kurang (folder backup ≥ ukuran DB + 2 GiB, volume data Postgres
#        ≥ 1,2 × ukuran DB + 2 GiB, dicek sebelum dan sesudah dump).
#        Lalu: pg_dump -Fc → db.dump, pg_restore -l, restore ke DB sementara paddydev_dm01_check
#        (selalu dihapus), core SQL dicoba dulu di DB sementara itu (durasi = perkiraan lama kunci),
#        lalu core SQL di database asli (psql -1, jumlah terhapus wajib = angka disetujui), cek
#        pasca-eksekusi (0 tersisa, checksum leave_balances sama, audit = angka pratinjau, 0
#        notifikasi "Dihapus" selama core). Hasil di ~/backups/dm01-<stamp>-eksekusi/.
#        Uji restore (keputusan controller): HAPUS, CHECKSUM leave_balances dan FILE hasil restore
#        harus SAMA PERSIS dengan database sebelum eksekusi; total baris per tabel hanya
#        ditampilkan, karena absensi berjalan (clock-in) di antara pratinjau dan dump membuat total
#        yang ketat gagal tanpa alasan.
#   4. DM01_APPROVED=yes DM01_EXEC_DIR=<folder eksekusi> tools/ops/dm01-cleanup.sh files
#        Hanya baris FILE (bukan FILE_LAIN) dari folder eksekusi yang berhasil (execute.ok):
#        cek tidak ada baris DB yang masih merujuk, tar ke files-<stamp>.tgz, cocokkan isi tar
#        dengan daftar, baru hapus. Berkas yang sudah tidak ada dihitung, bukan error.
#   5. Salin folder ~/backups/dm01-* (db.dump, files-*.tgz) ke lokal. FILE_LAIN ditinjau manual.
#   Pemulihan: DM01_RESTORE_APPROVED=yes tools/ops/dm01-cleanup.sh restore <folder eksekusi>
#        Baris yang cocok dengan kriteria cleanup di db.dump dimasukkan lagi (ON CONFLICT DO NOTHING:
#        id/kunci unik yang bentrok dengan baris baru dilewati dan dicatat LEWAT; satu transaksi,
#        trigger dimatikan sementara), berkas dari files-*.tgz dikembalikan
#        tanpa menimpa (pemilik 1001:1001, mode 644). Baris yang dibuat sesudah cleanup tidak disentuh.
#
# Variabel (bawaan untuk paddydev/production):
#   DM01_DB_CONTAINER     container Postgres                     (paddydev-db)
#   DM01_DB_NAME          database                               (POSTGRES_DB container)
#   DM01_DB_USER          user database                          (POSTGRES_USER container, atau postgres)
#   DM01_VOLUME           volume storage aplikasi (/app/storage)  (paddydev-storage)
#   DM01_BACKUP_DIR       folder backup                          (~/backups)
#   DM01_SQL_DIR          folder SQL DM-01                        (backend/database/sql/fixes repo ini)
#   DM01_APPROVED         harus "yes" untuk execute dan files
#   DM01_PREVIEW_DIR      (execute, wajib) folder pratinjau yang disetujui HR
#   DM01_PREVIEW_SHA256   (execute, opsional) SHA-256 pratinjau.txt yang disetujui
#   DM01_PAYROLL_ACK      (execute) "yes" = lanjut walau ada payroll belum final / periode belum ditutup
#   DM01_EXEC_DIR         (files, wajib) folder eksekusi yang berkasnya dihapus
#   DM01_RESTORE_APPROVED (restore) harus "yes"
#   DM01_APP_OWNER        (restore) pemilik berkas yang dikembalikan  (1001:1001, uid aplikasi)
#
# Gerbang payroll (keputusan):
#   - DRAFT (selalu menolak): hris.payroll_runs status 'draft' dengan periode ≤ 2026-10 ATAU
#     tanggal mulai periodenya ≤ 2026-10-20.
#   - PAYROLL_BELUM (menolak kecuali DM01_PAYROLL_ACK=yes): untuk tiap periode payroll yang
#     beririsan dengan [tanggal tertua data yang dihapus, 2026-10-20] — dari absensi, pengajuan
#     absensi, cuti, lembur, perjalanan dinas; pinjaman tidak dihitung — setiap perusahaan yang
#     punya employee_salary aktif pada periode itu tetapi tidak punya payroll run selain draft
#     (run tanpa perusahaan = run lama untuk semua perusahaan).
#   - PERIODE_TERBUKA (menolak kecuali DM01_PAYROLL_ACK=yes): baris hris.attendance_periods dengan
#     start_date ≤ 2026-10-20 yang status-nya bukan 'closed'.
#   Periode = start/end_date di hris.attendance_periods bila barisnya ada, selain itu dihitung dari
#   hris.attendance_settings.cutoff_day seperti domain.ComputePeriod (0 = bulan kalender; 1–28 =
#   tanggal cutoff+1 bulan sebelumnya s.d. tanggal cutoff).
#
# Berkas: nilai FILE adalah path relatif di bawah <storage>/private (storage.savePrivate:
# <folder>/<employee_id>/<nama>); volume dipasang di /app/storage pada container aplikasi
# (tools/deploy-paddydev.sh). Path diterima hanya bila cocok dengan pola folder lampiran DM-01, dan
# berkas dilewati bila folder atau berkasnya symlink, sehingga tidak ada yang keluar dari
# <volume>/private.
set -euo pipefail
umask 077

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
DB_CONTAINER=${DM01_DB_CONTAINER:-paddydev-db}
VOLUME=${DM01_VOLUME:-paddydev-storage}
BACKUP_DIR=${DM01_BACKUP_DIR:-$HOME/backups}
SQL_DIR=${DM01_SQL_DIR:-$SCRIPT_DIR/../../backend/database/sql/fixes}
APP_OWNER=${DM01_APP_OWNER:-1001:1001}
PREVIEW_SQL="$SQL_DIR/2026-11-03_dm01_preview.sql"
CORE_SQL="$SQL_DIR/2026-11-03_dm01_cleanup_core.sql"
CHECK_DB=paddydev_dm01_check
SCRATCH_DB=paddydev_dm01_restore
CUTOFF=2026-10-20
MAX_AGE_SECONDS=$((36 * 3600))
GIB=1073741824
TABLES=(attendance_requests attendance leaves overtime_requests business_trips loans)
# Urutan pemulihan: induk sebelum anak (attendance_requests.attendance_id → attendance).
RESTORE_TABLES=(attendance attendance_requests leaves overtime_requests business_trips loans)
# Sama dengan pola FILE di pratinjau, dibatasi ke folder lampiran milik enam tabel.
FILE_RE='^(attendance|attendance-requests|leave-attachments|overtime-attachments|loan-attachments)/[0-9a-f-]+/[A-Za-z0-9._-]+$'
# search_path sesi aplikasi (backend/internal/platform/database.SearchPath): role DB production
# tidak menyetelnya, sedangkan fungsi trigger memakai nama tabel tanpa schema.
APP_SEARCH_PATH='public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth'

MODE=${1:-}
ARG=${2:-}

die() { echo "GAGAL: $*" >&2; exit 1; }
info() { echo "• $*"; }
stamp() { TZ=Asia/Jakarta date +%Y%m%d-%H%M%S; }

usage() {
  cat >&2 <<EOF
Pemakaian: $0 preview|execute|files|restore <folder eksekusi>
  preview   pratinjau baca-saja (langkah 1)
  execute   backup + uji restore + hapus data (DM01_APPROVED=yes, DM01_PREVIEW_DIR=<pratinjau disetujui>)
  files     arsipkan lalu hapus berkas FILE (DM01_APPROVED=yes, DM01_EXEC_DIR=<folder eksekusi>)
  restore   kembalikan baris dan berkas dari folder eksekusi (DM01_RESTORE_APPROVED=yes)
Lihat kepala skrip ini dan docs/hris/runbook-dm01-cleanup.md.
EOF
  exit 2
}

# --- pembersihan saat keluar (juga saat gagal / Ctrl-C) ---
CLEANUP_FILES=()
CONTAINER_FILES=()
DROP_DBS=()
CORE_STARTED=0    # 1 sejak core SQL dijalankan di database asli: data mungkin sudah ter-COMMIT
RESTORE_STARTED=0 # 1 sejak transaksi pemulihan dijalankan
RUN_DIR_NOW=""
on_exit() {
  local rc=$? f
  for f in "${CONTAINER_FILES[@]}"; do
    docker exec "$DB_CONTAINER" rm -f -- "$f" >/dev/null 2>&1 || true
  done
  for f in "${DROP_DBS[@]}"; do
    docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" --if-exists "$f" >/dev/null 2>&1 ||
      echo "PERINGATAN: DB sementara $f gagal dihapus; hapus manual (dropdb $f)" >&2
  done
  if [ "$rc" != 0 ] && [ "$CORE_STARTED" = 1 ]; then
    echo "PERINGATAN: berhenti SETELAH core SQL dijalankan. Data mungkin sudah ter-COMMIT (atau sudah di-rollback bila psql gagal)." >&2
    echo "  Jangan ulangi execute. Periksa dengan '$0 preview' (HAPUS 0 = sudah terhapus) dan pratinjau-sesudah.txt / core.log di ${RUN_DIR_NOW:-folder eksekusi}." >&2
  fi
  if [ "$rc" != 0 ] && [ "$RESTORE_STARTED" = 1 ]; then
    echo "PERINGATAN: pemulihan berhenti di tengah. Transaksi DB berjalan utuh atau tidak sama sekali; restore aman diulang (idempoten)." >&2
  fi
  for f in "${CLEANUP_FILES[@]}"; do rm -f -- "$f"; done
  exit "$rc"
}
trap on_exit EXIT
trap 'exit 130' INT TERM

require_approval() {
  [ "${DM01_APPROVED:-}" = yes ] ||
    die "mode $MODE menghapus data permanen dan butuh persetujuan tertulis HR atas angka pratinjau (K-22/R8). Setelah ada, jalankan dengan DM01_APPROVED=yes."
}

container_env() {
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$DB_CONTAINER" | sed -n "s/^$1=//p" | head -n1
}

resolve_env() {
  [ "$(docker inspect -f '{{.State.Running}}' "$DB_CONTAINER" 2>/dev/null)" = true ] ||
    die "container DB $DB_CONTAINER tidak ditemukan atau tidak berjalan"
  DB_USER=${DM01_DB_USER:-$(container_env POSTGRES_USER)}
  DB_USER=${DB_USER:-postgres}
  DB_NAME=${DM01_DB_NAME:-$(container_env POSTGRES_DB)}
  DB_NAME=${DB_NAME:-$DB_USER}
  TOOL_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$DB_CONTAINER")
  [ -f "$PREVIEW_SQL" ] || die "SQL pratinjau tidak ada: $PREVIEW_SQL"
  [ -f "$CORE_SQL" ] || die "SQL core tidak ada: $CORE_SQL"
  mkdir -p "$BACKUP_DIR"
  [ -w "$BACKUP_DIR" ] || die "folder backup tidak bisa ditulis: $BACKUP_DIR"
  TARGET="host=$(hostname) container=$DB_CONTAINER database=$DB_NAME volume=$VOLUME"
  info "Target: host $(hostname), container $DB_CONTAINER, database $DB_NAME (user $DB_USER), volume $VOLUME, backup $BACKUP_DIR"
}

# Folder pratinjau/eksekusi mencatat targetnya; mode berikutnya hanya memakai folder dengan target sama.
same_target() { # <folder>
  [ -f "$1/target.txt" ] && [ "$(cat "$1/target.txt")" = "$TARGET" ] ||
    die "folder $1 dibuat untuk target lain ($(cat "$1/target.txt" 2>/dev/null || echo 'tidak tercatat')), bukan $TARGET"
}

psql_db() { # <database> <argumen psql…>
  local db=$1
  shift
  docker exec -i "$DB_CONTAINER" psql -X -U "$DB_USER" -d "$db" -v ON_ERROR_STOP=1 "$@"
}

new_dir() { # <jenis> → folder baru dm01-<stamp>-<jenis>[-n]
  local base d n=1
  base="$BACKUP_DIR/dm01-$(stamp)-$1"
  d=$base
  until mkdir "$d" 2>/dev/null; do
    n=$((n + 1))
    [ "$n" -le 50 ] || die "tidak bisa membuat folder $base"
    d="$base-$n"
  done
  printf '%s\n' "$d"
}

table_array() { # ARRAY['hris.x', …]::regclass[]
  local arr="" t
  for t in "${TABLES[@]}"; do arr+="${arr:+, }'hris.$t'"; done
  printf "ARRAY[%s]::regclass[]" "$arr"
}

# Inventaris FK yang merujuk enam tabel dan trigger pada enam tabel: perbedaan struktur di salinan
# production (drift) terlihat di pratinjau dan dibandingkan saat execute.
inventory_sql() {
  cat <<SQL
SET search_path = pg_catalog;
SELECT format(E'FK\t%s\t%s\t%s\t%s', c.conrelid::regclass, c.conname, c.confrelid::regclass, pg_get_constraintdef(c.oid))
FROM pg_constraint c
WHERE c.contype = 'f' AND c.confrelid = ANY ($(table_array))
ORDER BY 1;
SELECT format(E'TRIGGER\t%s\t%s\t%s\t%s', t.tgrelid::regclass, t.tgname, t.tgenabled, pg_get_triggerdef(t.oid))
FROM pg_trigger t
WHERE NOT t.tgisinternal AND t.tgrelid = ANY ($(table_array))
ORDER BY 1;
SQL
}

# Payroll run draft yang periodenya mencakup ≤ 20 Okt (lihat kepala skrip).
payroll_draft_sql() {
  cat <<SQL
SELECT format(E'PAYROLL_DRAFT\t%s\t%s-%s\tmulai %s', r.run_name, r.period_year, lpad(r.period_month::text, 2, '0'), m.mulai)
FROM hris.payroll_runs r
LEFT JOIN hris.attendance_periods ap ON ap.period_year = r.period_year AND ap.period_month = r.period_month
CROSS JOIN (SELECT COALESCE((SELECT CASE WHEN cutoff_day BETWEEN 1 AND 28 THEN cutoff_day ELSE 0 END
                             FROM hris.attendance_settings LIMIT 1), 0) AS cut) s
CROSS JOIN LATERAL (
  SELECT COALESCE(ap.start_date,
                  CASE WHEN s.cut = 0 THEN make_date(r.period_year, r.period_month, 1)
                       ELSE (make_date(r.period_year, r.period_month, 1) - interval '1 month')::date + s.cut END) AS mulai
) m
WHERE r.status = 'draft'
  AND (m.mulai <= DATE '$CUTOFF' OR (r.period_year, r.period_month) <= (2026, 10))
ORDER BY r.period_year, r.period_month, r.run_name;
SQL
}

# Payroll belum final per perusahaan dan periode absensi belum ditutup (lihat kepala skrip).
gate_sql() {
  cat <<SQL
WITH cut AS (
  SELECT COALESCE((SELECT CASE WHEN cutoff_day BETWEEN 1 AND 28 THEN cutoff_day ELSE 0 END
                   FROM hris.attendance_settings LIMIT 1), 0) AS c
),
awal AS (
  SELECT min(d) AS d FROM (
    SELECT min(date) FROM hris.attendance WHERE date <= DATE '$CUTOFF'
    UNION ALL SELECT min(date) FROM hris.attendance_requests WHERE date <= DATE '$CUTOFF'
    UNION ALL SELECT min(start_date) FROM hris.leaves WHERE end_date <= DATE '$CUTOFF'
    UNION ALL SELECT min(date) FROM hris.overtime_requests WHERE date <= DATE '$CUTOFF'
    UNION ALL SELECT min(start_date) FROM hris.business_trips WHERE end_date <= DATE '$CUTOFF'
  ) x(d)
),
bulan AS (
  SELECT extract(year FROM m)::int AS y, extract(month FROM m)::int AS mo
  FROM awal, generate_series(date_trunc('month', awal.d), DATE '2026-11-01', interval '1 month') AS m
  WHERE awal.d IS NOT NULL
),
periode AS (
  SELECT b.y, b.mo,
         COALESCE(ap.start_date, CASE WHEN cut.c = 0 THEN make_date(b.y, b.mo, 1)
                                      ELSE (make_date(b.y, b.mo, 1) - interval '1 month')::date + cut.c END) AS mulai,
         COALESCE(ap.end_date, CASE WHEN cut.c = 0 THEN (make_date(b.y, b.mo, 1) + interval '1 month')::date - 1
                                    ELSE make_date(b.y, b.mo, cut.c) END) AS akhir
  FROM bulan b CROSS JOIN cut
  LEFT JOIN hris.attendance_periods ap ON ap.period_year = b.y AND ap.period_month = b.mo
),
relevan AS (
  SELECT p.* FROM periode p, awal WHERE p.mulai <= DATE '$CUTOFF' AND p.akhir >= awal.d
)
SELECT format(E'PAYROLL_BELUM\t%s\t%s-%s\t%s..%s', COALESCE(c.name, '(tanpa perusahaan)'), r.y, lpad(r.mo::text, 2, '0'), r.mulai, r.akhir)
FROM relevan r
JOIN LATERAL (
  SELECT DISTINCT s.company_id FROM hris.employee_salary s
  WHERE s.is_active AND (s.effective_date IS NULL OR s.effective_date <= r.akhir)
    AND (s.end_date IS NULL OR s.end_date >= r.mulai)
) sc ON true
LEFT JOIN configuration.companies c ON c.id = sc.company_id
WHERE NOT EXISTS (
  SELECT 1 FROM hris.payroll_runs pr
  WHERE pr.period_year = r.y AND pr.period_month = r.mo AND pr.status <> 'draft'
    AND (pr.company_id IS NULL OR pr.company_id IS NOT DISTINCT FROM sc.company_id)
)
ORDER BY r.y, r.mo, 1;
SELECT format(E'PERIODE_TERBUKA\t%s-%s\t%s\t%s..%s', ap.period_year, lpad(ap.period_month::text, 2, '0'), ap.status, ap.start_date, ap.end_date)
FROM hris.attendance_periods ap
WHERE ap.start_date <= DATE '$CUTOFF' AND ap.status <> 'closed'
ORDER BY ap.period_year, ap.period_month;
SQL
}

# run_preview <database> <berkas keluaran>: pratinjau + inventaris + payroll, ditulis atomik.
run_preview() {
  local db=$1 out=$2
  # Tiap perintah dicek sendiri (dalam grup di kiri || set -e tidak berlaku).
  if ! psql_db "$db" -f - <"$PREVIEW_SQL" >"$out.tmp"; then
    rm -f -- "$out.tmp"; die "pratinjau SQL gagal di database $db"
  fi
  if ! inventory_sql | psql_db "$db" -qAt -f - >>"$out.tmp"; then
    rm -f -- "$out.tmp"; die "inventaris FK/trigger gagal di database $db"
  fi
  if ! payroll_draft_sql | psql_db "$db" -qAt -f - >>"$out.tmp"; then
    rm -f -- "$out.tmp"; die "cek payroll draft gagal di database $db"
  fi
  if ! gate_sql | psql_db "$db" -qAt -f - >>"$out.tmp"; then
    rm -f -- "$out.tmp"; die "cek payroll/periode absensi gagal di database $db"
  fi
  if ! grep -q $'^CHECKSUM\tleave_balances\t' "$out.tmp" || ! grep -q $'^FK\t' "$out.tmp" ||
    ! grep -q $'^TRIGGER\t' "$out.tmp" || [ "$(grep -c $'^HAPUS\t' "$out.tmp")" != "${#TABLES[@]}" ]; then
    rm -f -- "$out.tmp"; die "keluaran pratinjau tidak lengkap ($db): CHECKSUM, FK, TRIGGER dan HAPUS per tabel wajib ada"
  fi
  mv -- "$out.tmp" "$out"
}

# grep yang membedakan "tidak ada baris" (rc 1, wajar) dari kegagalan grep (rc ≥ 2).
grep_ok() {
  local rc=0
  grep "$@" || rc=$?
  [ "$rc" -le 1 ] || { echo "GAGAL: grep $* (rc $rc)" >&2; return 2; }
}
count_of() { # <pola> <berkas> → jumlah baris cocok
  local n rc=0
  n=$(grep -cE "$1" "$2") || rc=$?
  [ "$rc" -le 1 ] || { echo "GAGAL: grep -c $1 $2 (rc $rc)" >&2; return 2; }
  printf '%s\n' "$n"
}
lines_of() { grep_ok -E "^$1"$'\t' "$2"; }

# Bagian pratinjau yang harus sama dengan yang disetujui HR (data ≤ 20 Okt dan struktur);
# TETAP, STATUS tetap, CHECKSUM dan gerbang payroll ikut berubah oleh aktivitas harian sehingga
# tidak dibandingkan (gerbang payroll dicek ulang langsung saat execute).
core_lines() {
  grep_ok -E $'^(HAPUS|PENGECUALIAN|BERKAS|FILE|FILE_LAIN|FK|TRIGGER)\t|^STATUS\t[^\t]+\thapus\t' "$1"
}

summary() { # <pratinjau.txt>
  local p=$1
  echo "Akan dihapus (≤ 20 Okt 2026):"
  lines_of HAPUS "$p" | awk -F'\t' '{printf "  %-22s %s\n", $2, $3}'
  echo "Tetap ada (seluruh tabel):"
  lines_of TETAP "$p" | awk -F'\t' '{printf "  %-22s %s\n", $2, $3}'
  if [ -n "$(lines_of PENGECUALIAN "$p")" ]; then
    echo "Pengecualian (≤ 20 Okt tetapi TIDAK dihapus):"
    lines_of PENGECUALIAN "$p" | awk -F'\t' '{printf "  %-22s %s: %s\n", $2, $3, $4}'
  fi
  echo "Berkas: $(lines_of FILE "$p" | wc -l) FILE dihapus otomatis, $(lines_of FILE_LAIN "$p" | wc -l) FILE_LAIN ditinjau manual"
  echo "Inventaris: $(lines_of FK "$p" | wc -l) FK merujuk enam tabel, $(lines_of TRIGGER "$p" | wc -l) trigger"
  if [ -n "$(lines_of PAYROLL_DRAFT "$p")" ]; then
    echo "PERINGATAN: payroll draft yang mencakup ≤ 20 Okt (execute akan menolak):"
    lines_of PAYROLL_DRAFT "$p" | awk -F'\t' '{printf "  %s (%s, %s)\n", $2, $3, $4}'
  fi
  if [ -n "$(lines_of PAYROLL_BELUM "$p")" ]; then
    echo "PERINGATAN: perusahaan bergaji aktif tanpa payroll final pada periode ≤ 20 Okt (execute menolak kecuali DM01_PAYROLL_ACK=yes):"
    lines_of PAYROLL_BELUM "$p" | awk -F'\t' '{printf "  %s %s (%s)\n", $3, $2, $4}'
  fi
  if [ -n "$(lines_of PERIODE_TERBUKA "$p")" ]; then
    echo "PERINGATAN: periode absensi ≤ 20 Okt yang belum ditutup (execute menolak kecuali DM01_PAYROLL_ACK=yes):"
    lines_of PERIODE_TERBUKA "$p" | awk -F'\t' '{printf "  %s %s (%s)\n", $2, $3, $4}'
  fi
}

sha_of() { sha256sum "$1" | awk '{print $1}'; }

cmd_preview() {
  resolve_env
  local dir tmp
  # Folder pratinjau baru dibuat setelah pratinjau berhasil, jadi tidak ada folder pratinjau setengah jadi.
  tmp=$(mktemp "$BACKUP_DIR/.dm01-pratinjau-XXXXXX")
  CLEANUP_FILES+=("$tmp" "$tmp.tmp")
  run_preview "$DB_NAME" "$tmp"
  dir=$(new_dir pratinjau)
  printf '%s\n' "$TARGET" >"$dir/target.txt"
  mv -- "$tmp" "$dir/pratinjau.txt"
  (cd "$dir" && sha256sum pratinjau.txt >pratinjau.txt.sha256)
  lines_of FILE "$dir/pratinjau.txt" | cut -f2- >"$dir/files.txt"
  summary "$dir/pratinjau.txt"
  info "Pratinjau: $dir/pratinjau.txt"
  info "SHA-256 pratinjau: $(sha_of "$dir/pratinjau.txt")"
  info "Daftar berkas: $dir/files.txt"
  echo "Langkah berikutnya: kirim angka di atas + SHA-256 ke HR, tunggu persetujuan tertulis, lalu (≤ 36 jam)"
  echo "  DM01_APPROVED=yes DM01_PREVIEW_DIR=$dir DM01_PREVIEW_SHA256=$(sha_of "$dir/pratinjau.txt") $0 execute"
}

check_payroll_drafts() {
  local drafts
  drafts=$(payroll_draft_sql | psql_db "$DB_NAME" -qAt -f -) || die "cek payroll draft gagal"
  if [ -n "$drafts" ]; then
    echo "$drafts" | awk -F'\t' '{printf "  %s (%s, %s)\n", $2, $3, $4}' >&2
    die "ada payroll run berstatus draft yang periodenya mencakup ≤ 20 Okt 2026 (di atas). Selesaikan atau hapus run itu dulu: hitung ulangnya akan membaca data yang dihapus."
  fi
}

GATE_ACKED=""
check_payroll_gate() {
  local gate
  gate=$(gate_sql | psql_db "$DB_NAME" -qAt -f -) || die "cek payroll/periode absensi gagal"
  [ -n "$gate" ] || { info "Gerbang payroll/periode: bersih"; return 0; }
  echo "$gate" | awk -F'\t' '$1 == "PAYROLL_BELUM" {printf "  payroll belum final: %s %s (%s)\n", $3, $2, $4}
    $1 == "PERIODE_TERBUKA" {printf "  periode absensi belum ditutup: %s %s (%s)\n", $2, $3, $4}' >&2
  if [ "${DM01_PAYROLL_ACK:-}" != yes ]; then
    die "ada payroll belum final atau periode absensi ≤ 20 Okt yang belum ditutup (di atas). Finalkan payroll / tutup periode dulu, atau — bila HR menyatakan tertulis bahwa data itu sudah tercatat di luar sistem — jalankan ulang dengan DM01_PAYROLL_ACK=yes."
  fi
  cat >&2 <<'EOF'
################################################################################
##  PERINGATAN: DM01_PAYROLL_ACK=yes — gerbang payroll/periode DILEWATI.
##  Data absensi/cuti/lembur periode di atas akan dihapus walau payroll-nya belum
##  final atau periodenya belum ditutup. Pastikan ada persetujuan tertulis HR.
################################################################################
EOF
  GATE_ACKED=$gate
}

# Umur pratinjau dari nama foldernya (dm01-YYYYMMDD-HHMMSS-pratinjau[-n], WIB).
check_preview_age() { # <folder>
  local b now t age
  b=$(basename "$1")
  [[ "$b" =~ ^dm01-([0-9]{4})([0-9]{2})([0-9]{2})-([0-9]{2})([0-9]{2})([0-9]{2})-pratinjau(-[0-9]+)?$ ]] ||
    die "nama folder pratinjau tidak dikenal: $b (harus dm01-<YYYYMMDD-HHMMSS>-pratinjau dari mode preview)"
  t=$(TZ=Asia/Jakarta date -d "${BASH_REMATCH[1]}-${BASH_REMATCH[2]}-${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}" +%s) ||
    die "waktu pratinjau tidak terbaca dari $b"
  now=$(date +%s)
  age=$((now - t))
  [ "$age" -ge -300 ] || die "pratinjau $b bertanggal di masa depan; periksa jam server"
  [ "$age" -le "$MAX_AGE_SECONDS" ] ||
    die "pratinjau $b berumur $((age / 3600)) jam (> 36 jam). Jalankan preview lagi dan minta persetujuan HR atas angka baru."
  info "Umur pratinjau: $((age / 3600)) jam $((age % 3600 / 60)) menit (batas 36 jam)"
}

db_size() { # <database> → byte
  local s
  s=$(psql_db "$1" -qAt -c "SELECT pg_database_size(current_database())") || die "ukuran database $1 tidak terbaca"
  [[ "$s" =~ ^[0-9]+$ ]] || die "ukuran database $1 tidak valid: $s"
  printf '%s\n' "$s"
}

# Folder backup harus muat dump (≤ ukuran DB) + 2 GiB cadangan.
check_backup_space() { # <ukuran DB>
  local size=$1 avail need
  avail=$(df -Pk "$BACKUP_DIR" | tail -n 1 | awk '{print $4 * 1024}') || die "ruang kosong folder backup tidak terbaca"
  [[ "$avail" =~ ^[0-9]+$ ]] || die "ruang kosong folder backup tidak valid: $avail"
  need=$((size + 2 * GIB))
  info "Folder backup: kosong $((avail / 1048576)) MiB, perlu ≥ $((need / 1048576)) MiB (ukuran DB + 2 GiB)"
  [ "$avail" -ge "$need" ] ||
    die "ruang kosong folder backup $BACKUP_DIR ($((avail / 1048576)) MiB) kurang dari ukuran DB + 2 GiB ($((need / 1048576)) MiB); kosongkan disk dulu"
}

# DB sementara (uji restore / pemulihan) memakai volume data Postgres: ≥ 1,2 × ukuran DB + 2 GiB.
check_data_space() { # <ukuran DB> <keterangan>
  local size=$1 avail need
  avail=$(docker exec "$DB_CONTAINER" sh -c 'df -Pk "${PGDATA:-/var/lib/postgresql/data}" | tail -n 1' | awk '{print $4 * 1024}') ||
    die "ruang kosong volume data Postgres tidak terbaca"
  [[ "$avail" =~ ^[0-9]+$ ]] || die "ruang kosong volume data Postgres tidak valid: $avail"
  need=$((size + size / 5 + 2 * GIB))
  info "Volume data Postgres ($2): kosong $((avail / 1048576)) MiB, perlu ≥ $((need / 1048576)) MiB (1,2 × DB + 2 GiB)"
  [ "$avail" -ge "$need" ] ||
    die "ruang kosong volume data Postgres ($((avail / 1048576)) MiB) kurang dari 1,2 × ukuran DB + 2 GiB ($((need / 1048576)) MiB) untuk DB sementara ($2); kosongkan disk dulu"
}

# Pembungkus core: angka disetujui (psql -v expect_<tabel>) → dm01.expect_<tabel>, lalu \i core.
prepare_core() { # <folder eksekusi> <pratinjau disetujui>
  local dir=$1 approved=$2 t n wrapper base
  base="/tmp/dm01-core-$(basename "$dir")"
  CORE_ARGS=()
  wrapper="$dir/core-pembungkus.sql"
  {
    echo '\set ON_ERROR_STOP on'
    for t in "${TABLES[@]}"; do
      n=$(awk -F'\t' -v t="$t" '$1 == "HAPUS" && $2 == t {print $3}' "$approved")
      [[ "$n" =~ ^[0-9]+$ ]] || die "angka HAPUS $t tidak ada di $approved"
      CORE_ARGS+=(-v "expect_$t=$n")
      echo "SELECT set_config('dm01.expect_$t', :'expect_$t', true) AS expect_$t;"
    done
    echo "\\i $base.sql"
  } >"$wrapper"
  CONTAINER_FILES+=("$base.sql" "$base-pembungkus.sql")
  docker cp "$CORE_SQL" "$DB_CONTAINER:$base.sql" >/dev/null || die "salin core SQL ke container gagal"
  docker cp "$wrapper" "$DB_CONTAINER:$base-pembungkus.sql" >/dev/null || die "salin pembungkus core ke container gagal"
  CORE_WRAPPER="$base-pembungkus.sql"
}

run_core() { # <database> <log> → rc psql
  local rc=0
  psql_db "$1" -1 "${CORE_ARGS[@]}" -f "$CORE_WRAPPER" >"$2" 2>&1 || rc=$?
  cat -- "$2"
  return "$rc"
}

hapus_nol() { # <pratinjau> → 0 bila semua HAPUS = 0
  lines_of HAPUS "$1" | awk -F'\t' '$3 != 0 {bad=1} END {exit bad}'
}

cmd_execute() {
  require_approval
  resolve_env
  local approved_dir approved cur dir t size

  approved_dir=${DM01_PREVIEW_DIR:-}
  approved_dir=${approved_dir%/}
  [ -n "$approved_dir" ] ||
    die "DM01_PREVIEW_DIR wajib diisi dengan folder pratinjau yang angkanya disetujui HR (dm01-<stamp>-pratinjau)."
  approved="$approved_dir/pratinjau.txt"
  [ -f "$approved" ] || die "pratinjau tidak ada: $approved"
  info "Pratinjau yang disetujui: $approved"
  info "SHA-256 pratinjau: $(sha_of "$approved")"
  if [ -n "${DM01_PREVIEW_SHA256:-}" ] && [ "$(sha_of "$approved")" != "$DM01_PREVIEW_SHA256" ]; then
    die "SHA-256 $approved ($(sha_of "$approved")) beda dari DM01_PREVIEW_SHA256 ($DM01_PREVIEW_SHA256): bukan pratinjau yang disetujui"
  fi
  check_preview_age "$approved_dir"
  same_target "$approved_dir"

  check_payroll_drafts
  check_payroll_gate

  # Angka saat ini harus sama dengan yang disetujui.
  cur=$(mktemp "$BACKUP_DIR/.dm01-pratinjau-XXXXXX")
  CLEANUP_FILES+=("$cur" "$cur.tmp" "$cur.diff")
  run_preview "$DB_NAME" "$cur"
  if ! diff -u <(core_lines "$approved") <(core_lines "$cur") >"$cur.diff"; then
    head -n 60 "$cur.diff" >&2
    die "data atau struktur berubah sejak pratinjau yang disetujui ($approved). Jalankan preview lagi dan minta persetujuan HR atas angka baru."
  fi

  size=$(db_size "$DB_NAME")
  check_backup_space "$size"
  check_data_space "$size" "sebelum dump"

  dir=$(new_dir eksekusi)
  RUN_DIR_NOW=$dir
  printf '%s\n' "$TARGET" >"$dir/target.txt"
  info "Folder eksekusi: $dir"
  [ -z "$GATE_ACKED" ] || printf 'DM01_PAYROLL_ACK=yes oleh %s pada %s WIB\n%s\n' "$(id -un)" "$(stamp)" "$GATE_ACKED" >"$dir/payroll-ack.txt"
  cp -- "$approved" "$dir/pratinjau-disetujui.txt"
  sha_of "$approved" >"$dir/pratinjau-disetujui.sha256"
  mv -- "$cur" "$dir/pratinjau-sebelum.txt"
  lines_of FILE "$dir/pratinjau-sebelum.txt" | cut -f2- >"$dir/files.txt"

  # 1. Backup penuh.
  info "pg_dump -Fc → $dir/db.dump"
  docker exec "$DB_CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc >"$dir/db.dump" || die "pg_dump gagal"
  [ -s "$dir/db.dump" ] || die "db.dump kosong"
  docker exec -i "$DB_CONTAINER" pg_restore -l <"$dir/db.dump" >"$dir/db.toc" || die "pg_restore -l gagal: dump tidak terbaca"
  for t in "${TABLES[@]}" leave_balances; do
    grep -qE "TABLE DATA hris $t " "$dir/db.toc" || die "db.dump tidak memuat data hris.$t"
  done
  (cd "$dir" && sha256sum db.dump >db.dump.sha256)
  info "Ukuran db.dump: $(($(stat -c %s "$dir/db.dump") / 1048576)) MiB"

  # 2. Uji restore ke DB sementara; dibandingkan dengan pratinjau sebelum eksekusi.
  check_data_space "$size" "sesudah dump, sebelum uji restore"
  info "Uji restore ke DB sementara $CHECK_DB"
  DROP_DBS+=("$CHECK_DB")
  docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" --if-exists "$CHECK_DB" >/dev/null 2>&1 ||
    die "DB sisa $CHECK_DB tidak bisa dihapus (masih dipakai?)"
  docker exec "$DB_CONTAINER" createdb -U "$DB_USER" -T template0 "$CHECK_DB" || die "createdb $CHECK_DB gagal"
  docker exec -i "$DB_CONTAINER" pg_restore -U "$DB_USER" -d "$CHECK_DB" --no-owner --no-acl --exit-on-error \
    <"$dir/db.dump" >"$dir/restore.log" 2>&1 || die "restore db.dump ke $CHECK_DB gagal (lihat $dir/restore.log)"
  run_preview "$CHECK_DB" "$dir/pratinjau-restore.txt"
  if ! diff <(grep -E $'^(HAPUS|CHECKSUM|FILE)\t' "$dir/pratinjau-sebelum.txt") \
    <(grep -E $'^(HAPUS|CHECKSUM|FILE)\t' "$dir/pratinjau-restore.txt") >&2; then
    die "hasil restore berbeda dari database (HAPUS/CHECKSUM/FILE); eksekusi dibatalkan, tidak ada yang dihapus"
  fi
  echo "Jumlah baris per tabel (database saat pratinjau | hasil restore):"
  for t in "${TABLES[@]}"; do
    printf '  %-22s %s | %s\n' "$t" \
      "$(awk -F'\t' -v t="$t" '($1=="HAPUS"||$1=="TETAP") && $2==t {s+=$3} END {print s+0}' "$dir/pratinjau-sebelum.txt")" \
      "$(awk -F'\t' -v t="$t" '($1=="HAPUS"||$1=="TETAP") && $2==t {s+=$3} END {print s+0}' "$dir/pratinjau-restore.txt")"
  done
  info "Uji restore cocok"

  # 3. Gladi core di DB sementara (angka yang sama); durasinya = perkiraan lama kunci di database asli.
  prepare_core "$dir" "$approved"
  info "Gladi core SQL di $CHECK_DB"
  local t0 t1 core_rc=0
  t0=$(date +%s%N)
  run_core "$CHECK_DB" "$dir/core-gladi.log" >/dev/null || core_rc=$?
  t1=$(date +%s%N)
  [ "$core_rc" = 0 ] ||
    die "gladi core SQL gagal di DB sementara (psql rc $core_rc, lihat $dir/core-gladi.log); database asli TIDAK disentuh"
  run_preview "$CHECK_DB" "$dir/pratinjau-gladi-sesudah.txt"
  hapus_nol "$dir/pratinjau-gladi-sesudah.txt" || die "gladi: masih ada baris memenuhi kriteria hapus di DB sementara; database asli TIDAK disentuh"
  [ "$(lines_of CHECKSUM "$dir/pratinjau-sebelum.txt")" = "$(lines_of CHECKSUM "$dir/pratinjau-gladi-sesudah.txt")" ] ||
    die "gladi: checksum leave_balances berubah di DB sementara; database asli TIDAK disentuh"
  info "Gladi core lolos dalam $(((t1 - t0) / 1000000)) ms (perkiraan lama kunci tulis absensi/cuti di database asli)"
  docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" "$CHECK_DB" || die "dropdb $CHECK_DB gagal"
  info "DB sementara $CHECK_DB dihapus"

  # 4. Hapus data (satu transaksi; jumlah wajib = angka disetujui).
  echo "Angka yang disetujui HR ($approved):"
  lines_of HAPUS "$approved" | awk -F'\t' '{printf "  %-22s %s\n", $2, $3}'
  local mulai selesai
  mulai=$(psql_db "$DB_NAME" -qAt -c "SELECT clock_timestamp()") || die "jam database tidak terbaca"
  info "Menjalankan $(basename "$CORE_SQL") (psql -1)"
  CORE_STARTED=1
  core_rc=0
  run_core "$DB_NAME" "$dir/core.log" || core_rc=$?
  if [ "$core_rc" != 0 ]; then
    die "core SQL gagal (psql rc $core_rc); transaksi -1 dibatalkan sehingga tidak ada yang terhapus — pastikan dengan preview (lihat $dir/core.log)"
  fi
  selesai=$(psql_db "$DB_NAME" -qAt -c "SELECT clock_timestamp()") || die "jam database tidak terbaca"

  # 5. Cek pasca-eksekusi.
  run_preview "$DB_NAME" "$dir/pratinjau-sesudah.txt"
  local gagal=0 audit notif
  if ! hapus_nol "$dir/pratinjau-sesudah.txt"; then
    echo "PERINGATAN: masih ada baris yang memenuhi kriteria hapus:" >&2
    lines_of HAPUS "$dir/pratinjau-sesudah.txt" >&2
    gagal=1
  fi
  if [ -n "$(lines_of 'FILE(_LAIN)?' "$dir/pratinjau-sesudah.txt")" ]; then
    echo "PERINGATAN: pratinjau sesudah eksekusi masih memuat berkas" >&2
    gagal=1
  fi
  if [ "$(lines_of CHECKSUM "$dir/pratinjau-sebelum.txt")" != "$(lines_of CHECKSUM "$dir/pratinjau-sesudah.txt")" ]; then
    echo "PERINGATAN: checksum leave_balances berubah (sebelum vs sesudah) — lihat runbook (perubahan saldo cuti oleh aktivitas berjalan juga memicu ini)" >&2
    gagal=1
  fi
  audit=$(psql_db "$DB_NAME" -qAt -c "SELECT format(E'HAPUS\t%s\t%s', u.k, a.after->>u.k)
      FROM (SELECT after FROM audit.audit_log WHERE action = 'data_cleanup.dm01' ORDER BY created_at DESC, id DESC LIMIT 1) a,
           unnest(ARRAY['attendance_requests','attendance','leaves','overtime_requests','business_trips','loans'])
             WITH ORDINALITY AS u(k, i)
      ORDER BY u.i") || die "baca audit gagal"
  printf '%s\n' "$audit" >"$dir/audit-hapus.txt"
  if [ "$audit" != "$(lines_of HAPUS "$dir/pratinjau-sebelum.txt")" ]; then
    echo "PERINGATAN: jumlah di audit data_cleanup.dm01 berbeda dari pratinjau:" >&2
    diff <(lines_of HAPUS "$dir/pratinjau-sebelum.txt") "$dir/audit-hapus.txt" >&2 || true
    gagal=1
  fi
  # Notifikasi "… Dihapus" (trigger notifikasi absensi/cuti) yang dibuat selama core berjalan.
  notif=$(psql_db "$DB_NAME" -qAt -c "SELECT count(*) FROM public.notifications
      WHERE created_at BETWEEN '$mulai'::timestamptz AND '$selesai'::timestamptz AND title LIKE '%Dihapus%'") ||
    die "cek notifikasi gagal"
  echo "notifikasi Dihapus selama core ($mulai .. $selesai): $notif" >"$dir/notifikasi.txt"
  if [ "$notif" != 0 ]; then
    echo "PERINGATAN: $notif notifikasi \"Dihapus\" baru dibuat selama core berjalan (seharusnya 0)" >&2
    gagal=1
  fi
  if [ "$gagal" = 1 ]; then
    die "data SUDAH dihapus (COMMIT) tetapi cek pasca-eksekusi tidak lolos. Jangan jalankan 'files' sebelum diperiksa (runbook: tabel kegagalan); bila sudah dipastikan aman: touch $dir/execute.ok"
  fi
  {
    echo "eksekusi $(stamp) WIB oleh $(id -un)@$(hostname) — $DB_CONTAINER/$DB_NAME"
    cat "$dir/audit-hapus.txt"
  } >"$dir/execute.ok"

  echo "Terhapus (audit data_cleanup.dm01):"
  awk -F'\t' '{printf "  %-22s %s\n", $2, $3}' "$dir/audit-hapus.txt"
  info "Eksekusi selesai dan cek pasca-eksekusi lolos (notifikasi Dihapus: 0). Backup: $dir/db.dump"
  echo "Langkah berikutnya: DM01_APPROVED=yes DM01_EXEC_DIR=$dir $0 files"
}

# Dijalankan di container sementara: status per path dari stdin (ADA | HILANG | LEWATI).
# Folder dicek dengan readlink -f (tanpa symlink di mana pun sampai <volume>/private).
CLASSIFY_SH='
root=/s/private
[ -d "$root" ] && [ "$(readlink -f "$root")" = "$root" ] || { echo "ROOT_TIDAK_VALID"; exit 3; }
last=""; ok=0
while IFS= read -r p; do
  f="$root/$p"; d="${f%/*}"
  if [ "$d" != "$last" ]; then
    last=$d
    if [ "$(readlink -f "$d" 2>/dev/null)" = "$d" ]; then ok=1
    elif [ -e "$d" ] || [ -L "$d" ]; then ok=2
    else ok=0; fi
  fi
  if [ "$ok" = 0 ]; then s=HILANG
  elif [ "$ok" = 2 ] || [ -L "$f" ]; then s=LEWATI
  elif [ -f "$f" ]; then s=ADA
  elif [ -e "$f" ] || [ -L "$f" ]; then s=LEWATI
  else s=HILANG; fi
  printf "%s\t%s\n" "$s" "$p"
done'

tool() { # <ro|rw> <skrip sh> [folder host → /b:ro]; stdin diteruskan
  local extra=()
  [ -z "${3:-}" ] || extra=(-v "$3:/b:ro")
  docker run --rm -i --network none --entrypoint sh -v "$VOLUME:/s:$1" "${extra[@]}" "$TOOL_IMAGE" -c "$2"
}

check_paths() { # <daftar> → berhenti bila ada path di luar pola
  local bad
  bad=$(grep_ok -vE "$FILE_RE" "$1")
  [ -z "$bad" ] || {
    echo "$bad" | head -n 20 >&2
    die "$(echo "$bad" | wc -l) path di $1 tidak sesuai pola lampiran DM-01 (di atas); berhenti tanpa mengubah berkas apa pun"
  }
  if grep -qE '/(\.|\.\.)$' "$1"; then die "path '.'/'..' di $1; berhenti"; fi
}

cmd_files() {
  require_approval
  resolve_env
  local dir list n refs st status ada tgz hasil
  dir=${DM01_EXEC_DIR:-}
  dir=${dir%/}
  [ -n "$dir" ] || die "DM01_EXEC_DIR wajib diisi dengan folder eksekusi (dm01-<stamp>-eksekusi) yang berkasnya dihapus"
  [ -d "$dir" ] || die "folder eksekusi tidak ada: $dir"
  info "Folder eksekusi: $dir"
  same_target "$dir"
  [ -f "$dir/execute.ok" ] ||
    die "eksekusi di $dir belum berhasil (execute.ok tidak ada); berkas hanya dihapus setelah datanya terhapus"
  [ ! -f "$dir/files.ok" ] || die "penghapusan berkas untuk $dir sudah selesai (files.ok)"
  list="$dir/files.txt"
  [ -f "$list" ] || die "daftar berkas tidak ada: $list"
  n=$(count_of . "$list")
  check_paths "$list"
  if [ "$n" = 0 ]; then
    info "Tidak ada berkas FILE untuk dihapus"
    echo "files $(stamp): 0 berkas" >"$dir/files.ok"
    return 0
  fi
  docker volume inspect "$VOLUME" >/dev/null 2>&1 || die "volume $VOLUME tidak ada"

  # Tidak boleh ada baris (mana pun) yang masih merujuk berkas ini.
  refs=$({
    echo 'CREATE TEMP TABLE dm01_berkas (p text PRIMARY KEY); COPY dm01_berkas FROM STDIN;'
    sort -u "$list"
    printf '%s\n' '\.'
    cat <<'SQL'
WITH rujukan AS (
  SELECT clock_in_photo_url AS v FROM hris.attendance
  UNION ALL SELECT clock_out_photo_url FROM hris.attendance
  UNION ALL SELECT attachment_path FROM hris.attendance_requests
  UNION ALL SELECT attachment_url FROM hris.leaves
  UNION ALL SELECT unnest(attachments) FROM hris.overtime_requests
  UNION ALL SELECT unnest(attachments) FROM hris.loans
  UNION ALL SELECT unnest(collateral_attachments) FROM hris.loans
)
SELECT p FROM dm01_berkas WHERE p IN (SELECT v FROM rujukan WHERE v IS NOT NULL) ORDER BY p;
SQL
  } | psql_db "$DB_NAME" -qAt -f -) || die "cek rujukan berkas di database gagal"
  if [ -n "$refs" ]; then
    echo "$refs" | head -n 20 >&2
    die "$(echo "$refs" | wc -l) berkas masih dirujuk baris di database; berhenti tanpa menghapus apa pun"
  fi

  st=$(stamp)
  status="$dir/files-$st-status.txt"
  ada="$dir/files-$st-ada.txt"
  tgz="$dir/files-$st.tgz"
  hasil="$dir/files-$st-hasil.txt"

  sort -u "$list" | tool ro "$CLASSIFY_SH" >"$status" || die "pemeriksaan berkas di volume $VOLUME gagal"
  [ "$(count_of . "$status")" = "$(sort -u "$list" | wc -l)" ] || die "pemeriksaan berkas tidak lengkap"
  awk -F'\t' '$1 == "ADA" {print $2}' "$status" >"$ada"

  local n_ada n_hilang n_lewati n_hapus=0 n_gagal=0
  n_ada=$(count_of . "$ada")
  n_hilang=$(count_of $'^HILANG\t' "$status")
  n_lewati=$(count_of $'^LEWATI\t' "$status")

  if [ "$n_ada" -gt 0 ]; then
    info "Mengarsipkan $n_ada berkas → $tgz"
    tool ro 'cat >/tmp/daftar && tar -czf - -C /s/private -T /tmp/daftar' <"$ada" >"$tgz" || die "tar berkas gagal"
    (cd "$dir" && sha256sum "$(basename "$tgz")" >"$(basename "$tgz").sha256")
    if ! cmp -s <(sort "$ada") <(tool ro 'tar -tzf -' <"$tgz" | sort); then
      die "isi $tgz tidak sama dengan daftar berkas; berhenti tanpa menghapus apa pun"
    fi
    info "Isi arsip cocok dengan daftar ($n_ada path)"

    tool rw '
last=""; ok=0
while IFS= read -r p; do
  f="/s/private/$p"; d="${f%/*}"
  if [ "$d" != "$last" ]; then
    last=$d
    if [ "$(readlink -f "$d" 2>/dev/null)" = "$d" ]; then ok=1; else ok=0; fi
  fi
  if [ "$ok" != 1 ]; then printf "LEWATI\t%s\n" "$p"
  elif [ -f "$f" ] && [ ! -L "$f" ]; then
    if rm -f -- "$f" && [ ! -e "$f" ]; then printf "DIHAPUS\t%s\n" "$p"; else printf "GAGAL\t%s\n" "$p"; fi
  else printf "HILANG\t%s\n" "$p"; fi
done' <"$ada" >"$hasil" || die "penghapusan berkas terhenti (lihat $hasil)"
    local x
    n_hapus=$(count_of $'^DIHAPUS\t' "$hasil")
    n_gagal=$(count_of $'^GAGAL\t' "$hasil")
    x=$(count_of $'^HILANG\t' "$hasil")
    n_hilang=$((n_hilang + x))
    x=$(count_of $'^LEWATI\t' "$hasil")
    n_lewati=$((n_lewati + x))
  fi

  echo "Ringkasan berkas:"
  printf '  %-20s: %s\n' "daftar FILE" "$n" "diarsipkan (tar)" "$n_ada" "dihapus" "$n_hapus" \
    "sudah tidak ada" "$n_hilang" "dilewati (symlink)" "$n_lewati" "gagal" "$n_gagal"
  [ "$n_lewati" = 0 ] || info "Berkas dilewati (tinjau manual): lihat baris LEWATI di $status dan $hasil"
  [ "$n_gagal" = 0 ] || die "$n_gagal berkas gagal dihapus (lihat $hasil); jalankan files lagi setelah diperbaiki"
  echo "files $st: dihapus $n_hapus, sudah tidak ada $n_hilang, dilewati $n_lewati — arsip $tgz" >"$dir/files.ok"
  info "Selesai. Salin $dir ke lokal; tinjau FILE_LAIN di pratinjau secara manual."
}

# Predikat cleanup per tabel (sama dengan core SQL).
predicate() {
  case "$1" in
    attendance | attendance_requests | overtime_requests) echo "date <= DATE '$CUTOFF'" ;;
    leaves | business_trips) echo "end_date <= DATE '$CUTOFF'" ;;
    loans) echo "(requested_date AT TIME ZONE 'Asia/Jakarta')::date <= DATE '$CUTOFF'
      AND (status IN ('pending', 'manager_approved', 'rejected', 'cancelled', 'paid_off')
           OR (status = 'approved' AND COALESCE(remaining_balance, 0) <= 0))" ;;
    *) die "tabel tidak dikenal: $1" ;;
  esac
}

columns_of() { # <database> <tabel> → kolom (bukan generated), urut attnum, satu per baris
  psql_db "$1" -qAt -c "SELECT attname FROM pg_attribute WHERE attrelid = 'hris.$2'::regclass
    AND attnum > 0 AND NOT attisdropped AND attgenerated = '' ORDER BY attnum"
}

cmd_restore() {
  [ "${DM01_RESTORE_APPROVED:-}" = yes ] ||
    die "restore mengembalikan data DM-01 ke database asli; jalankan dengan DM01_RESTORE_APPROVED=yes setelah ada keputusan pemulihan"
  resolve_env
  local dir=${ARG%/} st t cols n_file scr size live_cols dump_cols only
  [ -n "$dir" ] || usage
  [ -d "$dir" ] || die "folder eksekusi tidak ada: $dir"
  info "Folder eksekusi: $dir"
  same_target "$dir"
  [ ! -f "$dir/restore.ok" ] || die "pemulihan dari $dir sudah selesai (restore.ok)"
  [ -s "$dir/db.dump" ] || die "db.dump tidak ada di $dir"
  (cd "$dir" && sha256sum -c --quiet db.dump.sha256) || die "SHA-256 db.dump tidak cocok dengan db.dump.sha256"
  st=$(stamp)

  # 1. Dump → DB sementara.
  size=$(db_size "$DB_NAME")
  check_data_space "$size" "DB sementara pemulihan"
  DROP_DBS+=("$SCRATCH_DB")
  docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" --if-exists "$SCRATCH_DB" >/dev/null 2>&1 ||
    die "DB sisa $SCRATCH_DB tidak bisa dihapus (masih dipakai?)"
  docker exec "$DB_CONTAINER" createdb -U "$DB_USER" -T template0 "$SCRATCH_DB" || die "createdb $SCRATCH_DB gagal"
  info "Restore db.dump ke DB sementara $SCRATCH_DB"
  docker exec -i "$DB_CONTAINER" pg_restore -U "$DB_USER" -d "$SCRATCH_DB" --no-owner --no-acl --exit-on-error \
    <"$dir/db.dump" >"$dir/restore-$st-scratch.log" 2>&1 || die "restore db.dump ke $SCRATCH_DB gagal (lihat $dir/restore-$st-scratch.log)"

  # 2. Skrip pemulihan (data lengkap dulu di berkas, baru dijalankan dalam satu transaksi).
  scr="$dir/restore-$st.sql"
  CLEANUP_FILES+=("$scr")
  {
    echo '\set ON_ERROR_STOP on'
    echo "SET LOCAL search_path = $APP_SEARCH_PATH;"
    echo "SET LOCAL lock_timeout = '10s';"
    cat <<SQL
DO \$\$
DECLARE n int;
BEGIN
  SELECT count(*) INTO n FROM pg_trigger
   WHERE NOT tgisinternal AND tgenabled = 'O'
     AND ((tgrelid = 'hris.attendance'::regclass AND tgname = 'trg_notify_attendance_changes')
       OR (tgrelid = 'hris.leaves'::regclass AND tgname = 'trg_notify_leave_changes'));
  IF n <> 2 THEN
    RAISE EXCEPTION 'DM-01 restore dibatalkan: trigger notifikasi absensi/cuti harus ada dan aktif sebelum pemulihan';
  END IF;
END \$\$;
SET LOCAL hris.allow_closed_period_write = 'on';
-- Semua trigger pengguna yang aktif pada enam tabel dimatikan selama pemulihan (baris kembali apa
-- adanya: tanpa notifikasi, tanpa hitung ulang jam kerja/total hari), lalu dinyalakan lagi.
CREATE TEMP TABLE dm01_trg ON COMMIT DROP AS
  SELECT tgrelid::regclass::text AS tabel, tgname FROM pg_trigger
  WHERE NOT tgisinternal AND tgenabled = 'O' AND tgrelid = ANY ($(table_array));
DO \$\$ DECLARE r record; BEGIN
  FOR r IN SELECT * FROM dm01_trg LOOP EXECUTE format('ALTER TABLE %s DISABLE TRIGGER %I', r.tabel, r.tgname); END LOOP;
END \$\$;
SQL
    for t in "${RESTORE_TABLES[@]}"; do
      live_cols=$(columns_of "$DB_NAME" "$t") || die "kolom hris.$t (database asli) tidak terbaca"
      dump_cols=$(columns_of "$SCRATCH_DB" "$t") || die "kolom hris.$t (dump) tidak terbaca"
      only=$(comm -23 <(sort <<<"$live_cols") <(sort <<<"$dump_cols") | paste -sd, -)
      [ -z "$only" ] || echo "PERINGATAN: kolom hris.$t yang tidak ada di dump memakai nilai bawaan: $only" >&2
      cols=$(grep -Fxf <(printf '%s\n' "$dump_cols") <<<"$live_cols" | sed 's/.*/"&"/' | paste -sd, -)
      [ -n "$cols" ] || die "tidak ada kolom bersama untuk hris.$t"
      n_file=$(psql_db "$SCRATCH_DB" -qAt -c "SELECT count(*) FROM hris.$t WHERE $(predicate "$t")") ||
        die "hitung baris dump hris.$t gagal"
      echo "CREATE TEMP TABLE dm01_r_$t ON COMMIT DROP AS SELECT $cols FROM hris.$t WITH NO DATA;"
      echo "COPY dm01_r_$t ($cols) FROM STDIN;"
      psql_db "$SCRATCH_DB" -qAt -c "COPY (SELECT $cols FROM hris.$t WHERE $(predicate "$t")) TO STDOUT" ||
        die "ekspor baris dump hris.$t gagal"
      printf '%s\n' '\.'
      echo "DO \$\$ BEGIN IF (SELECT count(*) FROM dm01_r_$t) <> $n_file THEN
  RAISE EXCEPTION 'DM-01 restore dibatalkan: baris hris.$t terbaca % dari $n_file', (SELECT count(*) FROM dm01_r_$t); END IF; END \$\$;"
      # ON CONFLICT tanpa target: id yang sudah ada ATAU bentrok kunci unik/eksklusi lain dengan baris
      # yang dibuat sesudah cleanup (mis. absensi baru di tanggal yang sama) → baris lama dilewati.
      echo "CREATE TEMP TABLE dm01_ins_$t ON COMMIT DROP AS SELECT id FROM hris.$t WITH NO DATA;
WITH ins AS (INSERT INTO hris.$t ($cols) SELECT $cols FROM dm01_r_$t ON CONFLICT DO NOTHING RETURNING id)
INSERT INTO dm01_ins_$t SELECT id FROM ins;
SELECT set_config('dm01.r_$t', (SELECT count(*) FROM dm01_ins_$t)::text, true), set_config('dm01.f_$t', '$n_file', true);
SELECT format(E'LEWAT\t$t\t%s', r.id) FROM dm01_r_$t r WHERE NOT EXISTS (SELECT 1 FROM dm01_ins_$t i WHERE i.id = r.id) ORDER BY 1;"
    done
    # Tautan attendance_requests.attendance_id yang dikosongkan ON DELETE SET NULL pada baris yang tetap.
    echo "CREATE TEMP TABLE dm01_r_tautan (id uuid, attendance_id uuid) ON COMMIT DROP;"
    echo "COPY dm01_r_tautan FROM STDIN;"
    psql_db "$SCRATCH_DB" -qAt -c "COPY (SELECT r.id, r.attendance_id FROM hris.attendance_requests r
      JOIN hris.attendance a ON a.id = r.attendance_id WHERE a.$(predicate attendance)) TO STDOUT" ||
      die "ekspor tautan pengajuan absensi gagal"
    printf '%s\n' '\.'
    cat <<SQL
WITH upd AS (
  UPDATE hris.attendance_requests r SET attendance_id = l.attendance_id
  FROM dm01_r_tautan l
  WHERE r.id = l.id AND r.attendance_id IS NULL AND EXISTS (SELECT 1 FROM hris.attendance a WHERE a.id = l.attendance_id)
  RETURNING 1
)
SELECT set_config('dm01.r_tautan', count(*)::text, true) FROM upd;
DO \$\$ DECLARE r record; BEGIN
  FOR r IN SELECT * FROM dm01_trg LOOP EXECUTE format('ALTER TABLE %s ENABLE TRIGGER %I', r.tabel, r.tgname); END LOOP;
END \$\$;
SET LOCAL hris.allow_closed_period_write = 'off';
INSERT INTO audit.audit_log (actor_id, actor_name, action, entity, entity_id, entity_label, after, reason)
SELECT NULL, 'Sistem (pemulihan DM-01)', 'data_cleanup.dm01_restore', 'hris', 'dm01',
       'Pemulihan cleanup data HRIS ≤ $CUTOFF',
       jsonb_build_object(
         'attendance', current_setting('dm01.r_attendance')::int,
         'attendance_requests', current_setting('dm01.r_attendance_requests')::int,
         'leaves', current_setting('dm01.r_leaves')::int,
         'overtime_requests', current_setting('dm01.r_overtime_requests')::int,
         'business_trips', current_setting('dm01.r_business_trips')::int,
         'loans', current_setting('dm01.r_loans')::int,
         'tautan_attendance_requests', current_setting('dm01.r_tautan')::int),
       'Pemulihan dari $(basename "$dir")/db.dump';
SELECT format(E'PULIH\t%s\t%s\t%s', t, current_setting('dm01.r_' || t), current_setting('dm01.f_' || t))
FROM unnest(ARRAY['attendance', 'attendance_requests', 'leaves', 'overtime_requests', 'business_trips', 'loans']) AS t;
SELECT format(E'PULIH\ttautan_attendance_requests\t%s', current_setting('dm01.r_tautan'));
SQL
  } >"$scr"
  docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" "$SCRATCH_DB" || die "dropdb $SCRATCH_DB gagal"

  info "Mengembalikan baris ke $DB_NAME (satu transaksi)"
  RESTORE_STARTED=1
  psql_db "$DB_NAME" -1 -qAt -f - <"$scr" >"$dir/restore-$st-db.txt" 2>&1 ||
    { cat "$dir/restore-$st-db.txt" >&2; die "pemulihan baris gagal; transaksi -1 dibatalkan sehingga database tidak berubah"; }
  rm -f -- "$scr"
  echo "Baris dikembalikan (dimasukkan / ada di dump):"
  grep_ok $'^PULIH\t' "$dir/restore-$st-db.txt" | awk -F'\t' '{printf "  %-28s %s%s\n", $2, $3, ($4 == "" ? "" : " / " $4)}'
  local lewat
  lewat=$(count_of $'^LEWAT\t' "$dir/restore-$st-db.txt")
  if [ "$lewat" != 0 ]; then
    echo "PERINGATAN: $lewat baris dilewati karena bentrok dengan baris yang sudah ada (id sama, atau kunci unik seperti absensi karyawan+tanggal dengan baris baru sesudah cleanup); baris yang ada dipertahankan. Daftar: baris LEWAT di $dir/restore-$st-db.txt"
  fi

  # 3. Berkas: hanya yang belum ada, tanpa menimpa.
  local tgz name entries klas pulih n_pulih=0 n_ada=0 n_lewati=0 x
  for tgz in "$dir"/files-*.tgz; do
    [ -f "$tgz" ] || continue
    name=$(basename "$tgz")
    if [ -f "$tgz.sha256" ]; then (cd "$dir" && sha256sum -c --quiet "$name.sha256") || die "SHA-256 $name tidak cocok"; fi
    entries="$dir/restore-$st-$name.isi"
    klas="$dir/restore-$st-$name.status"
    pulih="$dir/restore-$st-$name.pulih"
    tool ro "tar -tzf /b/$name" "$dir" | sort -u >"$entries" || die "isi $name tidak terbaca"
    check_paths "$entries"
    tool ro "$CLASSIFY_SH" <"$entries" >"$klas" || die "pemeriksaan berkas $name gagal"
    awk -F'\t' '$1 == "HILANG" {print $2}' "$klas" >"$pulih"
    x=$(count_of $'^ADA\t' "$klas"); n_ada=$((n_ada + x))
    x=$(count_of $'^LEWATI\t' "$klas"); n_lewati=$((n_lewati + x))
    [ -s "$pulih" ] || continue
    tool rw "set -e
tar -xzkf /b/$name -C /s/private -T /b/$(basename "$pulih")
while IFS= read -r p; do
  f=/s/private/\$p
  [ -f \"\$f\" ] && [ ! -L \"\$f\" ] || { echo \"GAGAL \$p\" >&2; exit 4; }
  chown $APP_OWNER \"\$f\"; chmod 644 \"\$f\"
  d=\${f%/*}
  while [ \"\$d\" != /s/private ]; do
    if [ \"\$(stat -c %u \"\$d\")\" = 0 ]; then chown $APP_OWNER \"\$d\"; chmod 755 \"\$d\"; fi
    d=\${d%/*}
  done
done < /b/$(basename "$pulih")" "$dir" || die "pengembalian berkas dari $name gagal"
    x=$(count_of . "$pulih"); n_pulih=$((n_pulih + x))
  done
  echo "Berkas: dikembalikan $n_pulih, sudah ada (tidak ditimpa) $n_ada, dilewati (symlink) $n_lewati"
  {
    echo "pemulihan $st WIB oleh $(id -un)@$(hostname) — $DB_CONTAINER/$DB_NAME"
    grep_ok $'^PULIH\t' "$dir/restore-$st-db.txt"
    echo "berkas dikembalikan $n_pulih, sudah ada $n_ada, dilewati $n_lewati"
  } >"$dir/restore.ok"
  info "Pemulihan selesai: $dir/restore.ok"
}

case "$MODE" in
  preview) cmd_preview ;;
  execute) cmd_execute ;;
  files) cmd_files ;;
  restore) cmd_restore ;;
  *) usage ;;
esac

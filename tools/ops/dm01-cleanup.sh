#!/usr/bin/env bash
# EPIC-083 / DM-01 — runner cleanup data HRIS bertanggal ≤ 20 Oktober 2026.
#
# Dasar: MOM 29 Oktober 2026 §2 No. 6, DM-01 ("database cleanup per tanggal 20 Oktober 2026 …
# didahului backup") dan K-22; ruling R1–R8 di docs/epics/EPIC-083-cleanup-data-20-oktober.md.
# SQL-nya: backend/database/sql/fixes/2026-11-03_dm01_preview.sql (baca-saja) dan
# 2026-11-03_dm01_cleanup_core.sql (hapus permanen, satu transaksi).
#
# Jalankan di host tempat container DB (paddydev-db) dan volume storage (paddydev-storage) berada.
#
# LANGKAH DAN PERSETUJUAN (R8) — urut, jangan dilompati:
#   1. tools/ops/dm01-cleanup.sh preview
#        Baca-saja. Menulis ~/backups/dm01-<stamp>-pratinjau/pratinjau.txt (jumlah per tabel,
#        rincian status, pengecualian, checksum leave_balances, berkas, inventaris FK/trigger,
#        payroll draft) dan files.txt (daftar FILE yang akan dihapus).
#   2. Kirim angka pratinjau ke HR (Manager HR) dan tunggu PERSETUJUAN TERTULIS (K-22).
#      Di production: juga izin eksplisit user untuk jam eksekusi (jam sepi).
#   3. DM01_APPROVED=yes tools/ops/dm01-cleanup.sh execute   (hari yang sama dengan pratinjau)
#        Menolak bila: DM01_APPROVED bukan "yes"; belum ada pratinjau hari ini (WIB); ada payroll
#        run draft yang periodenya mencakup ≤ 20 Okt; angka saat ini beda dari pratinjau yang
#        disetujui. Lalu: pg_dump -Fc → db.dump, pg_restore -l, restore ke DB sementara
#        paddydev_dm01_check dan bandingkan angkanya (DB sementara selalu dihapus), core SQL
#        (psql -1), cek pasca-eksekusi (0 yang tersisa untuk dihapus, checksum leave_balances
#        sama, baris audit = angka pratinjau). Hasil di ~/backups/dm01-<stamp>-eksekusi/.
#   4. DM01_APPROVED=yes tools/ops/dm01-cleanup.sh files
#        Hanya baris FILE (bukan FILE_LAIN) dari folder eksekusi terakhir yang berhasil:
#        cek tidak ada baris DB yang masih merujuk, tar ke files-<stamp>.tgz, cocokkan isi tar
#        dengan daftar, baru hapus. Berkas yang sudah tidak ada dihitung, bukan error.
#   5. Salin folder ~/backups/dm01-* (db.dump, files-*.tgz) ke lokal. FILE_LAIN ditinjau manual.
#
# Variabel (bawaan untuk paddydev/production):
#   DM01_DB_CONTAINER  container Postgres                     (paddydev-db)
#   DM01_DB_NAME       database                               (POSTGRES_DB container)
#   DM01_DB_USER       user database                          (POSTGRES_USER container, atau postgres)
#   DM01_VOLUME        volume storage aplikasi (/app/storage)  (paddydev-storage)
#   DM01_BACKUP_DIR    folder backup                          (~/backups)
#   DM01_SQL_DIR       folder SQL DM-01                        (backend/database/sql/fixes repo ini)
#   DM01_APPROVED      harus "yes" untuk execute dan files
#   DM01_RUN_DIR       (files) folder eksekusi tertentu; bawaan: folder eksekusi terakhir
#
# Payroll draft (keputusan): execute ditolak bila ada hris.payroll_runs status 'draft' dengan
# periode ≤ 2026-10, ATAU tanggal mulai periodenya ≤ 2026-10-20. Tanggal mulai = start_date di
# hris.attendance_periods bila baris periodenya ada, selain itu dihitung dari
# hris.attendance_settings.cutoff_day seperti domain.ComputePeriod (0 = tanggal 1; 1–28 = tanggal
# cutoff+1 bulan sebelumnya). Run draft seperti itu masih akan dihitung ulang dari data yang hilang.
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
PREVIEW_SQL="$SQL_DIR/2026-11-03_dm01_preview.sql"
CORE_SQL="$SQL_DIR/2026-11-03_dm01_cleanup_core.sql"
CHECK_DB=paddydev_dm01_check
TABLES=(attendance_requests attendance leaves overtime_requests business_trips loans)
# Sama dengan pola FILE di pratinjau, dibatasi ke folder lampiran milik enam tabel.
FILE_RE='^(attendance|attendance-requests|leave-attachments|overtime-attachments|loan-attachments)/[0-9a-f-]+/[A-Za-z0-9._-]+$'

MODE=${1:-}

die() { echo "GAGAL: $*" >&2; exit 1; }
info() { echo "• $*"; }
stamp() { TZ=Asia/Jakarta date +%Y%m%d-%H%M%S; }
today() { TZ=Asia/Jakarta date +%Y%m%d; }

usage() {
  cat >&2 <<EOF
Pemakaian: $0 preview|execute|files
  preview   pratinjau baca-saja (langkah 1)
  execute   backup + cek restore + hapus data (butuh DM01_APPROVED=yes dan pratinjau hari ini)
  files     arsipkan lalu hapus berkas lampiran FILE (butuh DM01_APPROVED=yes dan eksekusi berhasil)
Lihat kepala skrip ini untuk urutan persetujuan.
EOF
  exit 2
}

# --- pembersihan saat keluar (juga saat gagal / Ctrl-C) ---
CLEANUP_FILES=()
DROP_CHECK=0
on_exit() {
  local rc=$?
  if [ "$DROP_CHECK" = 1 ]; then
    docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" --if-exists "$CHECK_DB" >/dev/null 2>&1 ||
      echo "PERINGATAN: DB sementara $CHECK_DB gagal dihapus; hapus manual (dropdb $CHECK_DB)" >&2
  fi
  local f
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
  TARGET="container=$DB_CONTAINER database=$DB_NAME volume=$VOLUME"
  info "Target: container $DB_CONTAINER, database $DB_NAME (user $DB_USER), volume $VOLUME, backup $BACKUP_DIR"
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

latest_dir() { # <glob…> → folder terakhir (urut nama = urut waktu)
  local d last=""
  for d in "$@"; do [ -d "$d" ] && last=$d; done
  printf '%s' "$last"
}

# Inventaris FK yang merujuk enam tabel dan trigger pada enam tabel: perbedaan struktur di salinan
# production (drift) terlihat di pratinjau dan dibandingkan saat execute.
inventory_sql() {
  local arr="" t
  for t in "${TABLES[@]}"; do arr+="${arr:+, }'hris.$t'"; done
  cat <<SQL
SET search_path = pg_catalog;
SELECT format(E'FK\t%s\t%s\t%s\t%s', c.conrelid::regclass, c.conname, c.confrelid::regclass, pg_get_constraintdef(c.oid))
FROM pg_constraint c
WHERE c.contype = 'f' AND c.confrelid = ANY (ARRAY[$arr]::regclass[])
ORDER BY 1;
SELECT format(E'TRIGGER\t%s\t%s\t%s\t%s', t.tgrelid::regclass, t.tgname, t.tgenabled, pg_get_triggerdef(t.oid))
FROM pg_trigger t
WHERE NOT t.tgisinternal AND t.tgrelid = ANY (ARRAY[$arr]::regclass[])
ORDER BY 1;
SQL
}

# Payroll run draft yang periodenya mencakup ≤ 20 Okt (lihat kepala skrip).
payroll_draft_sql() {
  cat <<'SQL'
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
  AND (m.mulai <= DATE '2026-10-20' OR (r.period_year, r.period_month) <= (2026, 10))
ORDER BY r.period_year, r.period_month, r.run_name;
SQL
}

# run_preview <database> <berkas keluaran>: pratinjau + inventaris + payroll draft, ditulis atomik.
run_preview() {
  local db=$1 out=$2
  {
    psql_db "$db" -f - <"$PREVIEW_SQL"
    inventory_sql | psql_db "$db" -qAt -f -
    payroll_draft_sql | psql_db "$db" -qAt -f -
  } >"$out.tmp" || { rm -f -- "$out.tmp"; die "pratinjau gagal di database $db"; }
  grep -q $'^CHECKSUM\tleave_balances\t' "$out.tmp" || { rm -f -- "$out.tmp"; die "keluaran pratinjau tidak lengkap ($db)"; }
  mv -- "$out.tmp" "$out"
}

# Bagian pratinjau yang harus sama dengan yang disetujui HR (data ≤ 20 Okt dan struktur);
# TETAP, STATUS tetap dan CHECKSUM ikut berubah oleh aktivitas harian sehingga tidak dibandingkan.
core_lines() {
  grep -E $'^(HAPUS|PENGECUALIAN|BERKAS|FILE|FILE_LAIN|FK|TRIGGER)\t|^STATUS\t[^\t]+\thapus\t' "$1" || true
}

lines_of() { grep -E "^$1"$'\t' "$2" || true; }

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
}

cmd_preview() {
  resolve_env
  local dir
  dir=$(new_dir pratinjau)
  printf '%s\n' "$TARGET" >"$dir/target.txt"
  run_preview "$DB_NAME" "$dir/pratinjau.txt"
  lines_of FILE "$dir/pratinjau.txt" | cut -f2- >"$dir/files.txt"
  summary "$dir/pratinjau.txt"
  info "Pratinjau: $dir/pratinjau.txt"
  info "Daftar berkas: $dir/files.txt"
  echo "Langkah berikutnya: kirim angka di atas ke HR, tunggu persetujuan tertulis, lalu (hari yang sama)"
  echo "  DM01_APPROVED=yes $0 execute"
}

check_payroll_drafts() {
  local drafts
  drafts=$(payroll_draft_sql | psql_db "$DB_NAME" -qAt -f -) || die "cek payroll draft gagal"
  if [ -n "$drafts" ]; then
    echo "$drafts" | awk -F'\t' '{printf "  %s (%s, %s)\n", $2, $3, $4}' >&2
    die "ada payroll run berstatus draft yang periodenya mencakup ≤ 20 Okt 2026 (di atas). Selesaikan atau hapus run itu dulu: hitung ulangnya akan membaca data yang dihapus."
  fi
}

cmd_execute() {
  require_approval
  resolve_env
  local approved_dir approved cur dir t

  approved_dir=$(latest_dir "$BACKUP_DIR"/dm01-"$(today)"-*-pratinjau*)
  approved="$approved_dir/pratinjau.txt"
  [ -n "$approved_dir" ] && [ -f "$approved" ] ||
    die "belum ada pratinjau hari ini (WIB) di $BACKUP_DIR. Jalankan '$0 preview', minta persetujuan HR atas angkanya, lalu execute di hari yang sama."
  info "Pratinjau yang disetujui: $approved"
  same_target "$approved_dir"

  check_payroll_drafts

  # Angka saat ini harus sama dengan yang disetujui.
  cur=$(mktemp "$BACKUP_DIR/.dm01-pratinjau-XXXXXX")
  CLEANUP_FILES+=("$cur" "$cur.tmp")
  run_preview "$DB_NAME" "$cur"
  if ! diff -u <(core_lines "$approved") <(core_lines "$cur") >"$cur.diff"; then
    head -n 60 "$cur.diff" >&2
    CLEANUP_FILES+=("$cur.diff")
    die "data atau struktur berubah sejak pratinjau yang disetujui ($approved). Jalankan preview lagi dan minta persetujuan HR atas angka baru."
  fi
  rm -f -- "$cur.diff"

  dir=$(new_dir eksekusi)
  printf '%s\n' "$TARGET" >"$dir/target.txt"
  info "Folder eksekusi: $dir"
  cp -- "$approved" "$dir/pratinjau-disetujui.txt"
  mv -- "$cur" "$dir/pratinjau-sebelum.txt"
  lines_of FILE "$dir/pratinjau-sebelum.txt" | cut -f2- >"$dir/files.txt"

  # 1. Backup penuh.
  info "Ruang kosong folder backup: $(df -h --output=avail "$BACKUP_DIR" | tail -n1 | tr -d ' ')"
  info "pg_dump -Fc → $dir/db.dump"
  docker exec "$DB_CONTAINER" pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc >"$dir/db.dump" || die "pg_dump gagal"
  [ -s "$dir/db.dump" ] || die "db.dump kosong"
  docker exec -i "$DB_CONTAINER" pg_restore -l <"$dir/db.dump" >"$dir/db.toc" || die "pg_restore -l gagal: dump tidak terbaca"
  for t in "${TABLES[@]}" leave_balances; do
    grep -qE "TABLE DATA hris $t " "$dir/db.toc" || die "db.dump tidak memuat data hris.$t"
  done
  sha256sum "$dir/db.dump" >"$dir/db.dump.sha256"

  # 2. Uji restore ke DB sementara; dibandingkan dengan pratinjau sebelum eksekusi.
  info "Uji restore ke DB sementara $CHECK_DB"
  DROP_CHECK=1
  docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" --if-exists "$CHECK_DB" >/dev/null 2>&1 ||
    die "DB sisa $CHECK_DB tidak bisa dihapus (masih dipakai?)"
  docker exec "$DB_CONTAINER" createdb -U "$DB_USER" -T template0 "$CHECK_DB" || die "createdb $CHECK_DB gagal"
  docker exec -i "$DB_CONTAINER" pg_restore -U "$DB_USER" -d "$CHECK_DB" --no-owner --no-acl --exit-on-error \
    <"$dir/db.dump" >"$dir/restore.log" 2>&1 || die "restore db.dump ke $CHECK_DB gagal (lihat $dir/restore.log)"
  run_preview "$CHECK_DB" "$dir/pratinjau-restore.txt"
  docker exec "$DB_CONTAINER" dropdb -U "$DB_USER" "$CHECK_DB" || die "dropdb $CHECK_DB gagal"
  DROP_CHECK=0
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
  info "Uji restore cocok; DB sementara dihapus"

  # 3. Hapus data (satu transaksi).
  info "Menjalankan $(basename "$CORE_SQL") (psql -1)"
  if ! psql_db "$DB_NAME" -1 -f - <"$CORE_SQL" 2>&1 | tee "$dir/core.log"; then
    die "core SQL gagal; transaksi dibatalkan sehingga tidak ada yang terhapus (lihat $dir/core.log)"
  fi

  # 4. Cek pasca-eksekusi.
  run_preview "$DB_NAME" "$dir/pratinjau-sesudah.txt"
  local gagal=0 audit
  if lines_of HAPUS "$dir/pratinjau-sesudah.txt" | awk -F'\t' '$3 != 0 {bad=1} END {exit !bad}'; then
    echo "PERINGATAN: masih ada baris yang memenuhi kriteria hapus:" >&2
    lines_of HAPUS "$dir/pratinjau-sesudah.txt" >&2
    gagal=1
  fi
  if [ -n "$(lines_of 'FILE(_LAIN)?' "$dir/pratinjau-sesudah.txt")" ]; then
    echo "PERINGATAN: pratinjau sesudah eksekusi masih memuat berkas" >&2
    gagal=1
  fi
  if [ "$(lines_of CHECKSUM "$dir/pratinjau-sebelum.txt")" != "$(lines_of CHECKSUM "$dir/pratinjau-sesudah.txt")" ]; then
    echo "PERINGATAN: checksum leave_balances berubah (sebelum vs sesudah)" >&2
    gagal=1
  fi
  audit=$(psql_db "$DB_NAME" -qAt -c "SELECT format(E'HAPUS\t%s\t%s', u.k, a.after->>u.k)
      FROM (SELECT after FROM audit.audit_log WHERE action = 'data_cleanup.dm01' ORDER BY created_at DESC, id DESC LIMIT 1) a,
           unnest(ARRAY['attendance_requests','attendance','leaves','overtime_requests','business_trips','loans'])
             WITH ORDINALITY AS u(k, i)
      ORDER BY u.i")
  printf '%s\n' "$audit" >"$dir/audit-hapus.txt"
  if [ "$audit" != "$(lines_of HAPUS "$dir/pratinjau-sebelum.txt")" ]; then
    echo "PERINGATAN: jumlah di audit data_cleanup.dm01 berbeda dari pratinjau:" >&2
    diff <(lines_of HAPUS "$dir/pratinjau-sebelum.txt") "$dir/audit-hapus.txt" >&2 || true
    gagal=1
  fi
  if [ "$gagal" = 1 ]; then
    die "data SUDAH dihapus (COMMIT) tetapi cek pasca-eksekusi tidak lolos. Jangan jalankan 'files' sebelum diperiksa; bila sudah dipastikan aman: touch $dir/execute.ok"
  fi
  {
    echo "eksekusi $(stamp) WIB oleh $(id -un)@$(hostname) — $DB_CONTAINER/$DB_NAME"
    cat "$dir/audit-hapus.txt"
  } >"$dir/execute.ok"

  echo "Terhapus (audit data_cleanup.dm01):"
  awk -F'\t' '{printf "  %-22s %s\n", $2, $3}' "$dir/audit-hapus.txt"
  info "Eksekusi selesai dan cek pasca-eksekusi lolos. Backup: $dir/db.dump"
  echo "Langkah berikutnya: DM01_APPROVED=yes $0 files"
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
  elif [ -e "$f" ]; then s=LEWATI
  else s=HILANG; fi
  printf "%s\t%s\n" "$s" "$p"
done'

tool() { # <ro|rw> <skrip sh>; stdin diteruskan
  docker run --rm -i --network none --entrypoint sh -v "$VOLUME:/s:$1" "$TOOL_IMAGE" -c "$2"
}

cmd_files() {
  require_approval
  resolve_env
  local dir list n bad refs st status ada tgz hasil
  dir=${DM01_RUN_DIR:-$(latest_dir "$BACKUP_DIR"/dm01-*-eksekusi*)}
  [ -n "$dir" ] && [ -d "$dir" ] || die "belum ada folder eksekusi di $BACKUP_DIR; jalankan eksekusi (execute) dulu"
  info "Folder eksekusi: $dir"
  same_target "$dir"
  [ -f "$dir/execute.ok" ] ||
    die "eksekusi di $dir belum berhasil (execute.ok tidak ada); berkas hanya dihapus setelah datanya terhapus"
  [ ! -f "$dir/files.ok" ] || die "penghapusan berkas untuk $dir sudah selesai (files.ok)"
  list="$dir/files.txt"
  [ -f "$list" ] || die "daftar berkas tidak ada: $list"
  n=$(grep -c . "$list" || true)
  bad=$(grep -vE "$FILE_RE" "$list" | grep -c . || true)
  [ "$bad" = 0 ] || die "$bad path di $list tidak sesuai pola lampiran DM-01; berhenti tanpa menghapus apa pun"
  if grep -qE '/(\.|\.\.)$' "$list"; then die "path '.'/'..' di $list; berhenti"; fi
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
  [ "$(grep -c . "$status")" = "$(sort -u "$list" | grep -c .)" ] || die "pemeriksaan berkas tidak lengkap"
  awk -F'\t' '$1 == "ADA" {print $2}' "$status" >"$ada"

  local n_ada n_hilang n_lewati n_hapus=0 n_gagal=0
  n_ada=$(grep -c . "$ada" || true)
  n_hilang=$(grep -c $'^HILANG\t' "$status" || true)
  n_lewati=$(grep -c $'^LEWATI\t' "$status" || true)

  if [ "$n_ada" -gt 0 ]; then
    info "Mengarsipkan $n_ada berkas → $tgz"
    tool ro 'cat >/tmp/daftar && tar -czf - -C /s/private -T /tmp/daftar' <"$ada" >"$tgz" || die "tar berkas gagal"
    sha256sum "$tgz" >"$tgz.sha256"
    if ! cmp -s <(sort "$ada") <(tool ro 'tar -tzf -' <"$tgz" | sort); then
      die "isi $tgz tidak sama dengan daftar berkas; berhenti tanpa menghapus apa pun"
    fi
    info "Isi arsip cocok dengan daftar ($n_ada path)"

    tool rw '
while IFS= read -r p; do
  f="/s/private/$p"
  if [ -f "$f" ] && [ ! -L "$f" ]; then
    if rm -f -- "$f" && [ ! -e "$f" ]; then printf "DIHAPUS\t%s\n" "$p"; else printf "GAGAL\t%s\n" "$p"; fi
  else printf "HILANG\t%s\n" "$p"; fi
done' <"$ada" >"$hasil" || die "penghapusan berkas terhenti (lihat $hasil)"
    n_hapus=$(grep -c $'^DIHAPUS\t' "$hasil" || true)
    n_gagal=$(grep -c $'^GAGAL\t' "$hasil" || true)
    n_hilang=$((n_hilang + $(grep -c $'^HILANG\t' "$hasil" || true)))
  fi

  echo "Ringkasan berkas:"
  printf '  %-20s: %s\n' "daftar FILE" "$n" "diarsipkan (tar)" "$n_ada" "dihapus" "$n_hapus" \
    "sudah tidak ada" "$n_hilang" "dilewati (symlink)" "$n_lewati" "gagal" "$n_gagal"
  [ "$n_lewati" = 0 ] || info "Berkas dilewati (tinjau manual): lihat baris LEWATI di $status"
  [ "$n_gagal" = 0 ] || die "$n_gagal berkas gagal dihapus (lihat $hasil); jalankan files lagi setelah diperbaiki"
  echo "files $st: dihapus $n_hapus, sudah tidak ada $n_hilang, dilewati $n_lewati — arsip $tgz" >"$dir/files.ok"
  info "Selesai. Salin $dir ke lokal; tinjau FILE_LAIN di pratinjau secara manual."
}

case "$MODE" in
  preview) cmd_preview ;;
  execute) cmd_execute ;;
  files) cmd_files ;;
  *) usage ;;
esac

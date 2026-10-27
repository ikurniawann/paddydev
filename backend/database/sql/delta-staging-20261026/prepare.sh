#!/usr/bin/env bash
# Siapkan delta staging → production dari dump staging BARU (tanpa mengubah staging/production):
# dump staging + storage, dump production (read-only), hitung delta dari baseline cutover,
# arsip file, gladi di salinan lokal (termasuk jalur gagal) dan di VPS, lalu tulis laporan.
# Hasil di ~/Desktop/paddydev-backups/delta-<STAMP>.{sql,files.txt,files.tgz,md}. Eksekusi terpisah:
#   tools/vps/apply-delta.sh execute <delta.sql> <files.tgz>   (hanya atas izin user)
set -Eeuo pipefail
cd "$(dirname "$0")"
REPO="$(git rev-parse --show-toplevel)"
B="$HOME/Desktop/paddydev-backups"
T="${TEST_DB_URL:-postgres://postgres@127.0.0.1:55499/}"
BASE_DUMP="$B/paddydev-db-cutover-execute-20261024-233012.dump"
STAMP="$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)"
OUT="$B/delta-$STAMP"
umask 077
log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
restore() { # restore <db> <dump>
  psql "${T}postgres" -Xq -c "DROP DATABASE IF EXISTS $1" -c "CREATE DATABASE $1"
  docker run --rm -i --network host postgres:16-alpine pg_restore --no-owner --no-acl --exit-on-error -d "$T$1" < "$2"
}
test -s "$BASE_DUMP"

log "Dump staging (DB + storage) dan production ($STAMP)"
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$B/staging-db-$STAMP.dump"
docker exec -i paddydev-db pg_restore -l < "$B/staging-db-$STAMP.dump" > /dev/null
docker run --rm -v paddydev-storage:/s:ro alpine tar -C /s -czf - private > "$B/staging-storage-$STAMP.tgz"
ssh -o BatchMode=yes paddy-prod "docker exec paddydev-db sh -c 'pg_dump -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -Fc'" > "$B/prod-db-$STAMP.dump"
docker exec -i paddydev-db pg_restore -l < "$B/prod-db-$STAMP.dump" > /dev/null

log "Pulihkan baseline, staging, production ke DB test"
restore delta_base "$BASE_DUMP"
restore delta_stg "$B/staging-db-$STAMP.dump"
restore delta_prod "$B/prod-db-$STAMP.dump"

log "Hitung delta"
python3 -I generate.py --db "$T" --base delta_base --stg delta_stg --prod delta_prod \
  --out "$OUT.sql" --files "$OUT.files.txt" --report "$OUT.md" \
  --stg-label "dump staging $STAMP (${STAMP:9:2}.${STAMP:11:2} WIB)"

log "Arsip file foto/lampiran"
W="$(mktemp -d)"
cp "$OUT.files.txt" "$W.list"  # path sudah berawalan private/
tar -tzf "$B/staging-storage-$STAMP.tgz" | LC_ALL=C sort -u > "$W.all"
missing="$(LC_ALL=C sort -u "$W.list" | LC_ALL=C comm -23 - "$W.all" | wc -l)"
if [ "$missing" != 0 ]; then echo "$missing file tidak ada di arsip staging" >&2; exit 1; fi
if [ -s "$W.list" ]; then tar -xzf "$B/staging-storage-$STAMP.tgz" -C "$W" -T "$W.list"; else mkdir -p "$W/private"; fi
tar -czf "$OUT.files.tgz" -C "$W" private
rm -rf "$W" "$W.list" "$W.all"

log "Gladi lokal di salinan production"
psql "${T}postgres" -Xq -c 'DROP DATABASE IF EXISTS delta_rehearsal' -c 'CREATE DATABASE delta_rehearsal TEMPLATE delta_prod'
local_ok=$(psql "${T}delta_rehearsal" -X -v ON_ERROR_STOP=1 -q -f "$OUT.sql" 2>&1 | grep -oE 'delta staging OK.*' || true)
test -n "$local_ok" || { echo "Gladi lokal GAGAL" >&2; psql "${T}delta_rehearsal" -X -v ON_ERROR_STOP=1 -q -f "$OUT.sql" >&2 || true; exit 1; }
rerun=$(psql "${T}delta_rehearsal" -X -v ON_ERROR_STOP=1 -q -f "$OUT.sql" 2>&1 | grep -oE 'ERROR: .*' | head -1 || true)
# Delta hanya menulis data: skema salinan production sebelum dan sesudah delta harus identik.
# (migrate -verify tidak dipakai: image lokal bisa lebih baru dari production dan selalu "drift".)
schema() { docker run --rm --network host postgres:16-alpine pg_dump --schema-only --no-owner --no-acl "$T$1" | grep -vE '^(--|\\(un)?restrict)' | md5sum | cut -d' ' -f1; }
if [ "$(schema delta_prod)" = "$(schema delta_rehearsal)" ]; then verify="identik dengan salinan production sebelum delta"; else verify="BERBEDA dari salinan production sebelum delta"; fi

log "Gladi di VPS"
vps=$(bash "$REPO/tools/vps/apply-delta.sh" rehearse "$OUT.sql" "$OUT.files.tgz" 2>&1 | grep -E 'delta staging OK|ERROR|file|Gladi' || true)

{
  echo
  echo "## Hasil gladi"
  echo
  echo "- Lokal (salinan production $STAMP): $local_ok"
  echo "- Dijalankan ulang (harus gagal): ${rerun:-TIDAK GAGAL}"
  echo "- Skema setelah delta: $verify"
  echo "- VPS:"
  echo "$vps" | sed 's/^/  - /'
  echo
  echo "File: $OUT.sql, $OUT.files.tgz (chmod 600). Eksekusi: tools/vps/apply-delta.sh execute $OUT.sql $OUT.files.tgz"
} >> "$OUT.md"
log "Selesai. Laporan: $OUT.md"

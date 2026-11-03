#!/usr/bin/env bash
# Runner pengisian massal data kerja (EPIC-086): menjalankan
# backend/cmd/hris-data-kerja di dalam container paddydev-api (DB URL dari env
# container, tidak keluar dari server).
#
#   ENV=staging|prod tools/ops/data-kerja.sh audit
#   ENV=staging|prod tools/ops/data-kerja.sh template
#   ENV=staging|prod tools/ops/data-kerja.sh preview FILE.xlsx
#   DATA_KERJA_APPROVED=yes ENV=prod tools/ops/data-kerja.sh apply FILE.xlsx SHA256
#
# staging = Docker lokal; prod = ssh paddy-prod. Hasil (laporan, template,
# backup staging) ke $DATA_KERJA_OUT (bawaan ~/Desktop/paddydev-backups/data-kerja).
# apply: wajib DATA_KERJA_APPROVED=yes (izin user + HR) dan sha dari pratinjau;
# pg_dump dulu (umask 077) — prod ke ~/backups/data-kerja-<stamp>.dump di VPS.
set -euo pipefail

die() { echo "$2" >&2; exit "$1"; }
case "${ENV:-}" in staging|prod) ;; *) die 2 "ENV harus staging atau prod" ;; esac
MODE=${1:-}
FILE=""; SHA=""
case "$MODE" in
  audit|template) ;;
  preview|apply)
    FILE=${2:-}
    [ -n "$FILE" ] && [ -f "$FILE" ] || die 2 "file tidak ada: ${FILE:-(kosong)}"
    if [ "$MODE" = apply ]; then
      [ "${DATA_KERJA_APPROVED:-}" = yes ] || die 3 "apply butuh DATA_KERJA_APPROVED=yes (izin tertulis user dan HR)"
      SHA=${3:-}
      [ -n "$SHA" ] || die 2 "sha wajib untuk apply (salin dari laporan pratinjau)"
    fi ;;
  *) die 2 "mode harus audit, template, preview, atau apply" ;;
esac

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
STAMP=$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)
OUT=${DATA_KERJA_OUT:-$HOME/Desktop/paddydev-backups/data-kerja}/$ENV-$STAMP
mkdir -p "$OUT"
BIN="$OUT/hris-data-kerja"
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o "$BIN" ./cmd/hris-data-kerja)

# on runs a shell command on the host that owns the containers.
on() { if [ "$ENV" = prod ]; then ssh paddy-prod "$1"; else bash -c "$1"; fi; }
# put copies a local file to the host's work dir, prints the host path.
HOST_DIR=/tmp/data-kerja-$STAMP
on "mkdir -p $HOST_DIR"
put() {
  if [ "$ENV" = prod ]; then scp -q "$1" "paddy-prod:$HOST_DIR/$(basename "$1")"; else cp "$1" "$HOST_DIR/"; fi
  echo "$HOST_DIR/$(basename "$1")"
}
get() { if [ "$ENV" = prod ]; then scp -q "paddy-prod:$1" "$2"; else cp "$1" "$2"; fi; }

on "docker cp $(put "$BIN") paddydev-api:/tmp/hris-data-kerja"
ARGS="-mode $MODE"
if [ -n "$FILE" ]; then
  cp "$FILE" "$OUT/input.xlsx"
  on "docker cp $(put "$OUT/input.xlsx") paddydev-api:/tmp/data-kerja-input.xlsx"
  ARGS="$ARGS -file /tmp/data-kerja-input.xlsx"
fi
[ "$MODE" = template ] && ARGS="$ARGS -out /tmp/data-kerja-template.xlsx"
[ -n "$SHA" ] && ARGS="$ARGS -sha $SHA"

if [ "$MODE" = apply ]; then
  if [ "$ENV" = prod ]; then DUMP="~/backups/data-kerja-$STAMP.dump"; else DUMP="$OUT/db.dump"; fi
  on "set -e; umask 077; docker exec paddydev-db sh -c 'pg_dump -Fc -U \"\$POSTGRES_USER\" \"\$POSTGRES_DB\"' > $DUMP; ls -la $DUMP"
fi

set +e
on "docker exec paddydev-api /tmp/hris-data-kerja $ARGS" | tee "$OUT/$MODE.md"
CODE=${PIPESTATUS[0]}
set -e
if [ "$MODE" = template ] && [ "$CODE" = 0 ]; then
  on "docker cp paddydev-api:/tmp/data-kerja-template.xlsx $HOST_DIR/template.xlsx"
  get "$HOST_DIR/template.xlsx" "$OUT/Template-Data-Kerja-$ENV-$STAMP.xlsx"
  echo "Template: $OUT/Template-Data-Kerja-$ENV-$STAMP.xlsx"
fi
on "docker exec paddydev-api rm -f /tmp/hris-data-kerja /tmp/data-kerja-input.xlsx /tmp/data-kerja-template.xlsx; rm -rf $HOST_DIR" || true
echo "Laporan: $OUT/$MODE.md"
exit "$CODE"

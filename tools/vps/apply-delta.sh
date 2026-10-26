#!/usr/bin/env bash
# Terapkan delta SQL + arsip file storage ke production paddyadorable.biz.id (ssh paddy-prod).
#   rehearse <delta.sql> <files.tgz> : salin DB production ke delta_rehearsal di VPS, jalankan delta,
#                                      hapus DB itu. Production tidak berubah.
#   execute  <delta.sql> <files.tgz> : backup DB production, jalankan delta ke DB production, salin
#                                      file tanpa menimpa yang sudah ada, lalu cek semua file ada.
# File delta dibuat oleh backend/database/sql/delta-staging-20261026/generate.py (berisi data
# pribadi: jangan dicetak atau di-commit).
set -Eeuo pipefail
MODE="${1:?pakai: rehearse|execute <delta.sql> <files.tgz>}"
SQL="${2:?file delta.sql}"
TGZ="${3:?file files.tgz}"
R=paddy-prod
STAMP="$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)"
log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

case "$MODE" in rehearse|execute) ;; *) echo "mode tidak dikenal: $MODE" >&2; exit 2 ;; esac
test -s "$SQL" && test -s "$TGZ"

log "Kirim delta ke VPS ($MODE, $STAMP)"
ssh -o BatchMode=yes "$R" 'umask 077; mkdir -p ~/delta'
scp -q "$SQL" "$R:delta/delta.sql"
scp -q "$TGZ" "$R:delta/files.tgz"

ssh "$R" bash -s -- "$MODE" "$STAMP" <<'REMOTE'
set -Eeuo pipefail
MODE="$1"; STAMP="$2"
pg() { docker exec -i paddydev-db sh -c 'psql -U "$POSTGRES_USER" -X -v ON_ERROR_STOP=1 -q "$@"' _ "$@"; }
PROD_DB="$(docker exec paddydev-db sh -c 'printf %s "$POSTGRES_DB"')"
tar -tzf ~/delta/files.tgz | grep -v '/$' > ~/delta/files.list
echo "file di arsip: $(wc -l < ~/delta/files.list)"

if [ "$MODE" = rehearse ]; then
  pg -d postgres -c 'DROP DATABASE IF EXISTS delta_rehearsal WITH (FORCE)' -c 'CREATE DATABASE delta_rehearsal'
  docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
    | docker exec -i paddydev-db sh -c 'pg_restore -U "$POSTGRES_USER" -d delta_rehearsal --no-owner --no-acl --exit-on-error'
  status=0
  pg -d delta_rehearsal < ~/delta/delta.sql || status=$?
  pg -d postgres -c 'DROP DATABASE IF EXISTS delta_rehearsal WITH (FORCE)'
  existing=$(docker run --rm -i -v paddydev-storage:/s:ro alpine sh -c 'n=0; while read -r f; do [ -e "/s/$f" ] && n=$((n+1)); done; echo $n' < ~/delta/files.list)
  echo "file yang sudah ada di production (tidak akan ditimpa): $existing"
  exit "$status"
fi

umask 077; mkdir -p ~/backups
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > ~/backups/prod-db-before-delta-"$STAMP".dump
docker exec -i paddydev-db pg_restore -l < ~/backups/prod-db-before-delta-"$STAMP".dump > /dev/null
ls -la ~/backups/prod-db-before-delta-"$STAMP".dump
pg -d "$PROD_DB" < ~/delta/delta.sql
docker run --rm -i -v paddydev-storage:/s alpine sh -c \
  'mkdir /tmp/x && tar -C /tmp/x -xzf - && cp -rn /tmp/x/private/. /s/private/ && chown -R 1001:1001 /s/private' < ~/delta/files.tgz
docker run --rm -i -v paddydev-storage:/s:ro alpine sh -c \
  'm=0; while read -r f; do [ -f "/s/$f" ] || { echo "HILANG $f"; m=$((m+1)); }; done; echo "file hilang: $m"; [ "$m" = 0 ]' < ~/delta/files.list
REMOTE

if [ "$MODE" = execute ]; then
  curl -fsS --max-time 15 https://paddyadorable.biz.id/api/ready; echo
  log "Delta diterapkan ($STAMP). Backup sebelum delta: ~/backups/prod-db-before-delta-$STAMP.dump di VPS."
else
  log "Gladi selesai. Production tidak berubah."
fi

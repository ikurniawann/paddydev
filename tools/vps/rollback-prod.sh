#!/usr/bin/env bash
# Pulihkan production ke kondisi sebelum cutover: tools/vps/rollback-prod.sh <STAMP>
set -Eeuo pipefail
STAMP="${1:?pakai: rollback-prod.sh <STAMP dari log cutover>}"
ssh paddy-prod bash -s -- "$STAMP" <<'REMOTE'
set -Eeuo pipefail
STAMP="$1"; B="$HOME/backups"
test -s "$B/prod-db-before-cutover-$STAMP.dump"
test -s "$B/prod-storage-before-cutover-$STAMP.tgz"
docker stop paddydev paddydev-api || true
DB="$(docker exec paddydev-db sh -c 'printf %s "$POSTGRES_DB"')"
docker exec -i paddydev-db sh -c 'psql -U "$POSTGRES_USER" -d postgres -X -v ON_ERROR_STOP=1 -q -c "DROP DATABASE IF EXISTS \"$1\" WITH (FORCE)" -c "CREATE DATABASE \"$1\""' _ "$DB"
docker exec -i paddydev-db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$1" --no-owner --no-acl --exit-on-error' _ "$DB" < "$B/prod-db-before-cutover-$STAMP.dump"
docker run --rm -i -v paddydev-storage:/s alpine sh -c 'rm -rf /s/* && tar -C /s -xzf -' < "$B/prod-storage-before-cutover-$STAMP.tgz"
cp "$B/api.env.before-cutover-$STAMP" "$HOME/.config/paddydev/api.env"
docker start paddydev-api paddydev
REMOTE
docker start paddydev-api paddydev 2>/dev/null || true
echo "Production dipulihkan ke kondisi sebelum cutover $STAMP; live paddydev dinyalakan kembali."

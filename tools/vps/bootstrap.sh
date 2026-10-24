#!/usr/bin/env bash
# Menyiapkan VPS Paddy sekali jalan (idempoten): jaringan, volume, Postgres, Caddy.
# Env (web.env, api.env, db.env) diletakkan manual di $PADDYDEV_ENV_DIR dan tidak
# pernah masuk repo atau GitHub. Aplikasi sendiri di-deploy oleh tools/deploy-paddydev.sh.
set -Eeuo pipefail
cd "$(dirname "$0")"

ENV_DIR="${PADDYDEV_ENV_DIR:-$HOME/.config/paddydev}"
test -r "$ENV_DIR/db.env" || { echo "File tidak terbaca: $ENV_DIR/db.env" >&2; exit 1; }

docker network inspect paddydev-net >/dev/null 2>&1 || docker network create paddydev-net >/dev/null
docker volume inspect paddydev-storage >/dev/null 2>&1 || docker volume create paddydev-storage >/dev/null
docker volume inspect paddydev-db-data >/dev/null 2>&1 || docker volume create paddydev-db-data >/dev/null
docker volume inspect caddy-data >/dev/null 2>&1 || docker volume create caddy-data >/dev/null

# Volume storage dimiliki uid 1001 (web dan API menulis ke sana).
docker run --rm -v paddydev-storage:/s alpine chown 1001:1001 /s

if ! docker inspect paddydev-db >/dev/null 2>&1; then
  docker run -d --name paddydev-db --restart unless-stopped --network paddydev-net \
    --env-file "$ENV_DIR/db.env" -v paddydev-db-data:/var/lib/postgresql/data \
    -p 127.0.0.1:5485:5432 --log-opt max-size=20m --log-opt max-file=3 postgres:16-alpine
fi

docker rm -f paddydev-caddy >/dev/null 2>&1 || true
docker run -d --name paddydev-caddy --restart unless-stopped --network paddydev-net \
  -p 80:80 -p 443:443 -v "$PWD/Caddyfile:/etc/caddy/Caddyfile:ro" -v caddy-data:/data \
  --log-opt max-size=20m --log-opt max-file=3 caddy:2-alpine

echo 'Bootstrap selesai.'

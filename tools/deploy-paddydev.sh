#!/usr/bin/env bash
# Deploy the local checkout (Docker on this host). Default URL paddydev.reddie.id; override with PADDYDEV_URL.
# First run creates the containers; later runs replace them and roll back on failure.
set -Eeuo pipefail
cd "$(dirname "$0")/.."

ENV_DIR="${PADDYDEV_ENV_DIR:-$HOME/.config/paddydev}"
TAG="local-$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)"
WEB_IMAGE="paddydev:$TAG"
API_IMAGE="paddydev-api:$TAG"
WEB_PORT="${PADDYDEV_PORT:-8145}"

for file in web.env api.env; do
  test -r "$ENV_DIR/$file" || { echo "File tidak terbaca: $ENV_DIR/$file" >&2; exit 1; }
done
exec 9>"/tmp/paddydev-deploy-${UID}.lock"
flock -n 9 || { echo 'Deploy Paddy lain sedang berjalan.' >&2; exit 1; }
docker inspect paddydev-db >/dev/null
docker network inspect paddydev-net >/dev/null
docker volume inspect paddydev-storage >/dev/null
OLD_WEB="$(docker inspect --format '{{.Image}}' paddydev 2>/dev/null || true)"
OLD_API="$(docker inspect --format '{{.Image}}' paddydev-api 2>/dev/null || true)"

echo "Membangun web dan API: $TAG"
DOCKER_BUILDKIT=1 docker build --secret "id=dotenv,src=$ENV_DIR/web.env" -t "$WEB_IMAGE" .
DOCKER_BUILDKIT=1 docker build -t "$API_IMAGE" backend

# Migrasi memberi GRANT ke role ini; cluster baru belum memilikinya.
docker exec paddydev-db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q -c "
  DO \$\$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '\''authenticated'\'') THEN CREATE ROLE authenticated NOLOGIN; END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '\''service_role'\'') THEN CREATE ROLE service_role NOLOGIN; END IF;
  END \$\$;"'

echo 'Menerapkan migrasi sebelum mengganti container...'
docker run --rm --network paddydev-net --env-file "$ENV_DIR/api.env" \
  --entrypoint /usr/local/bin/migrate "$API_IMAGE" -allow-remote -apply
verify_status=0
docker run --rm --network paddydev-net --env-file "$ENV_DIR/api.env" \
  --entrypoint /usr/local/bin/migrate "$API_IMAGE" -allow-remote -verify || verify_status=$?
if [ "$verify_status" = 3 ]; then
  echo 'Skema berbeda dari migrasi. Deploy dihentikan; container lama tetap berjalan.' >&2
  exit 1
elif [ "$verify_status" != 0 ]; then
  echo "Peringatan: pemeriksaan skema tidak dapat dijalankan (exit $verify_status)." >&2
fi

# API memakai uid web (nextjs, 1001): keduanya menulis ke volume paddydev-storage
# yang dimiliki 1001. Dengan uid bawaan image (65532) upload CV, rekaman
# interview, dan memori assistant gagal "permission denied".
start_api() {
  docker run -d --name paddydev-api --restart unless-stopped --stop-timeout 30 --user 1001:1001 \
    --network paddydev-net --env-file "$ENV_DIR/api.env" -v paddydev-storage:/app/storage \
    --log-opt max-size=50m --log-opt max-file=5 "$1"
}
start_web() {
  docker run -d --name paddydev --restart unless-stopped --network paddydev-net \
    --env-file "$ENV_DIR/web.env" -v paddydev-storage:/app/storage \
    -p "127.0.0.1:$WEB_PORT:3000" --log-opt max-size=50m --log-opt max-file=5 "$1"
}
rollback() {
  trap - ERR INT TERM
  set +e
  echo 'Deploy gagal. Menjalankan kembali image sebelumnya; migrasi tidak dibatalkan.' >&2
  docker stop paddydev paddydev-api >/dev/null 2>&1
  docker rm paddydev paddydev-api >/dev/null 2>&1
  if [ -n "$OLD_API" ] && [ -n "$OLD_WEB" ]; then
    start_api "$OLD_API"
    start_web "$OLD_WEB"
  fi
  exit 1
}

if [ -n "$OLD_WEB" ] && [ -n "$OLD_API" ]; then
  docker tag "$OLD_WEB" paddydev:previous
  docker tag "$OLD_API" paddydev-api:previous
fi
trap rollback ERR INT TERM
docker rm -f paddydev paddydev-api >/dev/null 2>&1 || true
start_api "$API_IMAGE"
start_web "$WEB_IMAGE"

echo 'Menunggu web, database, dan API...'
ready=0
for attempt in $(seq 1 60); do
  if docker exec paddydev-api /usr/local/bin/api -healthcheck >/dev/null 2>&1 && \
    curl -fsS --max-time 10 "http://127.0.0.1:$WEB_PORT/api/ready" >/dev/null && \
    curl -fsS --max-time 10 "http://127.0.0.1:$WEB_PORT/login" >/dev/null; then
    ready=1
    break
  fi
  sleep 2
done
test "$ready" = 1
trap - ERR INT TERM
echo "Deploy selesai: ${PADDYDEV_URL:-https://paddydev.reddie.id} ($TAG)"

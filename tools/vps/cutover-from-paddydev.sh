#!/usr/bin/env bash
# Cutover production paddyadorable.biz.id dari live paddydev (host ini).
#   rehearse : restore ke DB sementara cutover_rehearsal di VPS + SQL cutover + cek skema,
#              lalu hapus DB itu. Live & production tidak berubah.
#   execute  : cek tulis-baru production, backup production, bekukan live, restore + SQL
#              cutover ke DB production, salin storage HRIS, rapikan env, deploy main,
#              lalu nyalakan paddydev sebagai staging (banner).
# Jalankan dari checkout main yang bersih. Wajib SSH alias paddy-prod.
set -Eeuo pipefail
cd "$(dirname "$0")/../.."
MODE="${1:?pakai: rehearse | execute}"
STAMP="$(TZ=Asia/Jakarta date +%Y%m%d-%H%M%S)"
BK="$HOME/Desktop/paddydev-backups"
SQL_DIR=backend/database/sql/cutover-prod
R=paddy-prod
STORAGE_DIRS=(private/attendance private/attendance-requests private/leave-attachments private/overtime-attachments)
log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
remote() { ssh "$R" bash cutover/cutover-remote.sh "$@"; }

log "Preflight ($MODE, $STAMP)"
test -z "$(git status --porcelain --untracked-files=no)" || { echo 'Checkout tidak bersih.' >&2; exit 1; }
git fetch -q paddydev main
test "$(git rev-parse HEAD)" = "$(git rev-parse paddydev/main)" || { echo 'HEAD bukan paddydev/main terbaru.' >&2; exit 1; }
ssh -o BatchMode=yes "$R" 'umask 077; mkdir -p ~/cutover; df -h / | tail -1'
df -h / | tail -1
docker inspect paddydev paddydev-api paddydev-db > /dev/null
scp -q tools/vps/cutover-remote.sh "$SQL_DIR/01-hris-only.sql" "$SQL_DIR/02-demo-dan-sesi.sql" "$R:cutover/"

if [ "$MODE" = execute ]; then
  : "${PROD_WRITE_CUTOFF:?isi PROD_WRITE_CUTOFF, mis. 2026-10-22T08:30:00Z}"
  writes="$(remote writes "$PROD_WRITE_CUTOFF" | tr -d '[:space:]')"
  if [ "$writes" != 0 ]; then
    echo "Production punya $writes perubahan setelah $PROD_WRITE_CUTOFF. Cutover dihentikan; minta keputusan user." >&2
    exit 1
  fi
  log "Backup production"
  remote backup "$STAMP"
  log "Bekukan live paddydev"
  # Bila gagal setelah titik ini, live dinyalakan lagi (datanya tidak diubah) dan
  # production dipulihkan dengan rollback-prod.sh.
  on_fail() {
    echo "CUTOVER GAGAL. Live paddydev dinyalakan kembali. Pulihkan production: tools/vps/rollback-prod.sh $STAMP" >&2
    docker start paddydev-api paddydev > /dev/null 2>&1 || true
  }
  trap on_fail ERR
  docker stop paddydev paddydev-api
fi

log "Dump live + storage HRIS"
umask 077
DUMP="$BK/paddydev-db-cutover-$MODE-$STAMP.dump"
TGZ="$BK/paddydev-storage-hris-$STAMP.tgz"
docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$DUMP"
docker exec -i paddydev-db pg_restore -l < "$DUMP" > /dev/null
docker run --rm -v paddydev-storage:/s:ro alpine tar -C /s -czf - "${STORAGE_DIRS[@]}" > "$TGZ"

log "Kirim ke VPS"
scp -q "$DUMP" "$R:cutover/live.dump"
scp -q "$TGZ" "$R:cutover/storage-hris.tgz"

if [ "$MODE" = rehearse ]; then
  log "Restore + SQL cutover (cutover_rehearsal)"
  status=0
  remote restore cutover_rehearsal && remote check cutover_rehearsal || status=$?
  remote drop cutover_rehearsal
  [ "$status" = 0 ] || { echo "Gladi GAGAL (exit $status)." >&2; exit "$status"; }
  log "Gladi selesai. Live & production tidak berubah."
  exit 0
fi

log "Hentikan web/API production"
ssh "$R" 'docker stop paddydev paddydev-api'
log "Restore + SQL cutover (PROD)"
remote restore PROD
remote check PROD
log "Salin storage HRIS"
remote storage
log "Rapikan env production"
remote env

log "Deploy main ke production"
# Source baru di folder bersih (file yang dihapus dari repo tidak tertinggal). Folder lama
# disimpan sebagai paddydev-src.old: Caddyfile production di-bind-mount dari sana.
git archive --format=tar HEAD | ssh "$R" 'set -e; rm -rf paddydev-src.new; mkdir paddydev-src.new; tar -x -C paddydev-src.new
  rm -rf paddydev-src.old; mv paddydev-src paddydev-src.old; mv paddydev-src.new paddydev-src'
ssh "$R" 'cd paddydev-src && PADDYDEV_URL=https://paddyadorable.biz.id bash tools/deploy-paddydev.sh' \
  || ssh "$R" 'cd paddydev-src && PADDYDEV_URL=https://paddyadorable.biz.id bash tools/deploy-paddydev.sh'

log "Verifikasi production"
curl -fsS --max-time 15 https://paddyadorable.biz.id/api/ready; echo
test "$(curl -s -o /dev/null -w '%{http_code}' https://paddyadorable.biz.id/login)" = 200

log "Nyalakan paddydev sebagai staging (banner)"
grep -q '^STAGING_BANNER_PRODUCTION_URL=' "$HOME/.config/paddydev/web.env" \
  || printf 'STAGING_BANNER_PRODUCTION_URL=https://paddyadorable.biz.id\n' >> "$HOME/.config/paddydev/web.env"
docker start paddydev-api paddydev
bash tools/deploy-paddydev.sh
trap - ERR
log "Cutover selesai ($STAMP). Rollback: tools/vps/rollback-prod.sh $STAMP"

#!/usr/bin/env bash
# Helper di VPS untuk cutover production (dipanggil tools/vps/cutover-from-paddydev.sh).
# Skema tidak dicek di sini: image API production bisa lebih tua dari dump; deploy-paddydev.sh
# menjalankan migrate -verify dengan image baru (exit 3 menghentikan deploy).
# Input di ~/cutover: live.dump, storage-hris.tgz, 01-hris-only.sql, 02-demo-dan-sesi.sql.
set -Eeuo pipefail
CMD="${1:?perintah}"; ARG="${2:-}"
DIR="$HOME/cutover"; ENV_DIR="$HOME/.config/paddydev"
pg() { docker exec -i paddydev-db sh -c 'psql -U "$POSTGRES_USER" -X -v ON_ERROR_STOP=1 -q "$@"' _ "$@"; }
prod_db() { docker exec paddydev-db sh -c 'printf %s "$POSTGRES_DB"'; }
db_name() { if [ "$1" = PROD ]; then prod_db; else printf %s "$1"; fi; }

case "$CMD" in
  writes) # jumlah tulis-baru di production setelah ARG (ISO timestamp)
    pg -d "$(prod_db)" -At -v cutoff="$ARG" <<'SQL'
SET default_transaction_read_only = on;
SELECT (SELECT count(*) FROM audit.audit_log WHERE created_at > :'cutoff')
     + (SELECT count(*) FROM hris.attendance WHERE updated_at > :'cutoff')
     + (SELECT count(*) FROM hris.leaves WHERE created_at > :'cutoff')
     + (SELECT count(*) FROM hris.overtime_requests WHERE created_at > :'cutoff')
     + (SELECT count(*) FROM hris.employees WHERE updated_at > :'cutoff');
SQL
    ;;
  backup) # backup DB, storage, dan api.env production dengan STAMP = ARG
    umask 077; mkdir -p "$HOME/backups"
    docker exec paddydev-db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$HOME/backups/prod-db-before-cutover-$ARG.dump"
    docker exec -i paddydev-db pg_restore -l < "$HOME/backups/prod-db-before-cutover-$ARG.dump" > /dev/null
    docker run --rm -v paddydev-storage:/s:ro alpine tar -C /s -czf - . > "$HOME/backups/prod-storage-before-cutover-$ARG.tgz"
    cp "$ENV_DIR/api.env" "$HOME/backups/api.env.before-cutover-$ARG"
    ls -la "$HOME/backups" | tail -3
    ;;
  restore) # buat ulang DB (ARG = nama DB atau PROD), restore live.dump, jalankan SQL cutover
    DB="$(db_name "$ARG")"
    pg -d postgres -c "DROP DATABASE IF EXISTS \"$DB\" WITH (FORCE)" -c "CREATE DATABASE \"$DB\""
    docker exec -i paddydev-db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$1" --no-owner --no-acl --exit-on-error' _ "$DB" < "$DIR/live.dump"
    for f in 01-hris-only.sql 02-demo-dan-sesi.sql; do pg -d "$DB" < "$DIR/$f"; done
    ;;
  check) # ringkasan data DB ARG setelah SQL cutover (read-only)
    pg -d "$(db_name "$ARG")" -c "SET default_transaction_read_only = on" -c "SELECT
      (SELECT count(*) FROM public.schema_migrations) AS migrasi,
      (SELECT count(*) FROM hris.employees) AS karyawan,
      (SELECT count(*) FROM hris.employees WHERE nip = 'DEMOPADDY') AS demo,
      (SELECT count(*) FROM hris.attendance) AS absensi,
      (SELECT count(*) FROM auth.users) AS akun,
      (SELECT count(*) FROM auth.sessions) AS sesi,
      (SELECT count(*) FROM pos.pos_products) AS produk_pos"
    ;;
  drop)
    pg -d postgres -c "DROP DATABASE IF EXISTS \"$ARG\" WITH (FORCE)"
    ;;
  storage) # ganti isi storage production dengan folder HRIS dari live
    docker run --rm -i -v paddydev-storage:/s alpine sh -c \
      'rm -rf /s/private /s/uploads /s/assistant-memory && tar -C /s -xzf - && chown -R 1001:1001 /s' < "$DIR/storage-hris.tgz"
    ;;
  env) # hapus setting testing dari api.env; cetak nama key saja
    sed -i '/^XENDIT_MOCK=/d;/^MEMBER_OTP_DEMO_CODE=/d' "$ENV_DIR/api.env"
    sed -n 's/=.*//p' "$ENV_DIR/api.env" | tr '\n' ' '; echo
    ;;
  *) echo "perintah tidak dikenal: $CMD" >&2; exit 2 ;;
esac

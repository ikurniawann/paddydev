#!/usr/bin/env bash
# Sinkron dokumen paddydev → salinan lokal erp-paddy (keputusan user 12-11-2026).
#
# Sejak 12-11-2026 kode dikerjakan di wit-id-indonesia/erp-paddy (branch
# development), yang sengaja tidak menyimpan docs/ dan seeder di git
# (.gitignore: "dokumentasi internal tidak disimpan di repo", "Seeder hanya
# untuk lokal"). Dokumen tetap ber-versi di repo ini; skrip ini menyalin isi
# yang sudah di-merge (ref paddydev/main, bukan working tree) ke jalur erp-paddy
# yang diabaikan git, supaya sesi di sana bisa membacanya.
#
# Usage: tools/sync-docs-erp-paddy.sh [ERP_DIR]   (bawaan ~/WORK/erp-paddy)
#        SYNC_REF=<ref> untuk ref lain (bawaan paddydev/main)
set -euo pipefail

ERP_DIR="${1:-$HOME/WORK/erp-paddy}"
REF="${SYNC_REF:-paddydev/main}"
cd "$(dirname "$0")/.."

if ! git -C "$ERP_DIR" rev-parse --git-dir >/dev/null 2>&1; then
  echo "Bukan repo git: $ERP_DIR" >&2
  exit 1
fi
# Jangan menyalin ke jalur yang akan muncul sebagai perubahan di erp-paddy.
if ! git -C "$ERP_DIR" check-ignore -q docs/x.md || ! git -C "$ERP_DIR" check-ignore -q apps/backend/database/seeders/x.sql; then
  echo "Jalur tujuan tidak diabaikan git erp-paddy — batal" >&2
  exit 1
fi

if [ "$REF" = "paddydev/main" ]; then
  git fetch -q paddydev main
fi
git rev-parse --verify -q "$REF^{commit}" >/dev/null || { echo "Ref tidak ditemukan: $REF" >&2; exit 1; }

before="$(git -C "$ERP_DIR" status --porcelain)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
git archive "$REF" docs backend/database/seeders | tar -x -C "$tmp"

mkdir -p "$ERP_DIR/docs" "$ERP_DIR/apps/backend/database/seeders"
rsync -a --delete "$tmp/docs/" "$ERP_DIR/docs/"
rsync -a --delete "$tmp/backend/database/seeders/" "$ERP_DIR/apps/backend/database/seeders/"

after="$(git -C "$ERP_DIR" status --porcelain)"
if [ "$before" != "$after" ]; then
  echo "Peringatan: git status erp-paddy berubah setelah sinkron — periksa .gitignore" >&2
  exit 1
fi
docs=$(find "$ERP_DIR/docs" -type f | wc -l)
seeders=$(find "$ERP_DIR/apps/backend/database/seeders" -type f | wc -l)
echo "docs: $docs file, seeders: $seeders file → $ERP_DIR ($(git rev-parse --short "$REF"))"

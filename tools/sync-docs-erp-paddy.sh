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
# Absolute before the cd below, so a relative path means the caller's folder.
ERP_DIR="$(cd "$ERP_DIR" 2>/dev/null && pwd -P)" || { echo "Folder tidak ada: ${1:-$HOME/WORK/erp-paddy}" >&2; exit 1; }
cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

# The target must be the root of a git repo, never a subfolder of another one:
# rsync --delete below removes ignored files that git cannot bring back.
if [ "$(git -C "$ERP_DIR" rev-parse --show-toplevel 2>/dev/null)" != "$ERP_DIR" ]; then
  echo "Bukan akar repo git: $ERP_DIR" >&2
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

for p in docs backend/database/seeders; do
  git cat-file -e "$REF:$p" 2>/dev/null || { echo "Folder $p tidak ada di $REF" >&2; exit 1; }
done
# Nothing git sees may live in the targets (tracked, or untracked and not
# ignored): --delete would remove it.
if [ -n "$(git -C "$ERP_DIR" ls-files -co --exclude-standard -- docs apps/backend/database/seeders)" ]; then
  echo "Ada file erp-paddy yang dilacak/tidak diabaikan git di docs/ atau seeders/ — batal" >&2
  exit 1
fi

before="$(git -C "$ERP_DIR" status --porcelain)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
git archive "$REF" docs backend/database/seeders | tar -x -C "$tmp"
# Every incoming file must be ignored by erp-paddy (a negation rule would
# turn it into a change there) — checked before anything is copied.
(cd "$tmp" && find docs backend/database/seeders -type f | sed 's#^backend/#apps/backend/#') > "$tmp/.incoming"
ignored=$(git -C "$ERP_DIR" check-ignore --stdin < "$tmp/.incoming" | wc -l || true)
if [ "$ignored" -ne "$(wc -l < "$tmp/.incoming")" ]; then
  echo "Sebagian file tujuan tidak diabaikan git erp-paddy (aturan negasi?) — batal" >&2
  exit 1
fi

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

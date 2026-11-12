#!/usr/bin/env bash
# Uji pengaman tools/sync-docs-erp-paddy.sh di repo-repo sementara.
# Usage: SYNC_REF=refs/remotes/paddydev/main bash tools/sync-docs-erp-paddy.test.sh
# Exit 0 bila semua lulus. Tidak menyentuh ~/WORK/erp-paddy.
S=$(mktemp -d); trap 'rm -rf "$S"' EXIT
SCRIPT="$(cd "$(dirname "$0")" && pwd)/sync-docs-erp-paddy.sh"; fail=0
mk() { rm -rf "$S/$1"; mkdir -p "$S/$1"; git -C "$S/$1" init -q; printf '/docs/\napps/backend/database/seeders/\n' > "$S/$1/.gitignore"; }
# A: tracked file under docs → refused BEFORE copying, file kept
mk a; mkdir -p $S/a/docs; echo keep > $S/a/docs/KEEP.md; git -C $S/a add -f docs/KEEP.md .gitignore; git -C $S/a -c user.email=t@t -c user.name=t commit -qm x
out=$(bash $SCRIPT $S/a 2>&1); rc=$?
[ $rc -eq 1 ] && [ -f $S/a/docs/KEEP.md ] && [ ! -d $S/a/docs/epics ] && echo "PASS A tracked file" || { echo "FAIL A rc=$rc $(ls $S/a/docs | head -3) :: $out"; fail=1; }
# B: negation rule → refused before copying
mk b; printf '/docs/*\n!/docs/epics/\napps/backend/database/seeders/\n' > $S/b/.gitignore
out=$(bash $SCRIPT $S/b 2>&1); rc=$?
[ $rc -eq 1 ] && [ ! -d $S/b/docs/epics ] && echo "PASS B negation" || { echo "FAIL B rc=$rc :: $out"; fail=1; }
# C: subfolder of another repo → refused
mk c; printf 'docs/
**/apps/backend/database/seeders/
' > $S/c/.gitignore; mkdir -p $S/c/sub
out=$(bash $SCRIPT $S/c/sub 2>&1); rc=$?
[ $rc -eq 1 ] && [ ! -d $S/c/sub/docs ] && echo "PASS C subfolder" || { echo "FAIL C rc=$rc :: $out"; fail=1; }
# D: clean target → copies
mk d
out=$(bash $SCRIPT $S/d 2>&1); rc=$?
[ $rc -eq 0 ] && [ -d $S/d/docs/epics ] && echo "PASS D clean copy" || { echo "FAIL D rc=$rc :: $out"; fail=1; }
exit $fail

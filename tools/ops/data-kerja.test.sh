#!/usr/bin/env bash
# Tes gerbang runner data kerja massal (EPIC-086): tools/ops/data-kerja.sh.
# Hanya memeriksa penolakan sebelum runner menyentuh go/docker/ssh: pembungkus
# `docker`, `ssh`, `scp`, dan `go` di PATH mencatat bila dipanggil.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RUNNER="$ROOT/tools/ops/data-kerja.sh"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/bin"
for tool in docker ssh scp go; do
  printf '#!/bin/sh\necho "%s $*" >> "%s/called"\n' "$tool" "$WORK" > "$WORK/bin/$tool"
  chmod +x "$WORK/bin/$tool"
done
touch "$WORK/isi.xlsx"
fail=0
expect() { # expect <exit> <stderr-part> <env...> -- <args...>
  local want=$1 part=$2; shift 2
  local envs=(); while [ "$1" != "--" ]; do envs+=("$1"); shift; done; shift
  set +e
  out=$(env PATH="$WORK/bin:$PATH" "${envs[@]}" bash "$RUNNER" "$@" 2>&1); code=$?
  set -e
  if [ "$code" != "$want" ] || [[ "$out" != *"$part"* ]]; then
    echo "GAGAL: $* → exit $code, keluaran: $out (harap $want, berisi '$part')"; fail=1
  fi
}
expect 3 "DATA_KERJA_APPROVED=yes" ENV=prod -- apply "$WORK/isi.xlsx" abc
expect 3 "DATA_KERJA_APPROVED=yes" ENV=staging -- apply "$WORK/isi.xlsx" abc
expect 2 "ENV harus staging atau prod" ENV=lokal -- audit
expect 2 "mode harus" ENV=staging -- hapus
expect 2 "sha wajib" ENV=prod DATA_KERJA_APPROVED=yes -- apply "$WORK/isi.xlsx"
expect 2 "file tidak ada" ENV=prod -- preview "$WORK/tidak-ada.xlsx"
expect 2 "sha harus 64 karakter heksadesimal" ENV=prod DATA_KERJA_APPROVED=yes -- apply "$WORK/isi.xlsx" 'abc; rm -rf /tmp/x' 
if [ -f "$WORK/called" ]; then echo "GAGAL: runner memanggil alat sebelum gerbang: $(cat "$WORK/called")"; fail=1; fi
[ "$fail" = 0 ] && echo PASS
exit "$fail"

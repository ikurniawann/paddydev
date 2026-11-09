# EPIC-096 — data kerja karyawan mengikuti file HR

Keputusan aturan ada di `docs/epics/EPIC-096-pemetaan-data-karyawan-hr.md`; rencana eksekusi di `docs/superpowers/plans/2026-11-09-pemetaan-data-karyawan-hr.md`.
File HR, snapshot, dan hasil generate berisi data pribadi: simpan di `~/Desktop/paddydev-backups/data-karyawan-hr/<stamp>/` (umask 077), **jangan di-commit**.

```bash
# 1. snapshot read-only + template EPIC-086 (production)
ssh paddy-prod "docker exec -i paddydev-db sh -c 'psql -X -tA -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -v today=$TODAY'" < snapshot.sql > $DIR/snapshot.json
ENV=prod tools/ops/data-kerja.sh template          # → template data-kerja.xlsx
# 2. generate
python3 generate.py --hr $DIR/hr.xlsx --snapshot $DIR/snapshot.json --template <template> --today $TODAY --out $DIR
# 3. terapkan berurutan (setelah backup dan izin)
psql -X -1 -v ON_ERROR_STOP=1 -f $DIR/01-jabatan-baru.sql
ENV=prod tools/ops/data-kerja.sh preview $DIR/02-data-kerja.xlsx
DATA_KERJA_APPROVED=yes ENV=prod tools/ops/data-kerja.sh apply $DIR/02-data-kerja.xlsx <sha>
psql -X -1 -v ON_ERROR_STOP=1 -f $DIR/03-data-karyawan.sql
# 4. verifikasi: snapshot + generate ulang → ringkasan tanpa perubahan
```

`03` gagal seluruhnya (RAISE EXCEPTION) bila satu nilai lama sudah berubah sejak snapshot: generate ulang dari snapshot baru.
Tes: `python3 -m unittest -v test_generate` dari folder ini.

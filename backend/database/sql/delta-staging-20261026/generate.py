#!/usr/bin/env python3
"""Delta data karyawan staging paddydev → production paddyadorable (setelah cutover 24-10-2026).

Membandingkan tiga DB lokal (dipulihkan dari dump dengan klien pg16):
  --base  dump live saat cutover (paddydev-db-cutover-execute-20261024-233012)
  --stg   dump staging terbaru
  --prod  dump production terbaru (hanya untuk klasifikasi; file SQL memeriksa ulang saat eksekusi)

Yang dibawa (keputusan user 26-10-2026): absensi + foto, pengajuan absensi, cuti, lembur, kasbon,
dan keputusan atasan atas baris itu, hanya untuk karyawan yang sudah ada saat cutover dan ada di
production. Baris yang merujuk karyawan/akun yang dibuat setelah cutover dianggap data uji
developer dan dibuang. Akun, password, notifikasi, audit, master, dan struktur tidak dibawa.
Bentrok dengan production tidak diterapkan otomatis; semuanya masuk laporan.

Output (berisi data pribadi: simpan chmod 600, jangan commit, jangan cetak):
  --out delta.sql  --files files.txt  --report laporan.md
"""
import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timedelta

SESSION = {"timezone": "UTC", "datestyle": "ISO, MDY", "intervalstyle": "postgres", "extra_float_digits": "1"}
CUTOVER = "2026-10-24 16:30:20+00"  # live dibekukan 23:30:20 WIB; dump baseline dibuat sesudahnya
# Tabel yang dibawa, urutan insert (pengajuan absensi merujuk absensi). key = penanda duplikat:
# baris production lain milik karyawan yang sama, dengan nilai kolom key yang sama, dibuat setelah cutover.
TABLES = {
    "hris.attendance": None,  # unik (employee_id, date): ditangani khusus
    "hris.attendance_requests": ["employee_id", "date", "kind"],
    "hris.leaves": ["employee_id", "start_date", "end_date", "leave_type"],
    "hris.overtime_requests": ["employee_id", "date"],
    "hris.loans": ["employee_id"],
}
FILE_COLUMNS = {
    "hris.attendance": ["clock_in_photo_url", "clock_out_photo_url"],
    "hris.attendance_requests": ["attachment_path"],
    "hris.leaves": ["attachment_url"],
    "hris.overtime_requests": ["attachments"],
}
STORAGE_PREFIX = "private/"


def main():
    ap = argparse.ArgumentParser()
    for k in ("db", "base", "stg", "prod", "out", "files", "report"):
        ap.add_argument("--" + k, required=True)
    ap.add_argument("--stg-label", default="dump staging")
    a = ap.parse_args()
    env = dict(os.environ, PGOPTIONS=" ".join(f"-c {k}={v.replace(' ', '')}" for k, v in SESSION.items()))

    def q(db, sql):
        r = subprocess.run(["psql", a.db + db, "-XAt", "-v", "ON_ERROR_STOP=1", "-c", sql], capture_output=True, text=True, env=env)
        if r.returncode:
            sys.exit(f"psql {db}: {r.stderr.strip()}")
        return r.stdout.rstrip("\n")

    def qj(db, sql):
        return json.loads(q(db, sql) or "null")

    def md5map(db, rel):
        return qj(db, f"SELECT coalesce(json_object_agg(id, md5(t::text)), '{{}}') FROM {rel} t")

    def rows(db, rel, ids):
        if not ids:
            return []
        lit = ",".join(f"'{i}'" for i in sorted(ids))
        return qj(db, f"SELECT coalesce(json_agg(t ORDER BY t.id), '[]') FROM {rel} t WHERE t.id IN ({lit})")

    def columns(rel):
        s, t = rel.split(".")
        return q(a.stg, f"SELECT string_agg(column_name, ',' ORDER BY ordinal_position) FROM information_schema.columns "
                        f"WHERE table_schema='{s}' AND table_name='{t}'").split(",")

    def fk_cols(rel, target):
        return q(a.stg, f"""SELECT coalesce(string_agg(att.attname, ','), '') FROM pg_constraint c
            JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = c.conkey[1]
            WHERE c.contype = 'f' AND array_length(c.conkey, 1) = 1 AND c.conrelid = '{rel}'::regclass
              AND c.confrelid = '{target}'::regclass""").split(",")

    base_emp = set(qj(a.base, "SELECT coalesce(json_agg(id), '[]') FROM hris.employees"))
    prod_emp = set(qj(a.prod, "SELECT coalesce(json_agg(id), '[]') FROM hris.employees"))
    base_usr = set(qj(a.base, "SELECT coalesce(json_agg(id), '[]') FROM auth.users"))
    prod_usr = set(qj(a.prod, "SELECT coalesce(json_agg(id), '[]') FROM auth.users"))
    emp_info = {r["id"]: r for r in qj(a.stg, "SELECT json_agg(json_build_object('id', id, 'nip', nip, 'nama', full_name, 'dibuat', created_at)) FROM hris.employees")}

    def who(emp_id):
        e = emp_info.get(emp_id) or {}
        return f"{e.get('nama', '?')} ({e.get('nip', emp_id)})"

    plan = {rel: {"insert": [], "update": [], "merge": []} for rel in TABLES}
    conflicts, dropped = [], []
    base_md5 = {}

    for rel, key in TABLES.items():
        b, s, p = md5map(a.base, rel), md5map(a.stg, rel), md5map(a.prod, rel)
        ins = {k for k in s if k not in b}
        upd = {k for k in s if k in b and s[k] != b[k]}
        base_md5[rel] = b
        emp_fk = [c for c in fk_cols(rel, "hris.employees") if c]
        usr_fk = [c for c in fk_cols(rel, "auth.users") if c]
        for r in rows(a.stg, rel, ins | upd):
            kind = "insert" if r["id"] in ins else "update"
            label = f"{rel} {kind} {who(r.get('employee_id'))} {r.get('date') or r.get('start_date') or str(r.get('created_at'))[:10]}"
            bad = [f"{c}→karyawan baru" for c in emp_fk if r.get(c) and r[c] not in base_emp] + \
                  [f"{c}→akun baru" for c in usr_fk if r.get(c) and r[c] not in base_usr]
            missing = [f"{c} tidak ada di production" for c in emp_fk if r.get(c) and r[c] not in prod_emp] + \
                      [f"{c} tidak ada di production" for c in usr_fk if r.get(c) and r[c] not in prod_usr]
            if bad:
                dropped.append(f"{label}: data uji ({', '.join(bad)})")
                continue
            if missing:
                conflicts.append(f"{label}: dilewati, {', '.join(missing)}")
                continue
            if kind == "update":
                if r["id"] not in p:
                    conflicts.append(f"{label}: dilewati, baris sudah dihapus di production")
                elif p[r["id"]] == s[r["id"]]:
                    pass  # production sudah sama
                elif p[r["id"]] != b[r["id"]]:
                    conflicts.append(f"{label}: dilewati, baris juga diubah di production setelah cutover")
                else:
                    plan[rel]["update"].append(r)
                continue
            if r["id"] in p:
                continue  # sudah ada di production (ikut dump cutover)
            if rel == "hris.attendance":
                pr = qj(a.prod, f"SELECT row_to_json(t) FROM hris.attendance t WHERE employee_id = '{r['employee_id']}' AND date = '{r['date']}'")
                if pr:
                    fmt = lambda x: "-" if not x else (datetime.fromisoformat(x) + timedelta(hours=7)).strftime("%H:%M")
                    desc = f"prod {fmt(pr['clock_in'])}–{fmt(pr['clock_out'])} / staging {fmt(r['clock_in'])}–{fmt(r['clock_out'])} WIB"
                    if pr["clock_out"] is None and r["clock_out"] and r["clock_in"] and (pr["clock_in"] is None or r["clock_in"] <= pr["clock_in"]):
                        plan[rel]["merge"].append(r)
                        conflicts.append(f"{label}: DIGABUNG (masuk awal + pulang akhir = nilai staging), {desc}")
                    else:
                        conflicts.append(f"{label}: dilewati, absen ganda tidak bisa digabung otomatis, {desc}")
                    continue
            else:
                cond = " AND ".join(f"{c} = '{r[c]}'" for c in key if r.get(c) is not None)
                dup = q(a.prod, f"SELECT count(*) FROM {rel} WHERE {cond} AND created_at > '{CUTOVER}'")
                if dup != "0":
                    conflicts.append(f"{label}: dilewati, sudah diajukan ulang di production ({dup} baris)")
                    continue
            plan[rel]["insert"].append(r)

    # Pengajuan absensi yang merujuk absensi yang tidak dibawa ikut dilewati.
    carried_att = {r["id"] for k in ("insert", "update", "merge") for r in plan["hris.attendance"][k]}
    stg_att = md5map(a.stg, "hris.attendance")
    for k in ("insert", "update"):
        keep = []
        for r in plan["hris.attendance_requests"][k]:
            aid = r.get("attendance_id")
            if aid and aid not in carried_att and aid in stg_att and aid not in base_md5["hris.attendance"]:
                conflicts.append(f"hris.attendance_requests {k} {who(r['employee_id'])} {r['date']}: dilewati, absensi terkait tidak dibawa")
            else:
                keep.append(r)
        plan["hris.attendance_requests"][k] = keep

    # Perubahan lain di staging yang sengaja tidak dibawa (ringkasan per tabel).
    other = q(a.stg, "SELECT string_agg(n.nspname || '.' || c.relname, ' ' ORDER BY 1) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace "
                     "WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')").split()
    skipped_tables = []
    for rel in other:
        if rel in TABLES:
            continue
        cnt = f"SELECT count(*) || ':' || coalesce(md5(string_agg(md5(t::text), '' ORDER BY md5(t::text))), '') FROM {rel} t"
        bs, ss = q(a.base, cnt), q(a.stg, cnt)
        if bs != ss:
            skipped_tables.append(f"{rel} (baris {bs.split(':')[0]} → {ss.split(':')[0]})")

    # --- SQL
    def j(x):
        return json.dumps(x, ensure_ascii=False).replace("'", "''")

    def t(rel):
        return rel.replace(".", "_")

    sql = ["\\set ON_ERROR_STOP on", "BEGIN;",
           "SET LOCAL search_path = public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth;"]
    sql += [f"SET LOCAL {k} = '{v}';" for k, v in SESSION.items()]
    for rel, key in TABLES.items():
        n = t(rel)
        sql.append(f"CREATE TEMP TABLE i_{n} ON COMMIT DROP AS SELECT * FROM json_populate_recordset(NULL::{rel}, '{j(plan[rel]['insert'])}');")
        sql.append(f"CREATE TEMP TABLE u_{n} ON COMMIT DROP AS SELECT * FROM json_populate_recordset(NULL::{rel}, '{j(plan[rel]['update'])}');")
        sql.append(f"CREATE TEMP TABLE b_{n} (id uuid PRIMARY KEY, m text) ON COMMIT DROP;")
        sql.append(f"INSERT INTO b_{n} SELECT key::uuid, value FROM json_each_text('{j({r['id']: base_md5[rel][r['id']] for r in plan[rel]['update']})}');")
        sql.append(f"""DO $$ BEGIN
          IF EXISTS (SELECT 1 FROM {rel} x JOIN i_{n} d USING (id)) THEN RAISE EXCEPTION '{rel}: id delta sudah ada di production'; END IF;
          IF (SELECT count(*) FROM {rel} x JOIN b_{n} b ON b.id = x.id WHERE md5(x::text) = b.m) <> (SELECT count(*) FROM b_{n})
            THEN RAISE EXCEPTION '{rel}: baris yang akan diperbarui sudah berubah di production'; END IF;
        END $$;""")
        if key:
            cond = " AND ".join(f"x.{c} IS NOT DISTINCT FROM d.{c}" for c in key)
            sql.append(f"""DO $$ BEGIN IF EXISTS (SELECT 1 FROM {rel} x JOIN i_{n} d ON {cond} WHERE x.id <> d.id AND x.created_at > '{CUTOVER}')
              THEN RAISE EXCEPTION '{rel}: muncul pengajuan ganda baru di production'; END IF; END $$;""")
    sql.append(f"CREATE TEMP TABLE m_att ON COMMIT DROP AS SELECT * FROM json_populate_recordset(NULL::hris.attendance, '{j(plan['hris.attendance']['merge'])}');")
    sql.append(f"""DO $$ BEGIN
      IF EXISTS (SELECT 1 FROM hris.attendance x JOIN i_hris_attendance d ON d.employee_id = x.employee_id AND d.date = x.date)
        THEN RAISE EXCEPTION 'hris.attendance: karyawan+tanggal delta sudah ada di production'; END IF;
      IF (SELECT count(*) FROM hris.attendance x JOIN m_att m ON m.employee_id = x.employee_id AND m.date = x.date
          WHERE x.clock_out IS NULL AND (x.clock_in IS NULL OR m.clock_in <= x.clock_in)) <> (SELECT count(*) FROM m_att)
        THEN RAISE EXCEPTION 'absen gabungan: baris production sudah berubah'; END IF;
      IF EXISTS (SELECT 1 FROM hris.attendance_periods p WHERE p.status = 'closed' AND EXISTS (SELECT 1 FROM
          (SELECT date FROM i_hris_attendance UNION SELECT date FROM m_att UNION SELECT date FROM u_hris_attendance) z
          WHERE z.date BETWEEN p.start_date AND p.end_date))
        THEN RAISE EXCEPTION 'periode absensi untuk tanggal delta sudah di-closing'; END IF;
    END $$;""")
    sql.append("CREATE TEMP TABLE before_n ON COMMIT DROP AS SELECT * FROM (VALUES " +
               ",".join(f"('{rel}', (SELECT count(*) FROM {rel}))" for rel in TABLES) + ") v(rel, n);")
    # Tanpa trigger: nilai turunan (jam kerja, terlambat) ikut dari staging dan tidak ada notifikasi baru.
    sql.append("SET LOCAL session_replication_role = replica;")
    for rel in TABLES:
        sql.append(f"INSERT INTO {rel} SELECT * FROM i_{t(rel)};")
    att_cols = [c for c in columns("hris.attendance") if c not in ("id", "employee_id", "date", "created_at")]
    sql.append(f"UPDATE hris.attendance x SET ({','.join(att_cols)}) = ({','.join('m.' + c for c in att_cols)}) "
               "FROM m_att m WHERE m.employee_id = x.employee_id AND m.date = x.date;")
    for rel in TABLES:
        cols = [c for c in columns(rel) if c != "id"]
        sql.append(f"UPDATE {rel} x SET ({','.join(cols)}) = ({','.join('u.' + c for c in cols)}) FROM u_{t(rel)} u WHERE u.id = x.id;")
    sql.append("SET LOCAL session_replication_role = origin;")
    want = {rel: len(plan[rel]["insert"]) for rel in TABLES}
    sql.append("DO $$ DECLARE bad text; BEGIN SELECT string_agg(b.rel || ' +' || (c.n - b.n) || '/' || e.w, ', ') INTO bad FROM before_n b "
               "JOIN (VALUES " + ",".join(f"('{r}', {n})" for r, n in want.items()) + ") e(rel, w) USING (rel) "
               "CROSS JOIN LATERAL (SELECT (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %s', b.rel), false, true, '')))[1]::text::bigint AS n) c "
               "WHERE c.n - b.n <> e.w; IF bad IS NOT NULL THEN RAISE EXCEPTION 'jumlah baris tidak sesuai: %', bad; END IF; END $$;")
    sql.append("""DO $$ DECLARE r record; n bigint; bad text := ''; BEGIN
      FOR r IN SELECT con.conname, con.conrelid::regclass AS src, con.confrelid::regclass AS dst,
          (SELECT string_agg(format('s.%I = d.%I', a1.attname, a2.attname), ' AND ') FROM unnest(con.conkey, con.confkey) k(c1, c2)
             JOIN pg_attribute a1 ON a1.attrelid = con.conrelid AND a1.attnum = k.c1
             JOIN pg_attribute a2 ON a2.attrelid = con.confrelid AND a2.attnum = k.c2) AS cond,
          (SELECT string_agg(format('s.%I IS NOT NULL', a1.attname), ' AND ') FROM unnest(con.conkey) k(c1)
             JOIN pg_attribute a1 ON a1.attrelid = con.conrelid AND a1.attnum = k.c1) AS notnull
        FROM pg_constraint con WHERE con.contype = 'f' AND con.conrelid::regclass::text = ANY (ARRAY[""" +
               ",".join(f"'{r}'" for r in TABLES) + """])
      LOOP
        EXECUTE format('SELECT count(*) FROM %s s WHERE %s AND NOT EXISTS (SELECT 1 FROM %s d WHERE %s)', r.src, r.notnull, r.dst, r.cond) INTO n;
        IF n > 0 THEN bad := bad || r.src || '.' || r.conname || '=' || n || ' '; END IF;
      END LOOP;
      IF bad <> '' THEN RAISE EXCEPTION 'FK yatim: %', bad; END IF;
    END $$;""")
    summary = ", ".join(f"{rel.split('.')[1]} +{len(plan[rel]['insert'])}/~{len(plan[rel]['update'])}" for rel in TABLES)
    summary += f", absen gabungan {len(plan['hris.attendance']['merge'])}"
    sql.append(f"DO $$ BEGIN RAISE NOTICE 'delta staging OK: {summary}'; END $$;")
    sql.append("COMMIT;")

    files = set()
    for rel, cols in FILE_COLUMNS.items():
        for k in ("insert", "update", "merge"):
            for r in plan[rel][k]:
                for c in cols:
                    v = r.get(c)
                    files.update(v if isinstance(v, list) else ([v] if v else []))

    old = os.umask(0o077)
    with open(a.out, "w") as f:
        f.write("\n".join(sql) + "\n")
    with open(a.files, "w") as f:
        f.write("".join(f"{STORAGE_PREFIX}{p}\n" for p in sorted(files)))
    names = {"hris.attendance": "Absensi", "hris.attendance_requests": "Pengajuan absensi", "hris.leaves": "Cuti",
             "hris.overtime_requests": "Lembur", "hris.loans": "Kasbon"}
    rep = [f"# Laporan delta staging → production", "", f"Sumber: {a.stg_label}. Baseline: dump cutover 24-10-2026 23:30 WIB.", "",
           "## Akan dibawa", "", "| Data | Baru | Keputusan/perubahan | Digabung |", "|---|---|---|---|"]
    for rel in TABLES:
        rep.append(f"| {names[rel]} | {len(plan[rel]['insert'])} | {len(plan[rel]['update'])} | {len(plan[rel]['merge']) if rel == 'hris.attendance' else '–'} |")
    rep += ["", f"File foto/lampiran: {len(files)}", "", f"## Bentrok dengan production ({len(conflicts)})", ""]
    rep += [f"- {c}" for c in conflicts] or ["- tidak ada"]
    rep += ["", f"## Dibuang sebagai data uji developer ({len(dropped)})", ""] + ([f"- {d}" for d in dropped] or ["- tidak ada"])
    new_emp = [who(e) for e in emp_info if e not in base_emp]
    rep += ["", f"Karyawan yang dibuat di staging setelah cutover (tidak dibawa): {', '.join(new_emp) or 'tidak ada'}", "",
            "## Perubahan staging lain yang tidak dibawa", ""] + ([f"- {x}" for x in skipped_tables] or ["- tidak ada"])
    with open(a.report, "w") as f:
        f.write("\n".join(rep) + "\n")
    os.umask(old)
    print(f"{summary}; file {len(files)}; bentrok {len(conflicts)}; data uji {len(dropped)}; tabel lain berubah {len(skipped_tables)}")


if __name__ == "__main__":
    main()

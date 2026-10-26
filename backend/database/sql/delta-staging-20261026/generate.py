#!/usr/bin/env python3
"""Delta data staging paddydev → production paddyadorable (setelah cutover 24-10-2026).

Membandingkan DB baseline (dump live saat cutover) dengan DB staging (dump tetap
staging-db-before-freeze-20261026-110218) per primary key + md5 baris, lalu menulis
satu file SQL yang menerapkan keputusan user 26-10-2026 ke production dalam satu
transaksi. Output berisi data pribadi (hash password, lokasi): simpan chmod 600,
jangan commit, jangan cetak isinya.

  python3 -I generate.py --db postgres://postgres@127.0.0.1:55499/ \
      --base mig_base --stg mig_stg --out delta.sql --files files.txt
"""
import argparse
import json
import os
import subprocess
import sys

# Setelan sesi yang sama di generator dan di file delta: md5(row::text) bergantung padanya.
SESSION = {"timezone": "UTC", "datestyle": "ISO, MDY", "intervalstyle": "postgres", "extra_float_digits": "1"}

# Keputusan user 26-10-2026.
SKIP_ATTENDANCE_REQUESTS = {"22efc71d-f0e0-47d7-9d21-d22e8f966cd7"}  # Alif 24-10, sudah diajukan ulang di prod
MERGE_ATTENDANCE = {("202601530", "2026-10-25"), ("EMP-2026-00008", "2026-10-25")}  # masuk awal + pulang akhir
INSERT_TABLES = ["hris.attendance", "hris.attendance_requests", "hris.leaves", "hris.overtime_requests",
                 "hris.loans", "public.notifications", "audit.audit_log"]
DECISION_TABLES = ["hris.leaves", "hris.overtime_requests"]  # baris diubah (persetujuan atasan)
STORAGE_PREFIX = "private/"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--base", required=True)
    ap.add_argument("--stg", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--files", required=True)
    a = ap.parse_args()
    env = dict(os.environ, PGOPTIONS=" ".join(f"-c {k}={v.replace(' ', '')}" for k, v in SESSION.items()))

    def q(db, sql):
        r = subprocess.run(["psql", a.db + db, "-XAt", "-v", "ON_ERROR_STOP=1", "-c", sql],
                           capture_output=True, text=True, env=env)
        if r.returncode:
            sys.exit(f"psql {db}: {r.stderr.strip()}")
        return r.stdout.rstrip("\n")

    def rows_md5(db, rel):
        out = q(db, f"SELECT coalesce(json_object_agg(id, md5(t::text)), '{{}}') FROM {rel} t")
        return json.loads(out)

    def rows_json(db, rel, ids):
        if not ids:
            return []
        lit = ",".join(f"'{i}'" for i in sorted(ids))
        return json.loads(q(db, f"SELECT coalesce(json_agg(t ORDER BY t.id), '[]') FROM {rel} t WHERE t.id IN ({lit})"))

    def columns(rel):
        s, t = rel.split(".")
        return q(a.stg, f"SELECT string_agg(format('%I', column_name), ',' ORDER BY ordinal_position) "
                        f"FROM information_schema.columns WHERE table_schema='{s}' AND table_name='{t}'").split(",")

    inserts, updates, base_md5 = {}, {}, {}
    for rel in sorted(set(INSERT_TABLES + DECISION_TABLES)):
        b, s = rows_md5(a.base, rel), rows_md5(a.stg, rel)
        inserts[rel] = {k for k in s if k not in b}
        updates[rel] = {k for k in s if k in b and s[k] != b[k]}
        base_md5[rel] = {k: b[k] for k in updates[rel]}

    inserts["hris.attendance_requests"] -= SKIP_ATTENDANCE_REQUESTS
    for rel in INSERT_TABLES:
        if rel not in DECISION_TABLES and updates[rel]:
            sys.exit(f"{rel}: {len(updates[rel])} baris diubah di staging, belum ada keputusan")

    # Notifikasi milik pengajuan yang dilewati (dibuat bersamaan, judul "Pengajuan Absensi: ...").
    skip_lit = ",".join(f"'{i}'" for i in SKIP_ATTENDANCE_REQUESTS)
    skip_notif = set(q(a.stg, f"""SELECT coalesce(string_agg(n.id::text, ' '), '') FROM public.notifications n
        JOIN hris.attendance_requests r ON r.id IN ({skip_lit})
        WHERE n.title LIKE 'Pengajuan Absensi:%' AND abs(extract(epoch FROM n.created_at - r.created_at)) < 5""").split())
    inserts["public.notifications"] -= skip_notif

    # Absensi: pisahkan baris yang digabung ke baris production (karyawan+tanggal sudah ada di prod).
    att = rows_json(a.stg, "hris.attendance", inserts["hris.attendance"])
    nip = dict(l.split("|") for l in q(a.stg, "SELECT id || '|' || nip FROM hris.employees").splitlines())
    merge = [r for r in att if (nip[r["employee_id"]], r["date"]) in MERGE_ATTENDANCE]
    if len(merge) != len(MERGE_ATTENDANCE):
        sys.exit(f"absen gabungan: ditemukan {len(merge)}, diharapkan {len(MERGE_ATTENDANCE)}")
    inserts["hris.attendance"] -= {r["id"] for r in merge}

    # Password yang diganti di staging.
    pw_base = json.loads(q(a.base, "SELECT json_object_agg(id, md5(password_hash)) FROM auth.users"))
    pw_stg = json.loads(q(a.stg, "SELECT json_object_agg(id, json_build_object('h', password_hash, 'm', md5(password_hash), 'u', updated_at)) FROM auth.users"))
    pw = [{"id": k, "base_md5": pw_base[k], "hash": v["h"], "updated_at": v["u"]}
          for k, v in pw_stg.items() if k in pw_base and pw_base[k] != v["m"]]

    def j(x):
        return json.dumps(x, ensure_ascii=False).replace("'", "''")

    sql = ["\\set ON_ERROR_STOP on", "BEGIN;",
           "SET LOCAL search_path = public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth;"]
    sql += [f"SET LOCAL {k} = '{v}';" for k, v in SESSION.items()]
    touched = sorted(set(INSERT_TABLES) | {"auth.users"})

    # --- Precondition: production belum berubah dari kondisi yang diperiksa.
    for rel in INSERT_TABLES:
        rows = rows_json(a.stg, rel, inserts[rel])
        sql.append(f"CREATE TEMP TABLE d_{rel.replace('.', '_')} ON COMMIT DROP AS SELECT * FROM json_populate_recordset(NULL::{rel}, '{j(rows)}');")
        sql.append(f"""DO $$ BEGIN IF EXISTS (SELECT 1 FROM {rel} t JOIN d_{rel.replace('.', '_')} d USING (id))
          THEN RAISE EXCEPTION '{rel}: id delta sudah ada di production'; END IF; END $$;""")
    sql.append(f"CREATE TEMP TABLE m_att ON COMMIT DROP AS SELECT * FROM json_populate_recordset(NULL::hris.attendance, '{j(merge)}');")
    sql.append("""DO $$ BEGIN
      IF EXISTS (SELECT 1 FROM hris.attendance t JOIN d_hris_attendance d ON d.employee_id = t.employee_id AND d.date = t.date)
        THEN RAISE EXCEPTION 'hris.attendance: karyawan+tanggal delta sudah ada di production'; END IF;
      IF (SELECT count(*) FROM hris.attendance t JOIN m_att m ON m.employee_id = t.employee_id AND m.date = t.date
          WHERE t.clock_out IS NULL AND t.clock_in IS NOT NULL AND m.clock_in <= t.clock_in AND m.clock_out IS NOT NULL) <> (SELECT count(*) FROM m_att)
        THEN RAISE EXCEPTION 'absen gabungan: baris production sudah berubah (jam pulang terisi / jam masuk lebih awal)'; END IF;
      IF EXISTS (SELECT 1 FROM hris.attendance_periods p WHERE p.status = 'closed' AND EXISTS (
          SELECT 1 FROM (SELECT date FROM d_hris_attendance UNION SELECT date FROM m_att) x WHERE x.date BETWEEN p.start_date AND p.end_date))
        THEN RAISE EXCEPTION 'periode absensi untuk tanggal delta sudah di-closing'; END IF;
    END $$;""")
    for rel in DECISION_TABLES:
        rows = rows_json(a.stg, rel, updates[rel])
        t = rel.replace(".", "_")
        sql.append(f"CREATE TEMP TABLE u_{t} ON COMMIT DROP AS SELECT * FROM json_populate_recordset(NULL::{rel}, '{j(rows)}');")
        sql.append(f"CREATE TEMP TABLE b_{t} (id uuid PRIMARY KEY, m text) ON COMMIT DROP;")
        sql.append(f"INSERT INTO b_{t} SELECT key::uuid, value FROM json_each_text('{j(base_md5[rel])}');")
        sql.append(f"""DO $$ BEGIN IF (SELECT count(*) FROM {rel} t JOIN b_{t} b ON b.id = t.id WHERE md5(t::text) = b.m) <> (SELECT count(*) FROM b_{t})
          THEN RAISE EXCEPTION '{rel}: baris yang diputuskan di staging sudah berubah di production'; END IF; END $$;""")
    sql.append(f"CREATE TEMP TABLE pw ON COMMIT DROP AS SELECT * FROM json_to_recordset('{j(pw)}') AS x(id uuid, base_md5 text, hash text, updated_at timestamptz);")
    sql.append("""DO $$ BEGIN IF (SELECT count(*) FROM auth.users u JOIN pw ON pw.id = u.id WHERE md5(u.password_hash) = pw.base_md5) <> (SELECT count(*) FROM pw)
      THEN RAISE EXCEPTION 'password: akun sudah mengganti password di production'; END IF; END $$;""")

    # --- Snapshot jumlah untuk verifikasi.
    sql.append("CREATE TEMP TABLE before_n ON COMMIT DROP AS SELECT * FROM (VALUES " + ",".join(
        f"('{rel}', (SELECT count(*) FROM {rel}))" for rel in INSERT_TABLES) + ") v(rel, n);")

    # --- Terapkan tanpa trigger (notifikasi ikut disalin; trigger akan menggandakannya).
    sql.append("SET LOCAL session_replication_role = replica;")
    for rel in INSERT_TABLES:
        sql.append(f"INSERT INTO {rel} SELECT * FROM d_{rel.replace('.', '_')};")
    att_cols = [c for c in columns("hris.attendance") if c not in ("id", "employee_id", "date", "created_at")]
    sql.append(f"""UPDATE hris.attendance t SET ({",".join(att_cols)}) = ({",".join("m." + c for c in att_cols)})
      FROM m_att m WHERE m.employee_id = t.employee_id AND m.date = t.date;""")
    for rel in DECISION_TABLES:
        cols = [c for c in columns(rel) if c != "id"]
        t = rel.replace(".", "_")
        sql.append(f"UPDATE {rel} t SET ({','.join(cols)}) = ({','.join('u.' + c for c in cols)}) FROM u_{t} u WHERE u.id = t.id;")
    sql.append("UPDATE auth.users u SET password_hash = pw.hash, updated_at = pw.updated_at FROM pw WHERE pw.id = u.id;")
    sql.append("SET LOCAL session_replication_role = origin;")

    # --- Verifikasi: jumlah bertambah tepat, tidak ada FK yatim di tabel yang disentuh.
    expect = {rel: len(inserts[rel]) for rel in INSERT_TABLES}
    sql.append("DO $$ DECLARE bad text; BEGIN SELECT string_agg(b.rel || ' +' || (n.c - b.n) || '/' || e.want, ', ') INTO bad FROM before_n b "
               "JOIN (VALUES " + ",".join(f"('{r}', {n})" for r, n in expect.items()) + ") e(rel, want) USING (rel) "
               "CROSS JOIN LATERAL (SELECT (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %s', b.rel), false, true, '')))[1]::text::bigint AS c) n "
               "WHERE n.c - b.n <> e.want; IF bad IS NOT NULL THEN RAISE EXCEPTION 'jumlah baris tidak sesuai: %', bad; END IF; END $$;")
    sql.append("""DO $$ DECLARE r record; n bigint; bad text := ''; BEGIN
      FOR r IN SELECT con.conname, con.conrelid::regclass AS src, con.confrelid::regclass AS dst,
          (SELECT string_agg(format('s.%I = d.%I', a1.attname, a2.attname), ' AND ')
             FROM unnest(con.conkey, con.confkey) k(c1, c2)
             JOIN pg_attribute a1 ON a1.attrelid = con.conrelid AND a1.attnum = k.c1
             JOIN pg_attribute a2 ON a2.attrelid = con.confrelid AND a2.attnum = k.c2) AS cond,
          (SELECT string_agg(format('s.%I IS NOT NULL', a1.attname), ' AND ')
             FROM unnest(con.conkey) k(c1) JOIN pg_attribute a1 ON a1.attrelid = con.conrelid AND a1.attnum = k.c1) AS notnull
        FROM pg_constraint con WHERE con.contype = 'f' AND con.conrelid::regclass::text = ANY (ARRAY[""" + ",".join(f"'{t}'" for t in touched) + """])
      LOOP
        EXECUTE format('SELECT count(*) FROM %s s WHERE %s AND NOT EXISTS (SELECT 1 FROM %s d WHERE %s)', r.src, r.notnull, r.dst, r.cond) INTO n;
        IF n > 0 THEN bad := bad || r.src || '.' || r.conname || '=' || n || ' '; END IF;
      END LOOP;
      IF bad <> '' THEN RAISE EXCEPTION 'FK yatim: %', bad; END IF;
    END $$;""")
    summary = ", ".join(f"{r.split('.')[1]} +{n}" for r, n in expect.items())
    summary += f", absen gabungan {len(merge)}, keputusan cuti {len(updates['hris.leaves'])}, keputusan lembur {len(updates['hris.overtime_requests'])}, password {len(pw)}"
    sql.append(f"DO $$ BEGIN RAISE NOTICE 'delta staging OK: {summary}'; END $$;")
    sql.append("COMMIT;")

    old = os.umask(0o077)
    with open(a.out, "w") as f:
        f.write("\n".join(sql) + "\n")

    # --- File storage yang dirujuk baris yang dibawa.
    files = set()
    for r in att:  # termasuk baris absen gabungan
        for c in ("clock_in_photo_url", "clock_out_photo_url"):
            if r.get(c):
                files.add(r[c])
    for r in rows_json(a.stg, "hris.overtime_requests", inserts["hris.overtime_requests"] | updates["hris.overtime_requests"]):
        files.update(r.get("attachments") or [])
    for r in rows_json(a.stg, "hris.attendance_requests", inserts["hris.attendance_requests"]):
        if r.get("attachment_path"):
            files.add(r["attachment_path"])
    for r in rows_json(a.stg, "hris.leaves", inserts["hris.leaves"] | updates["hris.leaves"]):
        if r.get("attachment_url"):
            files.add(r["attachment_url"])
    with open(a.files, "w") as f:
        f.write("".join(f"{STORAGE_PREFIX}{p}\n" for p in sorted(files)))
    os.umask(old)
    print(f"{summary}; file storage {len(files)}; notifikasi dilewati {len(skip_notif)}")


if __name__ == "__main__":
    main()

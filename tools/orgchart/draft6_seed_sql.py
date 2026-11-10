#!/usr/bin/env python3
"""EPIC-101: SQL seed bagan Draft 6 dari CSV klien (docs/hris/org-draft6).

Usage: python3 tools/orgchart/draft6_seed_sql.py docs/hris/org-draft6 \
         > backend/database/migrations/deltas/20261115090100_org_chart_draft6_seed.sql

Idempoten: ON CONFLICT DO NOTHING. needs_confirm/status mengikuti ruling R5.
"""
import csv
import sys

src = sys.argv[1]
CONFIRM_FLAGS = ("HIGHLIGHTED_NO_LEGEND", "DUPLICATE_CANDIDATE", "AMBIGUOUS_ABBR")


def q(v):
    return "NULL" if v is None or v == "" else "'" + v.replace("'", "''") + "'"


def arr(items):
    return "ARRAY[" + ",".join(q(i) for i in items) + "]::text[]" if items else "'{}'::text[]"


out = ["-- EPIC-101: seed bagan Draft 6. Dibuat oleh tools/orgchart/draft6_seed_sql.py dari",
       "-- docs/hris/org-draft6/paddy_org_positions_seed.csv + title_aliases.csv. Jangan diedit tangan.",
       "INSERT INTO hris.org_chart_versions (label, source_file)",
       "VALUES ('Draft 6', 'Org_Chart_Paddy_2026_-_Draft_6_1.xlsx') ON CONFLICT (label) DO NOTHING;", ""]
rows = list(csv.DictReader(open(f"{src}/paddy_org_positions_seed.csv", newline="")))
values = []
for i, r in enumerate(rows, start=1):
    flags = [f for f in r["flags"].split(";") if f]
    pos = r["is_position"] == "Y"
    confirm = pos and (r["level_confidence"] == "needs_confirm" or any(f in flags for f in CONFIRM_FLAGS))
    status = "planned" if "HIGHLIGHTED_NO_LEGEND" in flags else "active"
    stores = [s for s in r["store_codes"].split(";") if s]
    values.append("  (" + ", ".join([
        q(r["position_code"]), q(r["parent_code"]), str(i), q(r["normalized_title"]), q(r["raw_title"]),
        "true" if pos else "false", q(r["division_code"]), q(r["division"]),
        q(r["job_level"] if pos else ""), q(r["level_confidence"] if pos else ""),
        q(r["employment_type"] if pos else ""), q(r["vendor"]), q(r["region"]),
        arr(stores), q(status), "true" if confirm else "false", arr(flags)]) + ")")
out += ["INSERT INTO hris.org_chart_nodes (version_id, code, parent_code, sort_order, title, raw_title, is_position,",
        "  division_code, division_name, job_level, level_confidence, employment_type, vendor, region, store_codes,",
        "  status, needs_confirm, flags)",
        "SELECT v.id, s.* FROM hris.org_chart_versions v, (VALUES",
        ",\n".join(values),
        ") AS s (code, parent_code, sort_order, title, raw_title, is_position, division_code, division_name, job_level,",
        "  level_confidence, employment_type, vendor, region, store_codes, status, needs_confirm, flags)",
        "WHERE v.label = 'Draft 6'",
        "ON CONFLICT (version_id, code) DO NOTHING;", ""]
aliases = list(csv.DictReader(open(f"{src}/title_aliases.csv", newline="")))
out += ["INSERT INTO hris.org_title_aliases (alias, title, region, needs_review, note) VALUES",
        ",\n".join("  (" + ", ".join([q(a["alias"].strip().lower()), q(a["title"]), q(a["region"]),
                                       a["needs_review"].strip().lower(), q(a["note"])]) + ")" for a in aliases),
        "ON CONFLICT (alias) DO NOTHING;"]
print("\n".join(out))

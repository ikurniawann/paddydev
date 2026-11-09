#!/usr/bin/env python3
"""EPIC-096: data kerja karyawan production mengikuti file HR.

    generate.py --hr FILE.xlsx --snapshot snapshot.json --template data-kerja.xlsx --today YYYY-MM-DD --out DIR

Membaca file HR dan snapshot read-only (snapshot.sql), lalu menulis ke DIR:
  01-jabatan-baru.sql     jabatan yang belum ada di master (R2)
  02-data-kerja.xlsx      template EPIC-086 terisi: penempatan, departemen, jabatan
  03-data-karyawan.sql    atasan, status kontrak, join date, NIP (satu transaksi, guard nilai lama)
  rencana.csv             setiap perubahan dan setiap baris yang dilewati
  ringkasan.txt           jumlah per field dan per alasan
Hanya pustaka standar Python. Keputusan aturan: docs/epics/EPIC-096-pemetaan-data-karyawan-hr.md.
"""
import argparse
import csv
import datetime as dt
import json
import os
import re
import sys
import zipfile
from collections import Counter, defaultdict
from dataclasses import dataclass, field
from xml.etree import ElementTree as ET
from xml.sax.saxutils import escape

ACTOR = 'Ops (Claude Code atas persetujuan user)'
REASON = 'EPIC-096: data kerja mengikuti file HR 09-11'

BRANCH_ALIAS = {'headquarter': 'MIZU-BANDUNG', 'warehouse': 'ODOO-WAREHOUSE-PADDY'}
DEPT_ALIAS = {'operations / warehouse': 'warehouse', 'operations / scm': 'scm'}
STATUS = {'pkwt': 'contract', 'pkwtt': 'permanent', 'probation': 'probation', 'freelance': 'freelance',
          'daily worker': 'daily_worker', 'internship': 'intern'}
# Ejaan approver yang jelas (R3) dan approver ganda yang dipilih menurut toko pengatur jadwal.
APPROVER_ALIAS = {'zaidane': 'Zaidane Gusma', 'azka': 'Azka Hanifah Hamidah', 'risky melianda': 'Risky Melinda',
                  'bungsu tri sutisna': 'Bungsu Tri Sutrisna', 'adhimukti': 'Adhimukti Nurrahman'}
APPROVER_REGION = {'ricky / azka': ('Ricky Pratama Sidik', 'Azka Hanifah Hamidah')}
# Kata yang ditulis berbeda untuk jabatan yang sama.
WORD_ALIAS = {'spv': 'supervisor', 'koordinator': 'coordinator', 'operational': 'operation',
              'assisstant': 'assistant', 'patnership': 'partnership', 'receiveable': 'receivable', 'shopkeper': 'shopkeeper'}
# Nama jabatan file yang maksudnya jabatan HRIS lain (laporan pencocokan 09-11).
POSITION_ALIAS = {'shopkeeper (coordinator)': 'Shopkeeper Staff Coordinator',
                  'shopkeeper (freelance)': 'Shopkeeper Staff - Freelance',
                  'shopkeeper freelance (bandung)': 'Shopkeeper Freelance - Bandung Area',
                  'purchasing staff (local & import)': 'Purchasing Staff',
                  'office support': 'Store Support', 'admin logistic': 'Logistics Admin Staff',
                  'general affair supervisor': 'GA Supervisor', 'affiliator specialist staff': 'Affiliate Specialist',
                  'mini rolling shopkeeper staff': 'Mini Rolling Staff', 'human resource intern': 'HR Intern'}
LEVELS = {'freelance': 'Staff'}
# Ejaan judul jabatan baru dirapikan sebelum masuk master (R2).
TITLE_FIX = {'Patnership': 'Partnership', 'Assisstant': 'Assistant', 'SPV': 'Supervisor', 'Spv': 'Supervisor'}
POSITION_ALIAS_KEY = {}  # diisi setelah title_key terdefinisi


def norm(s):
    return re.sub(r'\s+', ' ', str(s or '').strip()).lower()


def clean(s):
    return re.sub(r'\s+', ' ', str(s or '').strip())


def nik_text(v):
    if isinstance(v, float) and v.is_integer():
        v = int(v)
    s = clean(v)
    return s[:-2] if re.fullmatch(r'\d+\.0', s) else s


def nip_key(v):
    return re.sub(r'\D', '', nik_text(v))


def iso_date(v):
    if isinstance(v, (dt.date, dt.datetime)):
        return v.strftime('%Y-%m-%d')
    s = clean(v)[:10]
    try:
        return dt.date.fromisoformat(s).isoformat()
    except ValueError:
        return None


@dataclass
class Change:
    employee_id: str
    nip: str
    name: str
    field: str
    old: object
    new: object
    old_label: str = ''
    new_label: str = ''


@dataclass
class Skip:
    sheet: str
    row: int
    nik: str
    name: str
    reason: str


@dataclass
class Plan:
    changes: list = field(default_factory=list)
    skipped: list = field(default_factory=list)
    new_positions: list = field(default_factory=list)
    placement_from: dict = field(default_factory=dict)
    roster_after_move: list = field(default_factory=list)
    matched: dict = field(default_factory=dict)
    not_in_file: list = field(default_factory=list)


class Masters:
    def __init__(self, snap):
        self.emps = snap['employees']
        self.by_id = {e['id']: e for e in self.emps}
        self.by_nip = defaultdict(list)
        self.by_exact_nip = {}
        self.by_name = defaultdict(list)
        for e in self.emps:
            if nip_key(e['nip']):
                self.by_nip[nip_key(e['nip'])].append(e)
            self.by_exact_nip[clean(e['nip'])] = e
            self.by_name[norm(e['full_name'])].append(e)
        self.branches = {b['code'] for b in snap['branches'] if b['active']}
        self.codes = {c.replace('ODOO-', '') for c in self.branches} | self.branches
        self.depts = {norm(d['name']): d['name'] for d in snap['departments'] if d['active']}
        self.positions = defaultdict(list)
        for p in snap['positions']:
            if p['active']:
                self.positions[title_key(p['title'])].append(p['title'])
        self.delegate = defaultdict(set)
        for d in snap['delegate_branches']:
            self.delegate[d['employee_id']].add(d['code'])

    def active_named(self, name):
        found = [e for e in self.by_name.get(norm(name), []) if e['is_active']]
        return found[0] if len(found) == 1 else None


def title_key(t):
    return ' '.join(WORD_ALIAS.get(w, w) for w in norm(re.sub(r'[()]', ' ', str(t or ''))).split(' '))


def same_title(a, b):
    """Sama setelah ejaan disamakan, termasuk beda akhiran " Staff" saja."""
    ka, kb = title_key(a), title_key(b)
    return bool(ka) and (ka == kb or ka + ' staff' == kb or kb + ' staff' == ka)


POSITION_ALIAS_KEY.update({title_key(k): v for k, v in POSITION_ALIAS.items()})


def strip_codes(title, codes):
    words = clean(title).split(' ')
    while len(words) > 1 and (words[0].upper() in codes or words[0] == '&'):
        words = words[1:]
    if len(words) > 1 and words[-1].upper() in codes:
        words = words[:-1]
    return ' '.join(words)


def match(m, r):
    """(karyawan, cara) atau (None, alasan)."""
    nk, name = nip_key(r.get('NIK Karyawan')), r.get('Nama Lengkap')
    cands = m.by_nip.get(nk, []) if nk else []
    e = cands[0] if len(cands) == 1 else None
    if e is not None and (not e['is_active'] or norm(e['full_name']) != norm(name)):
        alt = m.active_named(name)
        if alt is not None and alt['id'] != e['id']:
            return alt, 'nama'
    if e is not None:
        return e, 'nip'
    same = m.by_name.get(norm(name), [])
    act = [x for x in same if x['is_active']]
    if len(same) == 1:
        return same[0], 'nama'
    if len(act) == 1:
        return act[0], 'nama'
    return None, 'tidak ada di HRIS' if not same else f'nama ganda di HRIS ({len(same)})'


def desired(m, r, e, new_positions):
    """Nilai yang diminta file per field, dan isu per field yang tidak bisa dipastikan."""
    want, issues = {}, []
    raw = clean(r.get('Branch'))
    code = BRANCH_ALIAS.get(norm(raw)) or next((c for c in ('ODOO-' + raw.upper(), raw.upper()) if c in m.branches), None)
    if raw and code is None:
        issues.append(f"Branch '{raw}' tidak ada di master toko")
    elif code:
        want['branch'] = code
    dname = norm(r.get('Departemen'))
    dname = DEPT_ALIAS.get(dname, dname)
    dept = m.depts.get(dname)
    if dname and dept is None:
        issues.append(f"Departemen '{r.get('Departemen')}' tidak ada di master")
    elif dept:
        want['department'] = dept
    title = strip_codes(r.get('Posisi'), m.codes)
    if title:
        title = POSITION_ALIAS.get(norm(title)) or POSITION_ALIAS_KEY.get(title_key(title), title)
        found = m.positions.get(title_key(title), [])
        exact = [t for t in found if norm(t) == norm(title)]
        if same_title(title, e.get('position')):
            want['position'] = e['position']
        elif len(found) == 1 or exact:
            want['position'] = (exact or found)[0]
        elif found:
            issues.append(f"Posisi '{title}' cocok dengan lebih dari satu jabatan master")
        else:
            level = clean(r.get('Level Jabatan'))
            tidy = ' '.join(TITLE_FIX.get(w, w) for w in title.split(' '))
            new_positions.setdefault(title_key(title), (tidy, LEVELS.get(norm(level), level or 'Staff'), dept or ''))
            want['position'] = new_positions[title_key(title)][0]
    st = norm(r.get('Status Karyawan / Tipe Kontrak'))
    if st:
        if st in STATUS:
            want['employment_status'] = STATUS[st]
        else:
            issues.append(f"Status '{r.get('Status Karyawan / Tipe Kontrak')}' tidak dikenal")
    jd = r.get('Join Date')
    if jd not in (None, ''):
        if iso_date(jd):
            want['join_date'] = iso_date(jd)
        else:
            issues.append(f"Join date '{jd}' tidak terbaca")
    ap = clean(r.get('Approver di HRIS'))
    if ap:
        boss = None
        if norm(ap) in APPROVER_REGION:
            for name in APPROVER_REGION[norm(ap)]:
                b = m.active_named(name)
                if b is not None and code in m.delegate.get(b['id'], set()):
                    boss = b
        else:
            boss = m.active_named(APPROVER_ALIAS.get(norm(ap), ap))
        if boss is None or boss['id'] == e['id']:
            issues.append(f"approver tidak pasti: '{ap}'")
        else:
            want['reporting_to'] = boss['id']
    nk = nik_text(r.get('NIK Karyawan'))
    if nip_key(nk) and nip_key(nk) != nip_key(e['nip']):
        want['nip'] = nk
    return want, issues


def plan(hr_rows, snap, today):
    m = Masters(snap)
    p = Plan()
    new_positions = {}
    per_emp = defaultdict(list)
    for r in hr_rows:
        nik, name = nik_text(r.get('NIK Karyawan')), clean(r.get('Nama Lengkap'))
        e, how = match(m, r)
        if e is None:
            p.skipped.append(Skip(r['_sheet'], r['_row'], nik, name, how))
            continue
        if not e['is_active']:
            p.skipped.append(Skip(r['_sheet'], r['_row'], nik, name, 'nonaktif di HRIS'))
            p.matched[e['id']] = how
            continue
        p.matched[e['id']] = how
        want, issues = desired(m, r, e, new_positions)
        per_emp[e['id']].append((r, want, issues))
    wanted = {}
    for eid, items in per_emp.items():
        if len({json.dumps(w, sort_keys=True) for _, w, _ in items}) > 1:
            for r, _, _ in items:
                p.skipped.append(Skip(r['_sheet'], r['_row'], nik_text(r.get('NIK Karyawan')), clean(r.get('Nama Lengkap')),
                                      'muncul lebih dari sekali di file dengan isi berbeda'))
            continue
        r, want, issues = items[0]
        for i in issues:
            p.skipped.append(Skip(r['_sheet'], r['_row'], nik_text(r.get('NIK Karyawan')), clean(r.get('Nama Lengkap')), i))
        wanted[eid] = (r, want)
    # NIP: target tidak boleh dipakai karyawan lain yang NIP-nya tetap; dua karyawan tidak boleh ke NIP yang sama.
    moving = {eid for eid, (_, w) in wanted.items() if 'nip' in w}
    targets = Counter(w['nip'] for _, w in wanted.values() if 'nip' in w)
    for eid, (r, w) in wanted.items():
        if 'nip' not in w:
            continue
        owner = m.by_exact_nip.get(w['nip'])
        if owner is None or owner['id'] == eid:
            owner = next((o for o in m.by_nip.get(nip_key(w['nip']), []) if o['id'] != eid), owner)
        reason = None
        if targets[w['nip']] > 1:
            reason = f"NIP {w['nip']} diminta lebih dari satu karyawan"
        elif owner is not None and owner['id'] != eid and owner['id'] not in moving:
            reason = f"NIP {w['nip']} dipakai {owner['full_name']}" + ('' if owner['is_active'] else ' (nonaktif)')
        if reason:
            p.skipped.append(Skip(r['_sheet'], r['_row'], nik_text(r.get('NIK Karyawan')), clean(r.get('Nama Lengkap')), reason))
            del w['nip']
    used_new = set()
    for eid, (r, w) in sorted(wanted.items(), key=lambda kv: m.by_id[kv[0]]['nip'] or ''):
        e = m.by_id[eid]
        cur = {'branch': e.get('branch_code'), 'department': e.get('department'), 'position': e.get('position'),
               'employment_status': e.get('employment_status'), 'join_date': e.get('join_date'),
               'reporting_to': e.get('reporting_to'), 'nip': e.get('nip')}
        for f in ('branch', 'department', 'position', 'reporting_to', 'employment_status', 'join_date', 'nip'):
            if f not in w or w[f] == cur[f]:
                continue
            c = Change(eid, e['nip'], e['full_name'], f, cur[f], w[f], str(cur[f] or '-'), str(w[f]))
            if f == 'reporting_to':
                c.old_label = m.by_id[cur[f]]['full_name'] if cur[f] in m.by_id else '-'
                c.new_label = m.by_id[w[f]]['full_name']
            if f == 'position' and title_key(w[f]) in new_positions:
                used_new.add(title_key(w[f]))
            if f == 'branch':
                p.placement_from[eid] = today
                if e.get('roster_last') and e['roster_last'] >= today:
                    p.roster_after_move.append((eid, e['full_name'], e['roster_last']))
            p.changes.append(c)
    p.new_positions = [new_positions[k] for k in sorted(used_new)]
    seen = set(p.matched)
    p.not_in_file = [e for e in m.emps if e['is_active'] and e['id'] not in seen]
    return p


# --- xlsx (pustaka standar) -------------------------------------------------

NS = {'m': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main',
      'r': 'http://schemas.openxmlformats.org/officeDocument/2006/relationships',
      'rel': 'http://schemas.openxmlformats.org/package/2006/relationships'}
DATE_FMT_IDS = set(range(14, 23)) | {45, 46, 47}


def _col(ref):
    n = 0
    for ch in re.match(r'[A-Z]+', ref).group(0):
        n = n * 26 + ord(ch) - 64
    return n - 1


def read_xlsx_sheets(path):
    """{nama sheet: [[sel sebagai teks]]}. Sel tanggal → YYYY-MM-DD, bilangan bulat → tanpa .0."""
    with zipfile.ZipFile(path) as z:
        names = set(z.namelist())
        shared = []
        if 'xl/sharedStrings.xml' in names:
            for si in ET.fromstring(z.read('xl/sharedStrings.xml')).findall('m:si', NS):
                shared.append(''.join(t.text or '' for t in si.iter('{%s}t' % NS['m'])))
        date_styles = set()
        if 'xl/styles.xml' in names:
            st = ET.fromstring(z.read('xl/styles.xml'))
            custom = {int(f.get('numFmtId')): f.get('formatCode', '') for f in st.findall('m:numFmts/m:numFmt', NS)}
            for i, xf in enumerate(st.findall('m:cellXfs/m:xf', NS)):
                fid = int(xf.get('numFmtId', 0))
                code = re.sub(r'"[^"]*"|\[[^\]]*\]', '', custom.get(fid, '')).lower()
                if fid in DATE_FMT_IDS or (fid in custom and re.search(r'[dy]', code)):
                    date_styles.add(i)
        wb = ET.fromstring(z.read('xl/workbook.xml'))
        rels = {r.get('Id'): r.get('Target') for r in ET.fromstring(z.read('xl/_rels/workbook.xml.rels')).findall('rel:Relationship', NS)}
        out = {}
        for s in wb.findall('m:sheets/m:sheet', NS):
            target = rels[s.get('{%s}id' % NS['r'])].lstrip('/')
            target = target if target.startswith('xl/') else 'xl/' + target
            rows = []
            for row in ET.fromstring(z.read(target)).findall('m:sheetData/m:row', NS):
                cells = {}
                for c in row.findall('m:c', NS):
                    t, v = c.get('t'), c.find('m:v', NS)
                    if t == 'inlineStr':
                        val = ''.join(x.text or '' for x in c.iter('{%s}t' % NS['m']))
                    elif v is None or v.text is None:
                        val = ''
                    elif t == 's':
                        val = shared[int(v.text)]
                    elif t in ('str', 'b', 'e'):
                        val = v.text
                    else:
                        num = float(v.text)
                        if int(c.get('s', 0)) in date_styles:
                            val = (dt.date(1899, 12, 30) + dt.timedelta(days=int(num))).isoformat()
                        else:
                            val = str(int(num)) if num.is_integer() else v.text
                    cells[_col(c.get('r'))] = val
                if cells:
                    rows.append([cells.get(i, '') for i in range(max(cells) + 1)])
                else:
                    rows.append([])
            out[s.get('name')] = rows
    return out


def read_hr(path):
    rows = []
    for name, matrix in read_xlsx_sheets(path).items():
        header = None
        for i, r in enumerate(matrix, start=1):
            if not any(clean(c) for c in r):
                continue
            if header is None:
                header = [clean(h) for h in r]
                continue
            d = {h: (r[j] if j < len(r) else '') for j, h in enumerate(header) if h}
            d['_sheet'], d['_row'] = name, i
            rows.append(d)
    return rows


def write_xlsx(path, sheet, rows):
    """xlsx minimal satu sheet, semua sel teks (inlineStr)."""
    def cell(ci, ri, v):
        ref = ''
        n = ci + 1
        while n:
            n, rem = divmod(n - 1, 26)
            ref = chr(65 + rem) + ref
        return f'<c r="{ref}{ri}" t="inlineStr"><is><t xml:space="preserve">{escape(str(v))}</t></is></c>'
    body = ''.join(f'<row r="{ri}">' + ''.join(cell(ci, ri, v) for ci, v in enumerate(r) if v not in (None, '')) + '</row>'
                   for ri, r in enumerate(rows, start=1))
    files = {
        '[Content_Types].xml': '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>',
        '_rels/.rels': '<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
        'xl/workbook.xml': f'<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="{NS["m"]}" xmlns:r="{NS["r"]}"><sheets><sheet name="{escape(sheet)}" sheetId="1" r:id="rId1"/></sheets></workbook>',
        'xl/_rels/workbook.xml.rels': '<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>',
        'xl/worksheets/sheet1.xml': f'<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="{NS["m"]}"><sheetData>{body}</sheetData></worksheet>',
    }
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED) as z:
        for n, data in files.items():
            z.writestr(n, data)


# --- keluaran ---------------------------------------------------------------

PEOPLE_FIELDS = ('reporting_to', 'employment_status', 'join_date', 'nip')
TEMPLATE_FIELDS = ('branch', 'department', 'position')
COL_NIP, COL_BRANCH, COL_FROM, COL_DEPT, COL_POS = 0, 7, 16, 17, 18


def lit(v):
    return 'NULL' if v is None else "'" + str(v).replace("'", "''") + "'"


def sql_positions(new_positions):
    """01: jabatan baru (R2), idempoten; gagal bila setelahnya ada yang belum ada."""
    out = ['-- EPIC-096 01: jabatan baru dari file HR. Jalankan: psql -X -1 -v ON_ERROR_STOP=1 -f 01-jabatan-baru.sql',
           'SET LOCAL search_path = hris, public;']
    for title, level, dept in new_positions:
        out.append(f"""INSERT INTO hris.positions (brand_id, title, level, department, is_active)
SELECT (SELECT brand_id FROM hris.positions WHERE coalesce(is_active, true) GROUP BY brand_id ORDER BY count(*) DESC LIMIT 1),
       {lit(title)}, {lit(level)}, {lit(dept or None)}, true
WHERE NOT EXISTS (SELECT 1 FROM hris.positions WHERE coalesce(is_active, true) AND lower(btrim(title)) = lower({lit(title)}));""")
    titles = ', '.join(f'lower({lit(t)})' for t, _, _ in new_positions) or "''"
    out.append(f"""DO $$ DECLARE n int; BEGIN
  SELECT count(DISTINCT lower(btrim(title))) INTO n FROM hris.positions
   WHERE coalesce(is_active, true) AND lower(btrim(title)) IN ({titles});
  IF n <> {len(new_positions)} THEN RAISE EXCEPTION 'jabatan baru: % dari {len(new_positions)} ada', n; END IF;
END $$;""")
    return '\n'.join(out) + '\n'


def _check(what):
    return f"  GET DIAGNOSTICS n = ROW_COUNT;\n  IF n <> 1 THEN RAISE EXCEPTION 'data berubah sejak snapshot: {what}'; END IF;"


def sql_people(p):
    """03: atasan, status, join date, NIP. Guard nilai lama per kolom; NIP dua fase (prefix sementara)."""
    per = defaultdict(dict)
    names, nips = {}, {}
    for c in p.changes:
        if c.field in PEOPLE_FIELDS:
            per[c.employee_id][c.field] = c
            names[c.employee_id], nips[c.employee_id] = c.name, c.nip
    body = []
    for eid in sorted(per, key=lambda i: nips[i] or ''):
        if 'nip' in per[eid]:
            body.append(f"  UPDATE hris.employees SET nip = 'tmp096-' || id WHERE id = {lit(eid)} AND nip IS NOT DISTINCT FROM {lit(per[eid]['nip'].old)};")
            body.append(_check(f"NIP {nips[eid]}"))
    for eid in sorted(per, key=lambda i: nips[i] or ''):
        cs = per[eid]
        sets, guards, before, after = [], [], {}, {}
        for f, c in cs.items():
            cast = '::uuid' if f == 'reporting_to' else '::date' if f == 'join_date' else ''
            sets.append(f'{f} = {lit(c.new)}{cast}')
            old = f"'tmp096-' || id" if f == 'nip' else f'{lit(c.old)}{cast if c.old is not None else ""}'
            guards.append(f'{f} IS NOT DISTINCT FROM {old}')
            before[f], after[f] = c.old, c.new
        body.append(f"  UPDATE hris.employees SET {', '.join(sets)}, updated_at = now()\n   WHERE id = {lit(eid)} AND {' AND '.join(guards)};")
        body.append(_check(names[eid].replace("'", "''")))
        body.append(f"  INSERT INTO audit.audit_log (actor_id, actor_name, action, entity, entity_id, entity_label, before, after, reason)\n"
                    f"  VALUES (NULL, {lit(ACTOR)}, 'employee.update', 'hris.employees', {lit(eid)}, {lit(names[eid])}, "
                    f"{lit(json.dumps(before, sort_keys=True))}::jsonb, {lit(json.dumps(after, sort_keys=True))}::jsonb, {lit(REASON)});")
    return ('-- EPIC-096 03: atasan, status kontrak, join date, NIP. Jalankan: psql -X -1 -v ON_ERROR_STOP=1 -f 03-data-karyawan.sql\n'
            'SET LOCAL search_path = hris, public;\nDO $$ DECLARE n int; BEGIN\n' + '\n'.join(body) + '\nEND $$;\n')


def fill_template(matrix, p):
    """02: sheet Data template EPIC-086 — header dari template, satu baris per karyawan dengan perubahan
    penempatan/departemen/jabatan (pratinjau EPIC-086 menerima NIP aktif mana pun)."""
    header = matrix[0]
    want, names = defaultdict(dict), {}
    for c in p.changes:
        if c.field in TEMPLATE_FIELDS:
            want[clean(c.nip)][c.field] = c.new
            names[clean(c.nip)] = c.name
    out = [header]
    for nip in sorted(want):
        w = want[nip]
        r = [''] * len(header)
        r[COL_NIP], r[1] = nip, names[nip]
        if 'branch' in w:
            r[COL_BRANCH], r[COL_FROM] = w['branch'], next(iter(p.placement_from.values()))
        r[COL_DEPT] = w.get('department', '')
        r[COL_POS] = w.get('position', '')
        out.append(r)
    return out


def write_reports(out_dir, p):
    labels = {'branch': 'Penempatan', 'department': 'Departemen', 'position': 'Jabatan', 'reporting_to': 'Atasan',
              'employment_status': 'Status kontrak', 'join_date': 'Join date', 'nip': 'NIP'}
    with open(os.path.join(out_dir, 'rencana.csv'), 'w', newline='', encoding='utf-8') as f:
        w = csv.writer(f)
        w.writerow(['jenis', 'nip', 'nama', 'kolom', 'nilai_lama', 'nilai_baru', 'sheet', 'baris', 'alasan'])
        for c in p.changes:
            w.writerow(['ubah', c.nip, c.name, labels[c.field], c.old_label, c.new_label, '', '', ''])
        for s in p.skipped:
            w.writerow(['lewati', s.nik, s.name, '', '', '', s.sheet, s.row, s.reason])
        for t, lv, d in p.new_positions:
            w.writerow(['jabatan_baru', '', t, 'Jabatan', '', f'{t} ({lv}, {d})', '', '', ''])
        for eid, name, last in p.roster_after_move:
            w.writerow(['roster_toko_lama', '', name, 'Penempatan', '', '', '', '', f'roster sampai {last} tetap di toko lama'])
        for e in p.not_in_file:
            w.writerow(['tidak_di_file', e['nip'], e['full_name'], '', '', '', '', '', 'aktif di HRIS, tidak ada di file'])
    reasons = Counter(re.sub(r"'[^']*'|NIP \S+ dipakai .*|\(\d+\)", '…', s.reason) for s in p.skipped)
    lines = ['Perubahan per kolom:'] + [f'  {labels[k]}: {v}' for k, v in Counter(c.field for c in p.changes).most_common()]
    lines += [f'Karyawan berubah: {len({c.employee_id for c in p.changes})}', f'Jabatan baru: {len(p.new_positions)}',
              f'Roster sesudah pindah toko: {len(p.roster_after_move)}', f'Aktif HRIS tidak di file: {len(p.not_in_file)}',
              f'Dilewati: {len(p.skipped)}'] + [f'  {k}: {v}' for k, v in reasons.most_common()]
    text = '\n'.join(lines) + '\n'
    with open(os.path.join(out_dir, 'ringkasan.txt'), 'w', encoding='utf-8') as f:
        f.write(text)
    return text


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--hr', required=True)
    ap.add_argument('--snapshot', required=True)
    ap.add_argument('--template', help='template EPIC-086 dari data-kerja.sh template (untuk 02)')
    ap.add_argument('--today', required=True)
    ap.add_argument('--out', required=True)
    a = ap.parse_args(argv)
    os.makedirs(a.out, exist_ok=True)
    with open(a.snapshot, encoding='utf-8') as f:
        snap = json.load(f)
    p = plan(read_hr(a.hr), snap, a.today)
    with open(os.path.join(a.out, '01-jabatan-baru.sql'), 'w', encoding='utf-8') as f:
        f.write(sql_positions(p.new_positions))
    with open(os.path.join(a.out, '03-data-karyawan.sql'), 'w', encoding='utf-8') as f:
        f.write(sql_people(p))
    if a.template:
        write_xlsx(os.path.join(a.out, '02-data-kerja.xlsx'), 'Data', fill_template(read_xlsx_sheets(a.template)['Data'], p))
    sys.stdout.write(write_reports(a.out, p))
    return 0


if __name__ == '__main__':
    sys.exit(main())

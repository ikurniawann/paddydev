#!/usr/bin/env python3
"""Tes generate.py (EPIC-096): python3 -m unittest -v test_generate (dari folder ini)."""
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import generate as g  # noqa: E402

TODAY = '2026-11-10'


def emp(i, nip, name, **kw):
    e = {'id': f'e{i}', 'nip': nip, 'full_name': name, 'is_active': True, 'join_date': '2025-01-01',
         'employment_status': 'contract', 'schedule_type': 'shift', 'department_id': 'd-retail',
         'department': 'Retail Operational', 'job_title_id': 'p-shop', 'position': 'Shopkeeper Staff',
         'reporting_to': None, 'branch_code': 'ODOO-PPGA', 'roster_last': None}
    e.update(kw)
    return e


def snap(*emps):
    return {
        'employees': [
            emp(90, '202405059', 'Ricky Pratama Sidik', position='Store Supervisor - Jawa Barat 1'),
            emp(91, '202501138', 'Azka Hanifah Hamidah', position='Store Supervisor - Jawa Barat 2'),
            emp(92, '202608631', 'Zaidane Gusma', department='Warehouse', department_id='d-wh'),
            *emps],
        'branches': [{'code': c, 'active': True} for c in
                     ('MIZU-BANDUNG', 'ODOO-WAREHOUSE-PADDY', 'ODOO-PPGA', 'ODOO-PPBA', 'ODOO-PPMR', 'ODOO-PBSA', 'TWEM')],
        'departments': [{'id': 'd-retail', 'name': 'Retail Operational', 'active': True},
                        {'id': 'd-wh', 'name': 'Warehouse', 'active': True},
                        {'id': 'd-scm', 'name': 'SCM', 'active': True}],
        'positions': [{'id': 'p-shop', 'title': 'Shopkeeper Staff', 'level': 'Staff', 'department': 'Retail Operational', 'active': True},
                      {'id': 'p-perf', 'title': 'Perfumer Staff', 'level': 'Staff', 'department': 'Retail Operational', 'active': True},
                      {'id': 'p-pj', 'title': 'Penjahit', 'level': 'Staff', 'department': 'Retail Operational', 'active': True},
                      {'id': 'p-kol', 'title': 'KOL Specialist Supervisor', 'level': 'Supervisor', 'department': 'Marketing', 'active': True},
                      {'id': 'p-ar1', 'title': 'Account Receivable Staff', 'level': 'Staff', 'department': 'Finance & Accounting', 'active': True}],
        'delegate_branches': [{'employee_id': 'e90', 'code': 'ODOO-PPGA'}, {'employee_id': 'e91', 'code': 'ODOO-PPBA'}],
    }


def row(nik, name, sheet='PKWTPKWTT', line=2, **kw):
    r = {'NIK Karyawan': nik, 'Nama Lengkap': name, 'Join Date': '2025-01-01', 'Branch': 'PPGA',
         'Departemen': 'Retail Operational', 'Level Jabatan': 'Staff', 'Posisi': 'PPGA Shopkeeper Staff',
         'Approver di HRIS': '', 'Status Karyawan / Tipe Kontrak': 'PKWT', '_sheet': sheet, '_row': line}
    r.update(kw)
    return r


def changes(p, field=None):
    return {(c.employee_id, c.field): c.new for c in p.changes if field is None or c.field == field}


def skips(p):
    return [s.reason for s in p.skipped]


class PlanTest(unittest.TestCase):
    def test_match_by_nip_then_name(self):
        s = snap(emp(1, '201602001', 'Katrin Marwina'), emp(2, '202505268', 'Raya Juniar'),
                 emp(3, 'EMP-2026-00005', 'Rizki Givary'), emp(4, '1', 'Sama'), emp(5, '2', 'Sama'))
        p = g.plan([row(201602001.0, 'Katrin Marwina'), row('2025-05-268', 'Raya Juniar', line=3),
                    row(202610658.0, 'Rizki Givary', line=4), row(999.0, 'Sama', line=5)], s, TODAY)
        self.assertEqual(p.matched['e1'], 'nip')
        self.assertEqual(p.matched['e2'], 'nip')
        self.assertEqual(p.matched['e3'], 'nama')
        self.assertIn('nama ganda di HRIS (2)', skips(p))

    def test_inactive_match_prefers_active_by_name(self):
        s = snap(emp(1, '202609638', 'Dominic B Nababan', is_active=False), emp(2, '202608638', 'Dominic B Nababan'))
        p = g.plan([row(202609638.0, 'Dominic B Nababan', Branch='PPMR')], s, TODAY)
        self.assertEqual(changes(p, 'branch'), {('e2', 'branch'): 'ODOO-PPMR'})

    def test_inactive_untouched(self):
        p = g.plan([row(1.0, 'Fitrah', Branch='PPMR')], snap(emp(1, '1', 'Fitrah', is_active=False)), TODAY)
        self.assertEqual(p.changes, [])
        self.assertIn('nonaktif di HRIS', skips(p))

    def test_branch_mapping(self):
        s = snap(emp(1, '1', 'A'), emp(2, '2', 'B'), emp(3, '3', 'C'), emp(4, '4', 'D'), emp(5, '5', 'E'))
        p = g.plan([row(1.0, 'A', Branch='Headquarter'), row(2.0, 'B', Branch='Warehouse', line=3),
                    row(3.0, 'C', Branch='PPBA', line=4), row(4.0, 'D', Branch='TWEM', line=5),
                    row(5.0, 'E', Branch='XXXX', line=6)], s, TODAY)
        b = changes(p, 'branch')
        self.assertEqual(b[('e1', 'branch')], 'MIZU-BANDUNG')
        self.assertEqual(b[('e2', 'branch')], 'ODOO-WAREHOUSE-PADDY')
        self.assertEqual(b[('e3', 'branch')], 'ODOO-PPBA')
        self.assertEqual(b[('e4', 'branch')], 'TWEM')
        self.assertNotIn(('e5', 'branch'), b)
        self.assertIn("Branch 'XXXX' tidak ada di master toko", skips(p))

    def test_position_strip_store_code(self):
        s = snap(emp(1, '1', 'A'), emp(2, '2', 'B', position='Shopkeeper Staff'), emp(3, '3', 'C'), emp(4, '4', 'D'),
                 emp(5, '5', 'E', position='Account Receivable Staff', job_title_id='p-ar1'))
        p = g.plan([row(1.0, 'A', Posisi='PPMR Shopkeeper Staff'), row(2.0, 'B', Posisi='Penjahit PBSA', line=3),
                    row(3.0, 'C', Posisi='KOL Specialist SPV', **{'Level Jabatan': 'Supervisor'}, line=4),
                    row(4.0, 'D', Posisi='Packing Coordinator', Departemen='Operations / Warehouse',
                        **{'Level Jabatan': 'Coordinator'}, line=5),
                    row(5.0, 'E', Posisi='Account Receiveable Staff', line=6)], s, TODAY)
        pos = changes(p, 'position')
        self.assertNotIn(('e1', 'position'), pos)
        self.assertEqual(pos[('e2', 'position')], 'Penjahit')
        self.assertEqual(pos[('e3', 'position')], 'KOL Specialist Supervisor')
        self.assertEqual(pos[('e4', 'position')], 'Packing Coordinator')
        self.assertNotIn(('e5', 'position'), pos)
        self.assertEqual(p.new_positions, [('Packing Coordinator', 'Coordinator', 'Warehouse')])

    def test_status_mapping(self):
        want = {'PKWT': 'contract', 'PKWTT': 'permanent', 'Probation': 'probation', 'Freelance': 'freelance',
                'Daily Worker': 'daily_worker', 'Internship': 'intern'}
        emps = [emp(i, str(i), f'N{i}', employment_status='other') for i in range(1, 7)]
        rows = [row(float(i), f'N{i}', line=i + 1, **{'Status Karyawan / Tipe Kontrak': k}) for i, k in enumerate(want, 1)]
        st = changes(g.plan(rows, snap(*emps), TODAY), 'employment_status')
        self.assertEqual([st[(f'e{i}', 'employment_status')] for i in range(1, 7)], list(want.values()))

    def test_approver_alias_and_region(self):
        s = snap(emp(1, '1', 'A'), emp(2, '2', 'B'), emp(3, '3', 'C'), emp(4, '4', 'D'))
        p = g.plan([row(1.0, 'A', **{'Approver di HRIS': 'Zaidane'}),
                    row(2.0, 'B', Branch='PPGA', line=3, **{'Approver di HRIS': 'Ricky / Azka'}),
                    row(3.0, 'C', Branch='PPBA', line=4, **{'Approver di HRIS': 'Ricky / Azka'}),
                    row(4.0, 'D', Branch='Headquarter', line=5, **{'Approver di HRIS': 'Ricky / Azka'})], s, TODAY)
        a = changes(p, 'reporting_to')
        self.assertEqual(a[('e1', 'reporting_to')], 'e92')
        self.assertEqual(a[('e2', 'reporting_to')], 'e90')
        self.assertEqual(a[('e3', 'reporting_to')], 'e91')
        self.assertNotIn(('e4', 'reporting_to'), a)
        self.assertIn("approver tidak pasti: 'Ricky / Azka'", skips(p))

    def test_duplicate_rows_conflict(self):
        s = snap(emp(1, '1', 'Kayla'), emp(2, '2', 'Sama Isi'))
        p = g.plan([row(1.0, 'Kayla', Branch='PPGA'), row(1.0, 'Kayla', Branch='PPBA', line=3),
                    row(2.0, 'Sama Isi', Branch='PPBA', line=4), row(2.0, 'Sama Isi', Branch='PPBA', line=5)], s, TODAY)
        self.assertNotIn('e1', {c.employee_id for c in p.changes})
        self.assertIn('muncul lebih dari sekali di file dengan isi berbeda', skips(p))
        self.assertEqual(changes(p, 'branch'), {('e2', 'branch'): 'ODOO-PPBA'})

    def test_roster_after_from_reported(self):
        p = g.plan([row(1.0, 'A', Branch='PPBA')], snap(emp(1, '1', 'A', roster_last='2026-11-15')), TODAY)
        self.assertEqual(p.placement_from['e1'], TODAY)
        self.assertEqual(p.roster_after_move, [('e1', 'A', '2026-11-15')])

    def test_no_change_when_equal(self):
        p = g.plan([row(1.0, 'A')], snap(emp(1, '1', 'A')), TODAY)
        self.assertEqual(p.changes, [])

    def test_xlsx_roundtrip(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, 'x.xlsx')
            g.write_xlsx(path, 'Data', [['NIP', 'Nama', 'Tanggal'], ['201602001', 'Katrin', '2026-11-10']])
            self.assertEqual(g.read_xlsx_sheets(path)['Data'], [['NIP', 'Nama', 'Tanggal'], ['201602001', 'Katrin', '2026-11-10']])


class OutputTest(unittest.TestCase):
    def plan_people(self):
        s = snap(emp(1, '101', 'A'), emp(2, '102', 'B'), emp(3, '104', 'C'), emp(4, '105', 'D', employment_status='probation'))
        rows = [row('102', 'A'), row('103', 'B', line=3), row('104', 'C', line=4, **{'Approver di HRIS': 'Zaidane'}),
                row('105', 'D', line=5, **{'Join Date': '2024-02-03'})]
        return g.plan(rows, s, TODAY), s

    def test_positions_sql_idempotent(self):
        sql = g.sql_positions([('Packing Coordinator', 'Coordinator', 'Warehouse')])
        self.assertIn("WHERE NOT EXISTS", sql)
        self.assertIn("lower(btrim(title)) = lower('Packing Coordinator')", sql)
        self.assertIn('RAISE EXCEPTION', sql)

    def test_sql_guards_old_value(self):
        p, s = self.plan_people()
        sql = g.sql_people(p)
        self.assertIn("reporting_to IS NOT DISTINCT FROM NULL", sql)
        self.assertIn("employment_status IS NOT DISTINCT FROM 'probation'", sql)
        self.assertIn("join_date IS NOT DISTINCT FROM '2025-01-01'", sql)
        self.assertGreater(sql.count('GET DIAGNOSTICS'), 0)
        self.assertEqual(sql.count('GET DIAGNOSTICS'), sql.count('RAISE EXCEPTION'))

    def test_nip_swap_two_phase(self):
        p, _ = self.plan_people()
        sql = g.sql_people(p)
        nips = changes(p, 'nip')
        self.assertEqual(nips, {('e1', 'nip'): '102', ('e2', 'nip'): '103'})
        last_tmp = max(sql.index("nip = 'tmp096-' || id WHERE id = 'e1'"), sql.index("nip = 'tmp096-' || id WHERE id = 'e2'"))
        self.assertLess(last_tmp, sql.index("nip = '102'"))
        self.assertLess(last_tmp, sql.index("nip = '103'"))

    def test_nip_taken_by_other_skipped(self):
        s = snap(emp(1, '101', 'A'), emp(2, '102', 'Orang Lain'))
        p = g.plan([row('102', 'A')], s, TODAY)
        self.assertNotIn(('e1', 'nip'), changes(p))
        self.assertIn('NIP 102 dipakai Orang Lain', skips(p))

    def test_audit_one_row_per_employee(self):
        p, _ = self.plan_people()
        sql = g.sql_people(p)
        self.assertEqual(sql.count('INSERT INTO audit.audit_log'), 4)
        self.assertIn('\'{"reporting_to": null}\'::jsonb, \'{"reporting_to": "e92"}\'::jsonb', sql)

    def test_template_fill(self):
        s = snap(emp(1, '1', 'A'), emp(2, '2', 'B'), emp(3, '3', 'C'))
        p = g.plan([row(1.0, 'A', Branch='PPBA'), row(2.0, 'B', line=3, Posisi='Perfumer Staff', Departemen='SCM'),
                    row(3.0, 'C', line=4)], s, TODAY)
        header = ['NIP', 'Nama', 'Departemen Saat Ini', 'Jabatan Saat Ini', 'Penempatan Saat Ini', 'Tipe Saat Ini',
                  'Yang Perlu Dilengkapi', 'Penempatan Utama (Kode Toko)', 'Tipe Jadwal', 'Senin', 'Selasa', 'Rabu',
                  'Kamis', 'Jumat', 'Sabtu', 'Minggu', 'Berlaku Mulai', 'Departemen Baru', 'Jabatan Baru', 'Catatan']
        tpl = [header, ['9', 'Hanya yang belum lengkap'] + [''] * 18]  # template EPIC-086 tidak memuat semua karyawan
        out = g.fill_template(tpl, p)
        self.assertEqual(out[0], header)
        self.assertEqual([r[:2] for r in out[1:]], [['1', 'A']])  # departemen/jabatan lewat 03 (id), bukan template
        self.assertTrue(all(len(r) == len(header) for r in out))
        self.assertEqual((out[1][7], out[1][16], out[1][17], out[1][18]), ('ODOO-PPBA', TODAY, '', ''))
        sql = g.sql_people(p)
        self.assertIn("department_id = 'd-scm'::uuid", sql)
        self.assertIn("job_title_id = 'p-perf'::uuid", sql)
        self.assertIn("job_title_id IS NOT DISTINCT FROM 'p-shop'::uuid", sql)


class TidyTest(unittest.TestCase):
    def test_position_equivalents(self):
        s = snap(emp(1, '1', 'A', position='Store Support'), emp(2, '2', 'B'), emp(3, '3', 'C'), emp(4, '4', 'D'),
                 emp(5, '5', 'E', position='Quality Control Staff'), emp(6, '6', 'F', position='Warehouse Staff'))
        s['positions'] += [{'id': 'p-ss', 'title': 'Store Support', 'level': 'Staff', 'department': 'Retail Operational', 'active': True},
                           {'id': 'p-qc', 'title': 'Quality Control Staff', 'level': 'Staff', 'department': 'SCM', 'active': True},
                           {'id': 'p-la', 'title': 'Logistics Admin Staff', 'level': 'Staff', 'department': 'Warehouse', 'active': True}]
        s['branches'] += [{'code': 'ODOO-PPAY', 'active': True}, {'code': 'ODOO-PLAY', 'active': True}]
        p = g.plan([row(1.0, 'A', Posisi='PPAY & PLAY Office Support'), row(2.0, 'B', line=3, Posisi='PPGA Shopkeper Staff'),
                    row(3.0, 'C', line=4, Posisi='Host Live (Freelance)'), row(4.0, 'D', line=5, Posisi='Host Live Freelance'),
                    row(5.0, 'E', line=6, Posisi='Quality Control'), row(6.0, 'F', line=7, Posisi='Admin Logistic')], s, TODAY)
        pos = changes(p, 'position')
        self.assertEqual(pos[('e1', 'position')], 'Office Support')  # kode toko bertanda & dibuang
        self.assertNotIn(('e2', 'position'), pos)
        self.assertNotIn(('e5', 'position'), pos)
        self.assertEqual(pos[('e6', 'position')], 'Logistics Admin Staff')
        self.assertEqual(pos[('e3', 'position')], pos[('e4', 'position')])
        self.assertEqual(sorted(t for t, _, _ in p.new_positions), ['Host Live (Freelance)', 'Office Support'])

    def test_nip_digits_conflict(self):
        s = snap(emp(1, '2026-06-042', 'Syakira'), emp(2, '202610082', 'Syakira', is_active=False))
        p = g.plan([row('2026-10-082', 'Syakira')], s, TODAY)
        self.assertNotIn(('e1', 'nip'), changes(p))
        self.assertIn('NIP 2026-10-082 dipakai Syakira (nonaktif)', skips(p))

    def test_new_title_spelling_tidied(self):
        s = snap(emp(1, '1', 'A'), emp(2, '2', 'B'))
        p = g.plan([row(1.0, 'A', Posisi='Brand Patnership SPV', **{'Level Jabatan': 'Supervisor'}),
                    row(2.0, 'B', line=3, Posisi='Branding & Marketing Assisstant Manager', **{'Level Jabatan': 'Assistant Manager'})], s, TODAY)
        self.assertEqual(sorted(t for t, _, _ in p.new_positions), ['Brand Partnership Supervisor', 'Branding & Marketing Assistant Manager'])
        self.assertEqual(changes(p, 'position')[('e1', 'position')], 'Brand Partnership Supervisor')


class AmbiguousTest(unittest.TestCase):
    def test_existing_title_beats_alias_and_duplicate_picks_most_used(self):
        s = snap(emp(1, '1', 'Jejen', position='Office Support', job_title_id='p-os'), emp(2, '2', 'Keyla', position=None, job_title_id=None),
                 *[emp(10 + i, f'9{i}', f'X{i}', job_title_id='p-shop2') for i in range(4)])
        s['positions'] += [{'id': 'p-os', 'title': 'Office Support', 'level': 'Staff', 'department': 'HRGA', 'active': True},
                           {'id': 'p-shop2', 'title': 'Shopkeeper Staff', 'level': 'Staff', 'department': 'Retail Operational', 'active': True}]
        p = g.plan([row(1.0, 'Jejen', Posisi='Office Support'), row(2.0, 'Keyla', line=3)], s, TODAY)
        self.assertNotIn(('e1', 'position'), changes(p))
        self.assertIn("job_title_id = 'p-shop2'::uuid", g.sql_people(p))

    def test_new_position_resolved_by_title_in_sql(self):
        p = g.plan([row(1.0, 'A', Posisi='Driver', Departemen='Operations / Warehouse')], snap(emp(1, '1', 'A')), TODAY)
        sql = g.sql_people(p)
        self.assertIn("job_title_id = (SELECT id FROM hris.positions WHERE coalesce(is_active, true) AND lower(btrim(title)) = lower('Driver')", sql)
        self.assertIn("department_id = 'd-wh'::uuid", sql)


if __name__ == '__main__':
    unittest.main()

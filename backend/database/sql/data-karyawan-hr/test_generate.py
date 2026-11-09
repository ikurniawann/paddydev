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


if __name__ == '__main__':
    unittest.main()

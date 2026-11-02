"use client";

import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Combobox } from "@/components/ui/combobox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePayrollEntities } from "@/features/hris/payroll";
import { ArrowLeftIcon } from "@heroicons/react/24/outline";
import { CheckIcon } from "@heroicons/react/24/solid";
import { toast } from "sonner";
import { EMPTY_SALARY_FORM, formToSalaryPayload } from "@/lib/hris/salary-form";
import { useSalaryEmployees } from "../queries";
import { useCreateSalary } from "../mutations";
import { SalaryAmountFields } from "./salary-amount-fields";
import { SalaryRuleFields } from "./salary-rule-fields";
import { PayrollSwitchNote, usePayrollSwitches } from "./payroll-switch-note";

export function NewSalaryPage() {
  const router = useRouter();
  const params = useSearchParams();
  const entitiesQuery = usePayrollEntities();
  const switches = usePayrollSwitches();

  const employeesQuery = useSalaryEmployees();
  const employees = employeesQuery.data ?? [];
  // NIP ditaruh di `description` agar karyawan bisa dicari lewat nama ATAU NIP.
  const employeeOptions = employees.map((emp) => ({
    value: emp.id,
    label: emp.full_name,
    description: emp.nip ?? undefined,
  }));
  const createMutation = useCreateSalary();
  const saving = createMutation.isPending;

  const [formData, setFormData] = useState({
    ...EMPTY_SALARY_FORM,
    employee_id: "",
    company_id: params.get("entity") ?? "",
  });

  async function handleSave() {
    if (!formData.employee_id || !formData.company_id || !formData.base_salary) {
      toast.error("Karyawan, entitas pembayar, dan gaji pokok wajib diisi");
      return;
    }

    try {
      await createMutation.mutateAsync({
        ...formToSalaryPayload(formData),
        employee_id: formData.employee_id,
        company_id: formData.company_id,
        notes: undefined,
      });

      toast.success("Salary structure berhasil dibuat");
      setTimeout(() => {
        router.push(`/dashboard/hris/salary?entity=${formData.company_id}`);
      }, 1000);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Terjadi kesalahan");
    }
  }

  return (
    <div className="container mx-auto py-8 max-w-4xl">
      {/* Header */}
      <div className="mb-6 flex items-center gap-4">
        <Button
          variant="outline"
          onClick={() => router.push("/dashboard/hris/salary")}
        >
          <ArrowLeftIcon className="w-4 h-4 mr-2" />
          Kembali
        </Button>
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Tambah Salary Structure</h1>
          <p className="text-sm text-gray-500">Tambahkan struktur gaji baru untuk karyawan</p>
        </div>
      </div>

      {/* Salary Form */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-lg">Informasi Salary</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Employee Selection */}
          <div>
            <Label htmlFor="employee_id">Karyawan *</Label>
            <div className="mt-1">
              <Combobox
                options={employeeOptions}
                value={formData.employee_id}
                onChange={(value) => setFormData({ ...formData, employee_id: value })}
                placeholder={
                  employeesQuery.isLoading ? "Memuat karyawan…" : "Pilih karyawan"
                }
                searchPlaceholder="Cari nama atau NIP…"
                emptyMessage="Karyawan tidak ditemukan"
                disabled={employeesQuery.isLoading}
                allowClear
              />
            </div>
          </div>

          <div>
            <Label htmlFor="company_id">Entitas pembayar *</Label>
            <div className="mt-1">
              <Select value={formData.company_id} onValueChange={(v) => setFormData({ ...formData, company_id: String(v) })}>
                <SelectTrigger id="company_id">
                  <SelectValue placeholder={entitiesQuery.isLoading ? "Memuat entitas…" : "Pilih entitas"} />
                </SelectTrigger>
                <SelectContent>
                  {(entitiesQuery.data ?? []).map((e) => (
                    <SelectItem key={e.id} value={e.id}>
                      {e.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              Karyawan yang digaji beberapa entitas dibuatkan satu struktur gaji per entitas (mis. A Rp1jt, B Rp1jt);
              tiap entitas menerbitkan slip sendiri.
            </p>
          </div>

          <SalaryAmountFields
            values={formData}
            onChange={(field, value) => setFormData((prev) => ({ ...prev, [field]: value }))}
            baseSalaryRequired
          />

          <SalaryRuleFields
            values={formData}
            onChange={(field, value) => setFormData((prev) => ({ ...prev, [field]: value }))}
          />

          {/* Tax & Benefits */}
          <div>
            <h3 className="text-sm font-semibold text-gray-700 mb-3">Tax & Benefits</h3>
            <div className="space-y-3">
              <div>
                <Label htmlFor="ptkp_status">Status PTKP</Label>
                <p className="text-xs text-gray-500 mb-1">Penghasilan Tidak Kena Pajak</p>
                <select
                  id="ptkp_status"
                  value={formData.ptkp_status}
                  onChange={(e) => setFormData({ ...formData, ptkp_status: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-pink-500"
                >
                  <option value="TK/0">TK/0 (Tidak Kawin, 0 tanggungan)</option>
                  <option value="TK/1">TK/1 (Tidak Kawin, 1 tanggungan)</option>
                  <option value="TK/2">TK/2 (Tidak Kawin, 2 tanggungan)</option>
                  <option value="TK/3">TK/3 (Tidak Kawin, 3 tanggungan)</option>
                  <option value="K/0">K/0 (Kawin, 0 tanggungan)</option>
                  <option value="K/1">K/1 (Kawin, 1 tanggungan)</option>
                  <option value="K/2">K/2 (Kawin, 2 tanggungan)</option>
                  <option value="K/3">K/3 (Kawin, 3 tanggungan)</option>
                </select>
              </div>

              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <input
                  type="checkbox"
                  id="is_taxable"
                  checked={formData.is_taxable}
                  onChange={(e) => setFormData({ ...formData, is_taxable: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
                <Label htmlFor="is_taxable" className="text-sm font-medium text-gray-700 cursor-pointer">
                  Subjek Pajak (Kena PPh 21)
                </Label>
                <PayrollSwitchNote show={switches.pph21Off} />
              </div>

              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <input
                  type="checkbox"
                  id="bpjs_tk"
                  checked={formData.bpjs_tk_enrolled}
                  onChange={(e) => setFormData({ ...formData, bpjs_tk_enrolled: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
                <Label htmlFor="bpjs_tk" className="text-sm font-medium text-gray-700 cursor-pointer">
                  BPJS Ketenagakerjaan (JHT, JP, JKK, JKM)
                </Label>
                <PayrollSwitchNote show={switches.bpjsTkOff} />
              </div>

              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <input
                  type="checkbox"
                  id="bpjs_kes"
                  checked={formData.bpjs_kes_enrolled}
                  onChange={(e) => setFormData({ ...formData, bpjs_kes_enrolled: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
                <Label htmlFor="bpjs_kes" className="text-sm font-medium text-gray-700 cursor-pointer">
                  BPJS Kesehatan
                </Label>
                <PayrollSwitchNote show={switches.bpjsKesOff} />
              </div>

              <div className="flex items-center space-x-2">
                <input
                  type="checkbox"
                  id="tapera"
                  checked={formData.tapera_enrolled}
                  onChange={(e) => setFormData({ ...formData, tapera_enrolled: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
                <Label htmlFor="tapera" className="text-sm font-medium text-gray-700 cursor-pointer">
                  Tapera (Tabungan Perumahan Rakyat)
                </Label>
              </div>
            </div>
          </div>

          {/* Save Button */}
          <div className="flex justify-end gap-2 pt-4 border-t">
            <Button
              variant="outline"
              onClick={() => router.push("/dashboard/hris/salary")}
            >
              Batal
            </Button>
            <Button
              onClick={handleSave}
              disabled={saving}
              className="bg-pink-600 hover:bg-pink-700"
            >
              {saving ? (
                "Menyimpan..."
              ) : (
                <>
                  <CheckIcon className="w-4 h-4 mr-2" />
                  Simpan
                </>
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

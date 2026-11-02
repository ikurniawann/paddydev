"use client";

import { useState, useEffect, use } from "react";
import { useRouter } from "next/navigation";
import { useSalary } from "../queries";
import { useUpdateSalary } from "../mutations";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ArrowLeftIcon } from "@heroicons/react/24/outline";
import { CheckIcon } from "@heroicons/react/24/solid";
import { toast } from "sonner";
import { formToSalaryPayload, salaryToForm } from "@/lib/hris/salary-form";
import type { EmployeeSalary } from "../types";
import { SalaryAmountFields } from "./salary-amount-fields";
import { SalaryRuleFields } from "./salary-rule-fields";
import { PayrollSwitchNote, usePayrollSwitches } from "./payroll-switch-note";

interface PageProps {
  params: Promise<{ id: string }>;
}

export function EditSalaryPage({ params }: PageProps) {
  const { id } = use(params);
  const router = useRouter();
  const salaryQuery = useSalary(id);

  useEffect(() => {
    if (salaryQuery.isError) toast.error("Gagal memuat data salary");
  }, [salaryQuery.isError]);

  if (salaryQuery.isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-pink-600" />
      </div>
    );
  }

  if (!salaryQuery.data) {
    return (
      <div className="container mx-auto py-8">
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-gray-500 mb-4">Data salary tidak ditemukan</p>
            <Button onClick={() => router.push("/dashboard/hris/salary")}>
              Kembali
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <EditSalaryForm key={salaryQuery.data.id} salary={salaryQuery.data} />;
}

function EditSalaryForm({ salary }: { salary: EmployeeSalary }) {
  const router = useRouter();
  const updateMutation = useUpdateSalary();
  const switches = usePayrollSwitches();
  const saving = updateMutation.isPending;
  const [formData, setFormData] = useState(() => salaryToForm(salary));

  async function handleSave() {
    try {
      await updateMutation.mutateAsync({ id: salary.id, payload: formToSalaryPayload(formData) });
      toast.success("Data salary berhasil diupdate");
      setTimeout(() => {
        router.push("/dashboard/hris/salary");
      }, 1000);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Terjadi kesalahan");
    }
  }

  const employee = salary.employee;

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
          <h1 className="text-2xl font-bold text-gray-900">Edit Salary Structure</h1>
          <p className="text-sm text-gray-500">
            {employee?.full_name} - {employee?.nip}
          </p>
        </div>
      </div>

      {/* Employee Info Card */}
      <Card className="mb-6">
        <CardHeader className="pb-3">
          <CardTitle className="text-lg">Informasi Karyawan</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-gray-500">Nama</p>
              <p className="font-semibold">{employee?.full_name}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">NIP</p>
              <p>{employee?.nip}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Jabatan</p>
              <p>{employee?.position?.title || "-"}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Departemen</p>
              <p>{employee?.department?.name || "-"}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Entitas pembayar</p>
              <p className="font-semibold">{salary.company?.name ?? "Belum ada entitas"}</p>
              <p className="text-xs text-muted-foreground">Ganti entitas = buat struktur gaji baru.</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Salary Form */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-lg">Salary Structure</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <SalaryAmountFields
            values={formData}
            onChange={(field, value) => setFormData((prev) => ({ ...prev, [field]: value }))}
          />

          <SalaryRuleFields
            values={formData}
            onChange={(field, value) => setFormData((prev) => ({ ...prev, [field]: value }))}
          />

          {/* Tax & Benefits */}
          <div>
            <h3 className="text-sm font-semibold text-gray-700 mb-3">Tax & Benefits</h3>
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="ptkp_status">Status PTKP</Label>
                  <p className="text-xs text-gray-500">Penghasilan Tidak Kena Pajak</p>
                </div>
                <Select
                  value={formData.ptkp_status}
                  onValueChange={(value) => setFormData({ ...formData, ptkp_status: value })}
                >
                  <SelectTrigger className="w-[200px]" id="ptkp_status">
                    <SelectValue placeholder="Pilih status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="TK/0">TK/0 (Tidak Kawin, 0 tanggungan)</SelectItem>
                    <SelectItem value="TK/1">TK/1 (Tidak Kawin, 1 tanggungan)</SelectItem>
                    <SelectItem value="TK/2">TK/2 (Tidak Kawin, 2 tanggungan)</SelectItem>
                    <SelectItem value="TK/3">TK/3 (Tidak Kawin, 3 tanggungan)</SelectItem>
                    <SelectItem value="K/0">K/0 (Kawin, 0 tanggungan)</SelectItem>
                    <SelectItem value="K/1">K/1 (Kawin, 1 tanggungan)</SelectItem>
                    <SelectItem value="K/2">K/2 (Kawin, 2 tanggungan)</SelectItem>
                    <SelectItem value="K/3">K/3 (Kawin, 3 tanggungan)</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="is_taxable">Subjek Pajak</Label>
                  <p className="text-xs text-gray-500">Kena PPh 21</p>
                  <PayrollSwitchNote show={switches.pph21Off} />
                </div>
                <input
                  type="checkbox"
                  id="is_taxable"
                  checked={formData.is_taxable}
                  onChange={(e) => setFormData({ ...formData, is_taxable: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="bpjs_tk">BPJS Ketenagakerjaan</Label>
                  <p className="text-xs text-gray-500">JHT, JP, JKK, JKM</p>
                  <PayrollSwitchNote show={switches.bpjsTkOff} />
                </div>
                <input
                  type="checkbox"
                  id="bpjs_tk"
                  checked={formData.bpjs_tk_enrolled}
                  onChange={(e) => setFormData({ ...formData, bpjs_tk_enrolled: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="bpjs_kes">BPJS Kesehatan</Label>
                  <PayrollSwitchNote show={switches.bpjsKesOff} />
                </div>
                <input
                  type="checkbox"
                  id="bpjs_kes"
                  checked={formData.bpjs_kes_enrolled}
                  onChange={(e) => setFormData({ ...formData, bpjs_kes_enrolled: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <Label htmlFor="tapera">Tapera</Label>
                  <p className="text-xs text-gray-500">Tabungan Perumahan Rakyat</p>
                </div>
                <input
                  type="checkbox"
                  id="tapera"
                  checked={formData.tapera_enrolled}
                  onChange={(e) => setFormData({ ...formData, tapera_enrolled: e.target.checked })}
                  className="h-4 w-4 text-pink-600 focus:ring-pink-500 border-gray-300 rounded"
                />
              </div>
            </div>
          </div>

          {/* Notes */}
          <div>
            <Label htmlFor="notes">Catatan</Label>
            <textarea
              id="notes"
              value={formData.notes}
              onChange={(e) => setFormData({ ...formData, notes: e.target.value })}
              className="w-full mt-1 p-2 border rounded-md text-sm"
              rows={3}
              placeholder="Catatan tambahan..."
            />
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
                  Simpan Perubahan
                </>
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

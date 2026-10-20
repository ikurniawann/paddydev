"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ArrowLeftIcon, CheckCircle2, Loader2 } from "lucide-react";
import { countLeaveDays, indexHolidays } from "@/lib/hris/holidays";
import { todayWib } from "@/lib/dates";
import { leaveTypeInfo, leaveTypeOptions } from "@/lib/hris/leave-types";
import { applySingleDay, leaveFormNote } from "@/lib/hris/leave-form";
import { useLeaveEmployees, useLeaveQuota } from "../queries";
import { useCreateLeave } from "../mutations";
import { LeaveAttachmentField } from "./leave-attachment-field";
import { LeaveSuccessDialog, type LeaveSubmission } from "./leave-success-dialog";

const NO_HOLIDAYS = indexHolidays([]);

/** Pratinjau hari kerja (Senin-Jumat, minimal 1); angka final dihitung server. */
function previewBusinessDays(startDate: string, endDate: string): number {
  if (!startDate || !endDate) return 0;
  return countLeaveDays(startDate, endDate, NO_HOLIDAYS) || 1;
}

export function NewLeavePage() {
  const router = useRouter();

  const { data: employeesData } = useLeaveEmployees();
  const employees = employeesData ?? [];
  const createMutation = useCreateLeave();
  const submitting = createMutation.isPending;

  const [searchQuery, setSearchQuery] = useState("");
  const [showEmployeeDropdown, setShowEmployeeDropdown] = useState(false);
  const [submission, setSubmission] = useState<LeaveSubmission | null>(null);

  const [formData, setFormData] = useState({
    employee_id: "",
    leave_type: "annual",
    start_date: "",
    end_date: "",
    reason: "",
    attachment_url: "",
  });

  const month = (formData.start_date || todayWib()).slice(0, 7);
  const quota =
    useLeaveQuota(month, formData.employee_id || undefined, Boolean(formData.employee_id)).data ?? null;
  const leaveOptions = leaveTypeOptions({ leaderEligible: quota?.leader_eligible ?? false });
  // Jenis terpilih (mis. Izin Leader) hilang dari opsi bila karyawan diganti: jatuh ke "annual".
  const leaveType = leaveOptions.some((o) => o.value === formData.leave_type) ? formData.leave_type : "annual";
  const singleDay = Boolean(leaveTypeInfo(leaveType)?.singleDay);
  const quotaNote = leaveFormNote(leaveType, quota);

  const totalDays = previewBusinessDays(formData.start_date, formData.end_date);

  const filteredEmployees = employees.filter((emp) =>
    emp.full_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    emp.nip.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!formData.employee_id) {
      toast.error("Validasi Gagal: Pilih karyawan terlebih dahulu");
      return;
    }

    if (!formData.start_date || !formData.end_date) {
      toast.error("Validasi Gagal: Tanggal mulai dan selesai harus diisi");
      return;
    }

    if (formData.end_date < formData.start_date) {
      toast.error("Validasi Gagal: Tanggal selesai tidak boleh sebelum tanggal mulai");
      return;
    }

    if (!formData.reason || formData.reason.length < 10) {
      toast.error("Validasi Gagal: Alasan minimal 10 karakter");
      return;
    }

    try {
      const result = await createMutation.mutateAsync({ ...formData, leave_type: leaveType });
      const employee = employees.find((e) => e.id === formData.employee_id);
      setSubmission({
        ...result.data,
        total_days: totalDays,
        employee_name: employee?.full_name,
      });
    } catch (error) {
      toast.error(
        error instanceof Error ? `Pengajuan Gagal: ${error.message}` : "Gagal mengajukan cuti. Silakan coba lagi."
      );
    }
  };

  return (
    <div className="container mx-auto py-8 max-w-3xl">
      {/* Header */}
      <div className="mb-6 flex items-center gap-4">
        <Button variant="outline" onClick={() => router.push("/dashboard/hris/leaves")}>
          <ArrowLeftIcon className="w-4 h-4 mr-2" />
          Kembali
        </Button>
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Ajukan Cuti / Izin</h1>
          <p className="text-sm text-gray-500">Form pengajuan cuti atau izin karyawan</p>
        </div>
      </div>

      {/* Form */}
      <Card>
        <CardHeader>
          <CardTitle>Form Pengajuan Cuti</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-6">
            {/* Employee Selection */}
            <div>
              <Label htmlFor="employee_id">Karyawan *</Label>
              <div className="relative mt-1">
                <Input
                  id="employee_id"
                  placeholder="Cari nama atau NIP karyawan..."
                  value={searchQuery}
                  onChange={(e) => {
                    setSearchQuery(e.target.value);
                    setShowEmployeeDropdown(true);
                    if (e.target.value === '') {
                      setFormData({ ...formData, employee_id: "" });
                    }
                  }}
                  onFocus={() => setShowEmployeeDropdown(true)}
                  onBlur={() => setTimeout(() => setShowEmployeeDropdown(false), 200)}
                />
                {showEmployeeDropdown && filteredEmployees.length > 0 && (
                  <div className="absolute z-50 w-full mt-1 bg-white border border-gray-200 rounded-lg shadow-lg max-h-60 overflow-auto">
                    {filteredEmployees.map((emp) => (
                      <div
                        key={emp.id}
                        className="px-4 py-2 hover:bg-gray-100 cursor-pointer"
                        onClick={() => {
                          setFormData({ ...formData, employee_id: emp.id });
                          setSearchQuery(`${emp.full_name} (${emp.nip})`);
                          setShowEmployeeDropdown(false);
                        }}
                      >
                        <div className="font-medium text-sm">{emp.full_name}</div>
                        <div className="text-xs text-gray-500">{emp.nip} - {emp.department?.name || 'No Dept'}</div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Leave Type */}
            <div>
              <Label htmlFor="leave_type">Jenis Cuti *</Label>
              <Select
                value={leaveType}
                onValueChange={(value) => {
                  const days = applySingleDay(value, formData.start_date, formData.end_date);
                  setFormData({ ...formData, leave_type: value, start_date: days.start, end_date: days.end });
                }}
              >
                <SelectTrigger className="mt-1" id="leave_type">
                  <SelectValue placeholder="Pilih jenis cuti" />
                </SelectTrigger>
                <SelectContent>
                  {leaveOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Date Range */}
            <div className="grid grid-cols-2 gap-4">
              <div>
                <Label htmlFor="start_date">Tanggal Mulai *</Label>
                <Input
                  id="start_date"
                  type="date"
                  value={formData.start_date}
                  onChange={(e) => {
                    const days = applySingleDay(leaveType, e.target.value, formData.end_date);
                    setFormData({ ...formData, start_date: days.start, end_date: days.end });
                  }}
                  className="mt-1"
                />
              </div>
              <div>
                <Label htmlFor="end_date">Tanggal Selesai *</Label>
                <Input
                  id="end_date"
                  type="date"
                  value={formData.end_date}
                  disabled={singleDay}
                  onChange={(e) => setFormData({ ...formData, end_date: e.target.value })}
                  className="mt-1"
                />
              </div>
            </div>

            {/* Total Days Display */}
            {totalDays > 0 && (
              <div className="bg-green-50 border border-green-200 rounded-lg p-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-green-800">Total Hari (hari kerja):</span>
                  <Badge variant="outline" className="bg-green-100 text-green-800">
                    {totalDays} hari
                  </Badge>
                </div>
              </div>
            )}

            {quotaNote && (
              <p className="rounded-lg bg-sky-50 px-3 py-2 text-xs text-sky-700">{quotaNote}</p>
            )}

            {/* Reason */}
            <div>
              <Label htmlFor="reason">Alasan *</Label>
              <Textarea
                id="reason"
                placeholder="Jelaskan alasan pengajuan cuti..."
                value={formData.reason}
                onChange={(e) => setFormData({ ...formData, reason: e.target.value })}
                rows={4}
                className="mt-1"
              />
              <p className="text-xs text-gray-500 mt-1">Minimal 10 karakter</p>
            </div>

            <LeaveAttachmentField
              employeeId={formData.employee_id}
              onChange={(path) => setFormData((prev) => ({ ...prev, attachment_url: path }))}
            />

            {/* Submit Button */}
            <div className="flex gap-3 pt-4 border-t">
              <Button
                type="button"
                variant="outline"
                onClick={() => router.push("/dashboard/hris/leaves")}
                className="flex-1"
              >
                Batal
              </Button>
              <Button
                type="submit"
                disabled={submitting}
                className="flex-1 bg-pink-600 hover:bg-pink-700"
              >
                {submitting ? (
                  <>
                    <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                    Mengajukan...
                  </>
                ) : (
                  <>
                    <CheckCircle2 className="w-4 h-4 mr-2" />
                    Ajukan Cuti
                  </>
                )}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {submission && (
        <LeaveSuccessDialog
          submission={submission}
          onClose={() => router.push("/dashboard/hris/leaves")}
        />
      )}
    </div>
  );
}

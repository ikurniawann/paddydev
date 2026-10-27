"use client";

import { use } from "react";
import { toast } from "sonner";
import { OnboardingChecklist } from "@/components/hris/OnboardingChecklist";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ArrowLeft, User, Calendar, Briefcase, Mail, Phone, Loader2 } from "lucide-react";
import Link from "next/link";
import { formatDate } from "@/lib/format";
import { EMPLOYMENT_STATUS_BADGES, tenureDays } from "@/lib/hris/onboarding-view";
import { useOnboardingEmployee } from "../queries";

interface OnboardingPageProps {
  params: Promise<{ employee_id: string }>;
}

function EmploymentStatusBadge({ status }: { status: string }) {
  const badge = EMPLOYMENT_STATUS_BADGES[status];
  return (
    <Badge variant="outline" className={badge?.className ?? "bg-gray-100 text-gray-800"}>
      {badge?.label ?? status}
    </Badge>
  );
}

const handleTaskComplete = () =>
  toast.success("✅ Task Selesai", { description: "Progress onboarding telah diupdate" });

export function OnboardingPage({ params }: OnboardingPageProps) {
  const { employee_id: employeeId } = use(params);
  const { data: employee, isLoading } = useOnboardingEmployee(employeeId);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 animate-spin text-green-600" />
      </div>
    );
  }

  if (!employee) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center justify-center py-12">
          <p className="text-gray-500 mb-4">Karyawan tidak ditemukan</p>
          <Button asChild>
            <Link href="/dashboard/employees">
              <ArrowLeft className="w-4 h-4 mr-2" />
              Kembali ke Daftar Karyawan
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-500">
        <Link href="/dashboard/employees" className="hover:text-gray-900">
          Direktori Karyawan
        </Link>
        <span>/</span>
        <span className="text-gray-900 font-medium">Onboarding</span>
      </div>

      {/* Header with Back Button */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link href="/dashboard/employees">
            <ArrowLeft className="w-5 h-5" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Onboarding Karyawan</h1>
          <p className="text-gray-500 mt-1">Monitor dan kelola progress onboarding</p>
        </div>
      </div>

      {/* Employee Info Card */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-3">
            <div className="p-2 bg-green-100 rounded-full">
              <User className="w-6 h-6 text-green-600" />
            </div>
            <div>
              {employee.full_name}
              <EmploymentStatusBadge status={employee.employment_status} />
            </div>
            <span className="text-sm text-gray-500 font-normal ml-2">Informasi karyawan dan detail onboarding</span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {/* NIP */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">NIP</p>
              <p className="text-base text-gray-900">{employee.nip}</p>
            </div>

            {/* Department */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Departemen</p>
              <p className="text-base text-gray-900">
                {employee.department?.name || "-"}
              </p>
            </div>

            {/* Job Title */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Jabatan</p>
              <p className="text-base text-gray-900">
                {employee.job_title?.title || "-"}
              </p>
            </div>

            {/* Join Date */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Tanggal Bergabung</p>
              <div className="flex items-center gap-2 text-base text-gray-900">
                <Calendar className="w-4 h-4" />
                <span>{formatDate(employee.join_date)}</span>
              </div>
            </div>

            {/* Email */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Email</p>
              <div className="flex items-center gap-2 text-base text-gray-900">
                <Mail className="w-4 h-4" />
                <span>{employee.email}</span>
              </div>
            </div>

            {/* Phone */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Telepon</p>
              <div className="flex items-center gap-2 text-base text-gray-900">
                <Phone className="w-4 h-4" />
                <span>{employee.phone}</span>
              </div>
            </div>

            {/* Employment Status */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Status Kepegawaian</p>
              <div className="flex items-center gap-2">
                <Briefcase className="w-4 h-4 text-gray-400" />
                <EmploymentStatusBadge status={employee.employment_status} />
              </div>
            </div>

            {/* Days Since Join */}
            <div className="space-y-1">
              <p className="text-sm font-medium text-gray-700">Masa Kerja</p>
              <p className="text-base text-gray-900">
                {tenureDays(employee.join_date, new Date())} hari
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Onboarding Progress Info */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Checklist Onboarding</CardTitle>
              <span className="text-sm text-gray-500">Task yang harus diselesaikan selama masa onboarding</span>
            </div>
            <Badge variant="outline" className="bg-blue-100 text-blue-800">
              Auto-generated saat karyawan bergabung
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="bg-blue-50 border border-blue-200 p-4 rounded-lg mb-6">
            <h4 className="text-sm font-semibold text-blue-900 mb-2">
              ℹ️ Tentang Onboarding Checklist
            </h4>
            <ul className="text-sm text-blue-800 space-y-1">
              <li>• Checklist ini dibuat otomatis saat karyawan baru bergabung</li>
              <li>• Terdiri dari 12 task across 4 kategori: HRD, IT, Manager, Admin</li>
              <li>• Setiap task memiliki prioritas dan due date</li>
              <li>• PIC (Person In Charge) ditugaskan untuk memastikan penyelesaian</li>
              <li>• Centang task untuk menandai sebagai selesai</li>
            </ul>
          </div>

          {/* Onboarding Checklist Component */}
          <OnboardingChecklist
            employeeId={employeeId}
            canEdit={true}
            onTaskComplete={handleTaskComplete}
          />
        </CardContent>
      </Card>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Aksi Cepat</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-3">
            <Button variant="outline" asChild>
              <Link href={`/dashboard/employees`}>
                Lihat Profil Lengkap
              </Link>
            </Button>
            
            <Button variant="outline" asChild>
              <Link href={`/dashboard/hris/offboarding/${employeeId}`}>
                Kelola Offboarding
              </Link>
            </Button>

            <Button variant="outline" asChild>
              <Link href={`/dashboard/employees/${employeeId}?tab=leave`}>
                Lihat Saldo Cuti
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

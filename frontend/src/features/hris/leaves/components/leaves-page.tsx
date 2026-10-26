"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { formatDate } from "@/lib/format";
import { ApprovalButtons } from "@/components/hris/ApprovalButtons";
import { useHrApproveAccess } from "@/features/hris/approve-access";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Calendar,
  Plus,
  Search,
  Download,
  User,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
} from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LEAVE_TYPES } from "@/lib/hris/leave-types";
import { LEAVE_TYPE_LABELS, LEAVE_STATUS_LABELS, LEAVE_STATUS_COLORS } from "@/types/hris";
import { useLeaveList } from "../queries";
import { downloadLeavesCsv } from "../api";
import type { LeaveListParams } from "../types";

export function LeavesPage() {
  const router = useRouter();
  const [filterStatus, setFilterStatus] = useState<string>("all");
  const [filterType, setFilterType] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");

  const listParams = useMemo<LeaveListParams>(
    () => ({
      limit: 100,
      status: filterStatus !== "all" ? filterStatus : undefined,
      leave_type: filterType !== "all" ? filterType : undefined,
    }),
    [filterStatus, filterType]
  );

  const { data, isLoading } = useLeaveList(listParams);
  // Tombol keputusan HR hanya untuk pemegang `approve` daun Cuti (EPIC-067 R14); gagal tutup
  // selama memuat/error. Antrean atasan ada di ESS Persetujuan Tim, bukan di sini.
  const { data: access } = useHrApproveAccess();
  const canDecide = access?.leaves === true;
  const leaves = useMemo(() => data ?? [], [data]);

  const handleExport = async () => {
    try {
      const blob = await downloadLeavesCsv({
        status: listParams.status,
        leave_type: listParams.leave_type,
      });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `leave_export_${new Date().toISOString().split('T')[0]}.csv`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (error) {
      toast.error('Export gagal: ' + (error as Error).message);
    }
  };

  const filteredLeaves = leaves.filter((leave) => {
    if (!searchQuery) return true;
    const query = searchQuery.toLowerCase();
    return (
      leave.employee?.full_name?.toLowerCase().includes(query) ||
      leave.employee?.nip?.toLowerCase().includes(query) ||
      leave.reason?.toLowerCase().includes(query)
    );
  });

  const getStatusBadge = (status: string) => {
    const colors = LEAVE_STATUS_COLORS[status as keyof typeof LEAVE_STATUS_COLORS] || "bg-gray-100 text-gray-800";
    return (
      <Badge variant="outline" className={colors}>
        {LEAVE_STATUS_LABELS[status as keyof typeof LEAVE_STATUS_LABELS] || status}
      </Badge>
    );
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h1 className="text-xl sm:text-2xl font-bold text-gray-900">Manajemen Cuti & Izin</h1>
          <p className="text-sm text-gray-500 mt-1">Kelola pengajuan cuti karyawan dengan approval workflow</p>
        </div>

        <Button size="sm" className="bg-green-600 hover:bg-green-700" onClick={() => router.push('/dashboard/hris/leaves/insert')}>
          <Plus className="w-3.5 h-3.5 mr-1.5" />
          <span className="hidden sm:inline">Ajukan Cuti</span>
          <span className="sm:hidden">Cuti</span>
        </Button>
      </div>

      {/* Filters */}
      <Card>
        <CardContent className="pt-6">
          <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
            <div className="sm:col-span-2">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
                <Input
                  placeholder="Cari berdasarkan nama karyawan atau alasan..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-10"
                />
              </div>
            </div>

            <Select value={filterStatus} onValueChange={setFilterStatus}>
              <SelectTrigger>
                <SelectValue placeholder="Filter status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="pending">Pending</SelectItem>
                <SelectItem value="approved">Disetujui</SelectItem>
                <SelectItem value="rejected">Ditolak</SelectItem>
                <SelectItem value="cancelled">Dibatalkan</SelectItem>
              </SelectContent>
            </Select>

            <Select value={filterType} onValueChange={setFilterType}>
              <SelectTrigger>
                <SelectValue placeholder="Filter jenis" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Jenis</SelectItem>
                {LEAVE_TYPES.map((t) => (
                  <SelectItem key={t.code} value={t.code}>
                    {t.selectable ? t.label : `${t.label} (lama)`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Leave Requests List */}
      <div className="space-y-4">
        {isLoading ? (
          <Card>
            <CardContent className="flex items-center justify-center py-12">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-600"></div>
            </CardContent>
          </Card>
        ) : filteredLeaves.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center justify-center py-12">
              <AlertCircle className="w-12 h-12 text-gray-300 mb-4" />
              <p className="text-gray-500">Belum ada pengajuan cuti</p>
            </CardContent>
          </Card>
        ) : (
          filteredLeaves.map((leave) => (
            <Card key={leave.id}>
              <CardHeader>
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="p-2 bg-green-100 rounded-full">
                      <User className="w-5 h-5 text-green-600" />
                    </div>
                    <div>
                      <CardTitle className="text-lg">{leave.employee?.full_name || "Unknown"}</CardTitle>
                      <p className="text-sm text-gray-500">
                        {leave.employee?.nip} • {leave.employee?.department?.name || "-"}
                      </p>
                    </div>
                  </div>
                  {getStatusBadge(leave.status)}
                </div>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {/* Left Column */}
                  <div className="space-y-3">
                    <div>
                      <p className="text-sm font-medium text-gray-700">Jenis Cuti</p>
                      <Badge variant="outline" className="mt-1">
                        {LEAVE_TYPE_LABELS[leave.leave_type as keyof typeof LEAVE_TYPE_LABELS] || leave.leave_type}
                      </Badge>
                    </div>

                    <div>
                      <p className="text-sm font-medium text-gray-700">Periode</p>
                      <div className="flex items-center gap-2 mt-1 text-sm text-gray-600">
                        <Calendar className="w-4 h-4" />
                        <span>{formatDate(leave.start_date)}</span>
                        <span>-</span>
                        <span>{formatDate(leave.end_date)}</span>
                      </div>
                    </div>

                    <div>
                      <p className="text-sm font-medium text-gray-700">Durasi</p>
                      <div className="flex items-center gap-2 mt-1 text-sm text-gray-600">
                        <Clock className="w-4 h-4" />
                        <span>{leave.total_days} hari kerja</span>
                      </div>
                    </div>
                  </div>

                  {/* Right Column */}
                  <div className="space-y-3">
                    <div>
                      <p className="text-sm font-medium text-gray-700">Alasan</p>
                      <p className="text-sm text-gray-600 mt-1 line-clamp-2">{leave.reason}</p>
                    </div>

                    {leave.status === "approved" && leave.approver && (
                      <div className="flex items-center gap-2 text-sm text-green-600">
                        <CheckCircle2 className="w-4 h-4" />
                        <span>Disetujui oleh {leave.approver.full_name}</span>
                      </div>
                    )}

                    {leave.status === "rejected" && (
                      <div className="flex items-center gap-2 text-sm text-red-600">
                        <XCircle className="w-4 h-4" />
                        <span>Ditolak</span>
                      </div>
                    )}
                  </div>
                </div>

                {/* Actions */}
                {leave.status === "pending" && (
                  <div className="flex items-center justify-between mt-4 pt-4 border-t">
                    <p className="text-xs text-gray-500">
                      Diajukan: {formatDate(leave.created_at)}
                    </p>
                    {canDecide && <ApprovalButtons leaveId={leave.id} />}
                  </div>
                )}
              </CardContent>
            </Card>
          ))
        )}
      </div>

      {/* Stats Summary */}
      <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-4">
              <div className="p-3 bg-yellow-100 rounded-full">
                <AlertCircle className="w-6 h-6 text-yellow-600" />
              </div>
              <div>
                <p className="text-sm text-gray-500">Pending</p>
                <p className="text-2xl font-bold text-gray-900">
                  {leaves.filter(l => l.status === "pending").length}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-4">
              <div className="p-3 bg-green-100 rounded-full">
                <CheckCircle2 className="w-6 h-6 text-green-600" />
              </div>
              <div>
                <p className="text-sm text-gray-500">Disetujui</p>
                <p className="text-2xl font-bold text-gray-900">
                  {leaves.filter(l => l.status === "approved").length}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-4">
              <div className="p-3 bg-red-100 rounded-full">
                <XCircle className="w-6 h-6 text-red-600" />
              </div>
              <div>
                <p className="text-sm text-gray-500">Ditolak</p>
                <p className="text-2xl font-bold text-gray-900">
                  {leaves.filter(l => l.status === "rejected").length}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-4">
              <div className="p-3 bg-blue-100 rounded-full">
                <Calendar className="w-6 h-6 text-blue-600" />
              </div>
              <div>
                <p className="text-sm text-gray-500">Total</p>
                <p className="text-2xl font-bold text-gray-900">{leaves.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={handleExport}>
          <Download className="w-3.5 h-3.5 mr-1.5" />
          Export CSV
        </Button>
      </div>
    </div>
  );
}

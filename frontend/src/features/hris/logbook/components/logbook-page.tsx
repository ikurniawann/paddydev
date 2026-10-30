"use client";

import { useState } from "react";
import { BookOpenCheck } from "lucide-react";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLogbookDepartments, useLogbookMe } from "../queries";
import { LogbookChecklistTab } from "./checklist-tab";
import { LogbookRiwayatTab } from "./riwayat-tab";
import { LogbookTemplateTab } from "./template-tab";
import { LogbookKpiSummaryTab } from "./kpi-summary-tab";

const ALL_DEPARTMENTS = "all";

/**
 * Logbook Department — satu halaman ber-Tabs (EPIC-009):
 * Checklist (alur template-first) | Riwayat | Template | Ringkasan KPI.
 * Enforcement department dilakukan server; Select department hanya muncul
 * untuk akun full-access.
 */
export function LogbookPage() {
  const [tab, setTab] = useState("checklist");
  const [selectedDepartment, setSelectedDepartment] = useState(ALL_DEPARTMENTS);

  const meQuery = useLogbookMe();
  const me = meQuery.data ?? null;
  const isFullAccess = me?.is_full_access ?? false;

  const departmentsQuery = useLogbookDepartments();
  const departments = departmentsQuery.data ?? [];

  // Non-full-access: server mengunci ke department sendiri; kirim kosong saja.
  const departmentId = isFullAccess
    ? selectedDepartment === ALL_DEPARTMENTS
      ? ""
      : selectedDepartment
    : "";

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-semibold">
            <BookOpenCheck className="h-6 w-6" /> Logbook Department
          </h1>
          <p className="text-sm text-muted-foreground">
            Checklist KPI harian per department — dibuat dari template, diisi
            kepala department, direview HRD.
          </p>
        </div>

        {isFullAccess && (
          <div className="w-full space-y-1 md:w-64">
            <Label>Department</Label>
            <Select value={selectedDepartment} onValueChange={setSelectedDepartment}>
              <SelectTrigger>
                <SelectValue placeholder="Semua department" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL_DEPARTMENTS}>Semua Department</SelectItem>
                {departments.map((department) => (
                  <SelectItem key={department.id} value={department.id}>
                    {department.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        {!isFullAccess && me?.employee?.department?.name && (
          <p className="text-sm text-muted-foreground">
            Department:{" "}
            <span className="font-medium text-foreground">
              {me.employee.department.name}
            </span>
          </p>
        )}
      </div>

      {!meQuery.isLoading && me && !me.is_full_access && !me.employee?.department_id && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          Akun Anda belum tertaut ke karyawan/department mana pun — hubungi HRD
          untuk mengakses logbook.
        </div>
      )}

      <Tabs value={tab} onValueChange={setTab} className="w-full flex-col">
        {/* HP: tab digeser horizontal (bawaan TabsList); dari sm baru kolom sama lebar. */}
        <TabsList className="sm:grid sm:h-9 sm:w-full sm:max-w-xl sm:grid-cols-4">
          <TabsTrigger value="checklist">Checklist</TabsTrigger>
          <TabsTrigger value="riwayat">Riwayat</TabsTrigger>
          <TabsTrigger value="template">Template</TabsTrigger>
          <TabsTrigger value="kpi">Ringkasan KPI</TabsTrigger>
        </TabsList>

        <TabsContent value="checklist" className="mt-4">
          <LogbookChecklistTab
            departmentId={departmentId}
            onGoToTemplates={() => setTab("template")}
          />
        </TabsContent>
        <TabsContent value="riwayat" className="mt-4">
          <LogbookRiwayatTab
            me={me}
            departmentId={departmentId}
          />
        </TabsContent>
        <TabsContent value="template" className="mt-4">
          <LogbookTemplateTab
            me={me}
            departmentId={departmentId}
            departments={departments}
          />
        </TabsContent>
        <TabsContent value="kpi" className="mt-4">
          <LogbookKpiSummaryTab departmentId={departmentId} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

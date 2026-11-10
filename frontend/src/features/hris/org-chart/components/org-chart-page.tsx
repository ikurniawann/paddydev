"use client";

import { useState } from "react";
import Image from "next/image";
import { useRouter } from "next/navigation";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { BuildingOfficeIcon, UserGroupIcon, ChevronDownIcon, ChevronRightIcon } from "@heroicons/react/24/outline";
import { buildOrgTree, type OrgEmployee, type OrgNode } from "@/lib/hris/org-chart";
import { useOrgEmployees, useOrgDepartments } from "../queries";
import type { OrgDepartment } from "../types";
import { ChartGap } from "./chart-gap";
import { ChartTree } from "./chart-tree";

const STATUS_COLORS: Record<string, string> = {
  permanent: "bg-green-100 text-green-700",
  contract: "bg-blue-100 text-blue-700",
  probation: "bg-yellow-100 text-yellow-700",
  internship: "bg-purple-100 text-purple-700",
};

const STATUS_LABELS: Record<string, string> = { permanent: "Tetap", contract: "Kontrak", probation: "Probasi" };

function EmployeeCard({ node, depth }: { node: OrgNode; depth: number }) {
  const router = useRouter();
  const [expanded, setExpanded] = useState(depth < 2);
  const hasReports = node.direct_reports.length > 0;

  return (
    <div className="flex flex-col items-center">
      {/* Card */}
      <div
        className="relative group cursor-pointer"
        onClick={() => router.push(`/dashboard/employees/${node.id}`)}
      >
        <div className="w-44 bg-white border border-gray-200 rounded-lg p-3 shadow-sm hover:shadow-md hover:border-blue-300 transition-all">
          <div className="flex flex-col items-center text-center">
            {node.photo_url ? (
              <Image
                src={node.photo_url}
                alt={node.full_name}
                width={40}
                height={40}
                unoptimized
                className="w-10 h-10 rounded-full object-cover mb-2"
              />
            ) : (
              <div className="w-10 h-10 rounded-full bg-gradient-to-br from-blue-400 to-indigo-500 flex items-center justify-center text-sm font-bold text-white mb-2">
                {node.full_name.charAt(0)}
              </div>
            )}
            <p className="text-xs font-semibold text-gray-900 leading-tight">{node.full_name}</p>
            <p className="text-xs text-gray-500 mt-0.5 leading-tight">{node.job_title?.title || "—"}</p>
            {node.employment_status && (
              <Badge className={`mt-1 text-xs px-1.5 py-0 ${STATUS_COLORS[node.employment_status] || "bg-gray-100 text-gray-600"}`}>
                {STATUS_LABELS[node.employment_status] ?? node.employment_status}
              </Badge>
            )}
          </div>
        </div>

        {/* Expand button */}
        {hasReports && (
          <button
            className="absolute -bottom-3 left-1/2 -translate-x-1/2 w-6 h-6 bg-white border border-gray-300 rounded-full flex items-center justify-center shadow-sm hover:bg-blue-50 hover:border-blue-400 z-10"
            onClick={(e) => { e.stopPropagation(); setExpanded((v) => !v); }}
            title={expanded ? "Sembunyikan" : "Tampilkan bawahan"}
          >
            {expanded ? (
              <ChevronDownIcon className="w-3 h-3 text-gray-500" />
            ) : (
              <ChevronRightIcon className="w-3 h-3 text-gray-500" />
            )}
          </button>
        )}
      </div>

      {/* Children */}
      {hasReports && expanded && (
        <div className="mt-6 relative">
          {/* Vertical connector from parent */}
          <div className="absolute left-1/2 -top-6 w-0.5 h-6 bg-gray-300" />

          <div className="flex gap-6 relative">
            {/* Horizontal connector */}
            {node.direct_reports.length > 1 && (
              <div className="absolute top-0 left-0 right-0 h-0.5 bg-gray-300" style={{ top: 0 }} />
            )}
            {node.direct_reports.map((child) => (
              <div key={child.id} className="relative flex flex-col items-center">
                {/* Vertical connector to child */}
                <div className="w-0.5 h-4 bg-gray-300 mb-2" />
                <EmployeeCard node={child} depth={depth + 1} />
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function DepartmentSection({ dept, employees }: { dept: OrgDepartment; employees: OrgEmployee[] }) {
  const [collapsed, setCollapsed] = useState(false);
  const rootNodes = buildOrgTree(employees);

  return (
    <div className="mb-10">
      <button
        className="flex items-center gap-2 mb-6 group"
        onClick={() => setCollapsed((v) => !v)}
      >
        <div className="flex items-center gap-2 bg-gray-900 text-white px-4 py-2 rounded-lg shadow-sm group-hover:bg-gray-800 transition-colors">
          <BuildingOfficeIcon className="w-4 h-4" />
          <span className="font-semibold text-sm">{dept.name}</span>
          <span className="bg-white/20 text-white text-xs px-1.5 py-0.5 rounded">{employees.length}</span>
          {collapsed ? <ChevronRightIcon className="w-3 h-3" /> : <ChevronDownIcon className="w-3 h-3" />}
        </div>
      </button>
      {!collapsed && (
        <div className="overflow-x-auto pb-4">
          <div className="flex gap-8 min-w-max px-4">
            {rootNodes.map((node) => (
              <EmployeeCard key={node.id} node={node} depth={0} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ActualStructure() {
  const [selectedDept, setSelectedDept] = useState("all");

  const employeesQuery = useOrgEmployees();
  const departmentsQuery = useOrgDepartments();
  const employees = employeesQuery.data ?? [];
  const departments = departmentsQuery.data ?? [];
  const loading = employeesQuery.isLoading || departmentsQuery.isLoading;

  const filteredEmployees = selectedDept === "all"
    ? employees
    : employees.filter((e) => e.department_id === selectedDept);

  // Group by department
  const deptGroups = departments
    .map((d) => ({
      dept: d,
      employees: filteredEmployees.filter((e) => e.department_id === d.id),
    }))
    .filter((g) => g.employees.length > 0);

  const noDeptEmployees = filteredEmployees.filter((e) => !e.department_id);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin w-8 h-8 border-2 border-gray-300 border-t-blue-500 rounded-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Struktur Organisasi</h1>
          <p className="text-sm text-gray-500 mt-1">{filteredEmployees.length} karyawan aktif</p>
        </div>
        <Select value={selectedDept} onValueChange={(v) => setSelectedDept(v ?? "all")}>
          <SelectTrigger className="w-full sm:w-52">
            <SelectValue placeholder="Semua Departemen" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua Departemen</SelectItem>
            {departments.map((d) => (
              <SelectItem key={d.id} value={d.id}>{d.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Legend */}
      <div className="flex flex-wrap gap-3 text-xs">
        {[
          { label: "Tetap", color: "bg-green-100 text-green-700" },
          { label: "Kontrak", color: "bg-blue-100 text-blue-700" },
          { label: "Probasi", color: "bg-yellow-100 text-yellow-700" },
        ].map((l) => (
          <Badge key={l.label} className={l.color}>{l.label}</Badge>
        ))}
        <span className="text-gray-400">· Klik kartu untuk lihat profil · Klik tombol + untuk expand/collapse</span>
      </div>

      {/* Org Chart */}
      {filteredEmployees.length === 0 ? (
        <Card>
          <CardContent className="py-16 text-center text-gray-400">
            <UserGroupIcon className="w-10 h-10 mx-auto mb-3 text-gray-300" />
            Tidak ada karyawan aktif ditemukan
          </CardContent>
        </Card>
      ) : (
        <div className="bg-gray-50 rounded-xl p-6 min-h-96 overflow-auto">
          {deptGroups.map(({ dept, employees: deptEmps }) => (
            <DepartmentSection key={dept.id} dept={dept} employees={deptEmps} />
          ))}
          {noDeptEmployees.length > 0 && (
            <DepartmentSection
              dept={{ id: "none", name: "Tanpa Departemen" }}
              employees={noDeptEmployees}
            />
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Struktur Organisasi: struktur aktual dari atasan langsung karyawan, bagan
 * Draft 6 dari klien, dan selisih keduanya (EPIC-101).
 */
export function OrgChartPage() {
  return (
    <Tabs defaultValue="actual" className="space-y-4">
      <TabsList>
        <TabsTrigger value="actual">Struktur Aktual</TabsTrigger>
        <TabsTrigger value="chart">Bagan Draft 6</TabsTrigger>
        <TabsTrigger value="gap">Selisih</TabsTrigger>
      </TabsList>
      <TabsContent value="actual">
        <ActualStructure />
      </TabsContent>
      <TabsContent value="chart">
        <ChartTree />
      </TabsContent>
      <TabsContent value="gap">
        <ChartGap />
      </TabsContent>
    </Tabs>
  );
}

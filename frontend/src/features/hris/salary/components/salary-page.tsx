"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Download, Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { PlusIcon, PencilIcon } from "@heroicons/react/24/outline";
import { formatDate, formatRupiah } from "@/lib/format";
import { EntityTabs, useEntityParam } from "@/features/hris/payroll";
import { todayWib } from "@/lib/dates";
import { useSalaryList } from "../queries";
import { downloadSalaryXlsx, matchesSalarySearch, sortSalaries } from "../salary-export";

export function SalaryPage() {
  const router = useRouter();

  const salaryQuery = useSalaryList();
  const salaries = salaryQuery.data ?? [];
  // Gaji aktif tanpa entitas tidak milik tab mana pun: tampil sebagai peringatan, bukan tab.
  const unassigned = salaries.filter((s) => s.is_active !== false && !s.company_id);
  const { entity, setEntity, entities, loading: entitiesLoading } = useEntityParam();
  const entityName = entities.find((e) => e.id === entity)?.name ?? "";
  // EPIC-075 R3/R4: urut nama A–Z, cari nama/NIP di tab entitas yang terbuka.
  const [search, setSearch] = useState("");
  const [exporting, setExporting] = useState(false);
  const inEntity = useMemo(() => sortSalaries(salaries.filter((s) => s.company_id === entity)), [salaries, entity]);
  const visible = inEntity.filter((s) => matchesSalarySearch(s, search));
  const loading = salaryQuery.isLoading || entitiesLoading;

  async function handleExport() {
    setExporting(true);
    try {
      await downloadSalaryXlsx(visible, entityName, todayWib());
    } catch (err) {
      toast.error("Export gagal: " + (err instanceof Error ? err.message : "unknown"));
    } finally {
      setExporting(false);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-pink-600" />
      </div>
    );
  }

  return (
    <div className="space-y-6 pb-12">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Salary Structure</h1>
          <p className="text-sm text-gray-500">Kelola struktur gaji karyawan</p>
        </div>
        <Button onClick={() => router.push(`/dashboard/hris/salary/insert${entity ? `?entity=${entity}` : ""}`)} className="bg-pink-600 hover:bg-pink-700">
          <PlusIcon className="w-4 h-4 mr-2" />
          Tambah Salary
        </Button>
      </div>

      {unassigned.length > 0 && (
        <div className="rounded-2xl bg-warning-soft p-4 text-sm">
          <p className="font-semibold">{unassigned.length} struktur gaji aktif belum punya entitas pembayar</p>
          <p className="mt-0.5 text-muted-foreground">
            Gaji ini tidak ikut payroll entitas mana pun. Buat versi baru dengan entitas pembayar lewat
            &quot;Tambah Salary&quot;; versi tanpa entitas otomatis digantikan.
          </p>
          <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1">
            {unassigned.map((s) => (
              <li key={s.id}>
                <Link href={`/dashboard/hris/salary/${s.id}`} className="font-medium text-forest underline-offset-4 hover:underline">
                  {s.employee?.full_name}
                </Link>
                <span className="text-xs text-muted-foreground"> · {s.employee?.nip}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <EntityTabs value={entity} entities={entities} onChange={setEntity} />

      {/* Salary Table */}
      <Card>
        <CardHeader className="space-y-3">
          <CardTitle>Daftar Salary Structure{entityName ? ` — ${entityName}` : ""}</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-full sm:w-72">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <Input
                className="pl-9"
                placeholder="Cari nama atau NIP"
                aria-label="Cari nama atau NIP"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <Button variant="outline" onClick={() => void handleExport()} disabled={exporting || visible.length === 0}>
              <Download className="mr-2 h-4 w-4" />
              Export Excel
            </Button>
            <span className="text-xs text-gray-500">
              {visible.length} dari {inEntity.length} karyawan
            </span>
          </div>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b">
                  <th className="text-left py-3 px-4 font-medium text-gray-500">Karyawan</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">Gaji Pokok</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">Tunjangan Tetap</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">Transport</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">Makan</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">Status PTKP</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">Efektif</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">Aksi</th>
                </tr>
              </thead>
              <tbody>
                {visible.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-gray-500">
                      {inEntity.length > 0
                        ? "Tidak ada karyawan yang cocok"
                        : entityName
                          ? `Belum ada struktur gaji untuk ${entityName}.`
                          : "Belum ada entitas aktif."}
                    </td>
                  </tr>
                ) : (
                  visible.map((salary) => (
                    <tr key={salary.id} className="border-b hover:bg-gray-50">
                      <td className="py-3 px-4">
                        <div>
                          <div className="font-medium">{salary.employee?.full_name}</div>
                          <div className="text-xs text-gray-500">{salary.employee?.nip}</div>
                        </div>
                      </td>
                      <td className="text-right py-3 px-4 font-medium">
                        {formatRupiah(salary.base_salary)}
                      </td>
                      <td className="text-right py-3 px-4">
                        {formatRupiah(salary.fixed_allowance)}
                      </td>
                      <td className="text-right py-3 px-4">
                        {formatRupiah(salary.transport_allowance)}
                      </td>
                      <td className="text-right py-3 px-4">
                        {formatRupiah(salary.meal_allowance)}
                      </td>
                      <td className="py-3 px-4">
                        <Badge>{salary.ptkp_status}</Badge>
                      </td>
                      <td className="py-3 px-4 text-sm text-gray-500">
                        {formatDate(salary.effective_date)}
                      </td>
                      <td className="text-right py-3 px-4">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => router.push(`/dashboard/hris/salary/${salary.id}`)}
                        >
                          <PencilIcon className="w-4 h-4" />
                        </Button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { BuildingOfficeIcon, PlusIcon, UserGroupIcon } from "@heroicons/react/24/outline";
import { Filter, Search, Users, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { buildListUrl } from "@/lib/api-client";
import { useDepartmentList } from "@/features/master-data/departments";
import { PurchasingListSection } from "@/features/purchasing/components/shared/purchasing-list-section";
import { PurchasingTablePagination } from "@/features/purchasing/components/shared/purchasing-table-pagination";
import { useIamAccess } from "@/components/iam/iam-access-provider";
import { IAM } from "@/lib/iam/prefixes";
import { OS_PATH } from "@/lib/desktop/deep-link";
import { impersonateUser } from "../api";
import { useUserDirectoryStats, useUserList } from "../queries";
import { EMPLOYEES_ROUTES } from "../constants";
import { ContractExpiryBanner } from "./contract-expiry-banner";
import { NoManagerCard } from "./no-manager-card";
import { ResetPasswordDialog, type ResetPasswordTarget } from "./reset-password-dialog";
import { CreateAccountDialog } from "./create-account-dialog";
import {
  DEFAULT_EMPLOYEE_DIRECTORY_FILTERS,
  DEFAULT_USER_LIST_FILTERS,
  UsersListFiltersPanel,
  countActiveFilters,
  type UserListFilters,
} from "./users-list-filters";
import { UsersTable } from "./users-table";
import { BulkScheduleTypeDialog } from "./bulk-schedule-type-dialog";
import { EmployeeImportDialog } from "./employee-import-dialog";
import type { ScheduleType } from "@/lib/hris/schedule-type";
import { todayWib } from "@/lib/dates";
import { NAV_FROM_EMPLOYEES } from "@/lib/iam/nav-context";
import type { UserEmployeeItem } from "@/lib/users/user-mapper";

interface UsersListPageProps {
  /**
   * "directory" — menu Karyawan (/dashboard/employees): direktori karyawan HRIS.
   * "accounts" — menu Manajemen User (/dashboard/settings/users): akun login,
   * role, dan akses aplikasi (reset password, buat akun).
   */
  variant?: "directory" | "accounts";
}

export function UsersListPage({ variant = "directory" }: UsersListPageProps) {
  const isAccountsView = variant === "accounts";
  const showAppActions = isAccountsView;
  const router = useRouter();
  const { hasPrefix } = useIamAccess();
  const canManageUsers = hasPrefix(IAM.settingsUsers);
  const canResetPassword = canManageUsers;
  const canCreateAccount = canManageUsers;
  const canLoginAs = !isAccountsView && canManageUsers;

  const [searchQuery, setSearchQuery] = useState("");
  const [search, setSearch] = useState("");
  const defaultFilters = isAccountsView ? DEFAULT_USER_LIST_FILTERS : DEFAULT_EMPLOYEE_DIRECTORY_FILTERS;
  const [filters, setFilters] = useState<UserListFilters>(defaultFilters);
  const [filterOpen, setFilterOpen] = useState(false);
  const [page, setPage] = useState(1);
  const [resetTarget, setResetTarget] = useState<ResetPasswordTarget | null>(null);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [createAccountTarget, setCreateAccountTarget] = useState<UserEmployeeItem | null>(null);
  const [loginAsEmployeeId, setLoginAsEmployeeId] = useState<string | null>(null);
  // Ubah massal tipe jadwal: hanya direktori Karyawan untuk HR.
  const canBulkSchedule = !isAccountsView && hasPrefix(IAM.hrisKepegawaian);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkOpen, setBulkOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);

  const perPage = 15;

  const listParams = {
    search: search.trim() || undefined,
    department_id: filters.department !== "all" ? filters.department : undefined,
    employment_status: filters.status !== "all" ? filters.status : undefined,
    is_active: filters.active !== "all" ? filters.active : undefined,
    is_access_app: filters.access !== "all" ? filters.access : undefined,
    role: filters.role || undefined,
    schedule_type:
      filters.scheduleType === "shift" || filters.scheduleType === "non_shift"
        ? (filters.scheduleType as ScheduleType)
        : undefined,
    page,
    limit: perPage,
    sort_by: "full_name",
    sort_order: "asc" as const,
  };

  const { data, isLoading, isError, refetch: refetchList } = useUserList(listParams);
  const { data: departments = [] } = useDepartmentList();
  const { data: stats = { total: 0, active: 0, withAccess: 0 } } = useUserDirectoryStats();
  const rows = data?.data ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / perPage));

  const activeFilterCount = countActiveFilters(filters, defaultFilters);
  const isFilterActive = activeFilterCount > 0;

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setSearch(searchQuery.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timeout);
  }, [searchQuery]);

  function handleFilterChange(patch: Partial<UserListFilters>) {
    setFilters((prev) => ({ ...prev, ...patch }));
    setPage(1);
  }

  function handleResetFilters() {
    setSearchQuery("");
    setSearch("");
    setFilters(defaultFilters);
    setPage(1);
  }

  function handleResetPassword(row: UserEmployeeItem) {
    setResetTarget({ id: row.id, fullName: row.fullName });
    setResetDialogOpen(true);
  }

  async function handleLoginAs(row: UserEmployeeItem) {
    if (!row.userId || loginAsEmployeeId) return;
    setLoginAsEmployeeId(row.id);
    try {
      const result = await impersonateUser(row.userId);
      toast.success(result.message || `Logged in as ${row.fullName}`);
      // Full reload agar seluruh state (session, layout, menu) mengikuti user baru.
      window.location.href = result.data.role === "super_admin" ? OS_PATH : "/dashboard/me";
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to login as user");
      setLoginAsEmployeeId(null);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col items-start justify-between gap-4 border-b border-gray-200/70 pb-4 sm:flex-row sm:items-center">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">
            {isAccountsView ? "Manajemen User" : "Karyawan"}
          </h1>
          <p className="text-sm text-gray-500">
            {isAccountsView
              ? "Kelola akun login, role, dan akses aplikasi"
              : "Direktori karyawan HRIS"}{" "}
            — {total} total
          </p>
        </div>
        {!isAccountsView && (
          <Link href={EMPLOYEES_ROUTES.insert}>
            <Button className="h-10 w-full gap-2 rounded-lg bg-pink-600 px-3 text-sm font-semibold text-white shadow-sm hover:bg-pink-700 sm:w-auto">
              <PlusIcon className="h-4 w-4" />
              Add Employee
            </Button>
          </Link>
        )}
      </div>

      {!isAccountsView && <ContractExpiryBanner from={NAV_FROM_EMPLOYEES} />}
      {!isAccountsView && <NoManagerCard />}

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {[
          { label: "Total Employees", value: stats.total, icon: UserGroupIcon },
          { label: "Active", value: stats.active, icon: UserGroupIcon },
          { label: "App Access", value: stats.withAccess, icon: UserGroupIcon },
          {
            label: "Departments",
            value: departments.length,
            icon: BuildingOfficeIcon,
          },
        ].map((item) => (
          <div
            key={item.label}
            className="rounded-xl border border-gray-200/70 bg-white px-4 py-3 shadow-sm"
          >
            <div className="flex items-center gap-2 text-xs font-medium text-gray-500">
              <item.icon className="h-3.5 w-3.5 text-pink-500" />
              {item.label}
            </div>
            <p className="mt-1 text-2xl font-bold text-gray-900">{item.value}</p>
          </div>
        ))}
      </div>

      <PurchasingListSection
        icon={Users}
        title={isAccountsView ? "Daftar User" : "Employee List"}
        description={
          isAccountsView
            ? "Akun login karyawan, role, dan status akses aplikasi."
            : "Track employee records and employment status."
        }
        toolbar={
          <div className="flex w-full flex-col gap-3 sm:w-auto md:flex-row md:items-center">
            <label className="relative w-full md:w-80">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <Input
                placeholder="Search name, employee ID, email..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="h-10 bg-white pl-10 pr-10 text-sm focus:border-pink-400 focus:ring-2 focus:ring-pink-100"
              />
              {searchQuery ? (
                <button
                  type="button"
                  onClick={() => setSearchQuery("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 transition-colors hover:text-gray-700"
                  aria-label="Clear search"
                >
                  <X className="h-4 w-4" />
                </button>
              ) : null}
            </label>

            <Button
              type="button"
              variant="outline"
              onClick={() => setFilterOpen((open) => !open)}
              className={
                isFilterActive
                  ? "h-10 gap-2 rounded-lg border-pink-600 bg-pink-600 px-3 text-sm font-semibold !text-white shadow-sm hover:!border-pink-700 hover:!bg-pink-700 hover:!text-white [&_*]:!text-white [&_svg]:!text-white"
                  : "h-10 gap-2 rounded-lg border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 shadow-sm hover:!border-pink-200 hover:!bg-pink-50 hover:!text-pink-700"
              }
            >
              <Filter className="h-4 w-4" />
              Filter
              {isFilterActive ? (
                <span className="ml-1 inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-white/20 px-1.5 text-xs text-white">
                  {activeFilterCount}
                </span>
              ) : null}
            </Button>

            {!isAccountsView ? (
              <Button
                type="button"
                variant="outline"
                className="h-10 flex-shrink-0 rounded-lg border-gray-200/80"
                onClick={() => {
                  // Export mengikuti pencarian & filter yang sedang aktif.
                  window.location.href = buildListUrl("/api/hris/employees/export", {
                    search: listParams.search,
                    department_id: listParams.department_id,
                    employment_status: listParams.employment_status,
                    is_active: listParams.is_active,
                    schedule_type: listParams.schedule_type,
                  });
                }}
              >
                Export
              </Button>
            ) : null}
            {canBulkSchedule ? (
              <Button
                type="button"
                variant="outline"
                className="h-10 flex-shrink-0 rounded-lg border-gray-200/80"
                onClick={() => setImportOpen(true)}
              >
                Import
              </Button>
            ) : null}
            {(search || isFilterActive || page > 1) && (
              <Button
                variant="outline"
                onClick={handleResetFilters}
                className="h-10 flex-shrink-0 rounded-lg border-gray-200/80"
              >
                Reset
              </Button>
            )}
          </div>
        }
      >
        {filterOpen ? (
          <UsersListFiltersPanel
            filters={filters}
            departments={departments}
            showRole={showAppActions}
            onChange={handleFilterChange}
          />
        ) : null}

        {isLoading ? (
          <div className="py-14 text-center">
            <div className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-t-pink-500" />
            <p className="mt-2 text-sm text-gray-500">Loading employees...</p>
          </div>
        ) : isError ? (
          <p className="py-14 text-center text-sm text-gray-500">Failed to load employees</p>
        ) : rows.length === 0 ? (
          <div className="py-14 text-center">
            <Users className="mx-auto mb-4 h-12 w-12 text-gray-300" />
            <p className="text-gray-500">No employees found</p>
            <Link href={EMPLOYEES_ROUTES.insert}>
              <Button
                variant="outline"
                className="mt-4 h-10 gap-2 rounded-lg border-pink-200 bg-white px-3 text-sm font-medium text-pink-700 shadow-sm hover:border-pink-200 hover:bg-pink-50 hover:text-pink-700"
              >
                <PlusIcon className="h-4 w-4" />
                Add First Employee
              </Button>
            </Link>
          </div>
        ) : (
          <>
            {canBulkSchedule && selectedIds.size > 0 ? (
              <div className="mx-4 mb-2 flex flex-wrap items-center gap-2 rounded-lg bg-info-soft px-3 py-2 text-sm text-info">
                <span>{selectedIds.size} karyawan dipilih</span>
                <Button size="sm" onClick={() => setBulkOpen(true)}>
                  Set Shift / Non-Shift ({selectedIds.size})
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSelectedIds(new Set())}>
                  Batal pilih
                </Button>
              </div>
            ) : null}
            <div className="px-4">
              <UsersTable
                selectedIds={canBulkSchedule ? selectedIds : undefined}
                onSelectedChange={canBulkSchedule ? setSelectedIds : undefined}
                rows={rows}
                onView={(id) => router.push(EMPLOYEES_ROUTES.detail(id))}
                onEdit={(id) => router.push(EMPLOYEES_ROUTES.edit(id))}
                onResetPassword={
                  showAppActions && canResetPassword ? handleResetPassword : undefined
                }
                onCreateAccount={
                  showAppActions && canCreateAccount ? setCreateAccountTarget : undefined
                }
                onLoginAs={canLoginAs ? handleLoginAs : undefined}
                loginAsEmployeeId={loginAsEmployeeId}
                showAppActions={showAppActions}
              />
            </div>
            <PurchasingTablePagination
              page={page}
              totalPages={totalPages}
              totalItems={total}
              pageSize={perPage}
              onPageChange={setPage}
            />
          </>
        )}
      </PurchasingListSection>

      <ResetPasswordDialog
        target={resetTarget}
        open={resetDialogOpen}
        onOpenChange={(open) => {
          setResetDialogOpen(open);
          if (!open) setResetTarget(null);
        }}
      />

      {importOpen ? (
        <EmployeeImportDialog open onOpenChange={setImportOpen} onDone={() => void refetchList()} />
      ) : null}

      {bulkOpen ? (
        <BulkScheduleTypeDialog
          open
          ids={[...selectedIds]}
          today={todayWib()}
          onOpenChange={setBulkOpen}
          onDone={() => {
            setSelectedIds(new Set());
            void refetchList();
          }}
        />
      ) : null}

      {createAccountTarget && (
        <CreateAccountDialog
          employee={{
            id: createAccountTarget.id,
            full_name: createAccountTarget.fullName,
            email: createAccountTarget.email ?? "",
          }}
          onClose={() => setCreateAccountTarget(null)}
        />
      )}
    </div>
  );
}

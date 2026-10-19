"use client";

import { use, useState, type ReactNode } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  ArrowLeftIcon,
  ArrowPathIcon,
  BriefcaseIcon,
  BuildingStorefrontIcon,
  CalendarDaysIcon,
  ClockIcon,
  DocumentTextIcon,
  UserCircleIcon,
} from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { EmployeePlacementsTab } from "@/features/hris/placements";
import { useIamAccess } from "@/components/iam/iam-access-provider";
import { IAM } from "@/lib/iam/prefixes";
import { EMPLOYEES_ROUTES } from "../constants";
import { useHRISEmployeeDetail } from "../queries";
import { CreateAccountDialog } from "./create-account-dialog";
import { EmployeeAttendanceTab } from "./detail/employee-attendance-tab";
import { EmployeeInfoTab } from "./detail/employee-info-tab";
import { EmployeeProfileHeader } from "./detail/employee-profile-header";
import { EmploymentHistoryTab } from "./detail/employment-history-tab";
import { LeaveBalanceTab } from "./detail/leave-balance-tab";
import { EmployeeContractsTab } from "./employee-contracts-tab";
import { EmployeeDocumentsTab } from "./employee-documents-tab";
import { EmployeeLifecycleTab } from "./employee-lifecycle-tab";
import { EmployeeShiftsTab } from "./employee-shifts-tab";
import { ScheduleTypeDialog } from "./schedule-type-dialog";
import { ResignDialog } from "./resign-dialog";
import { todayWib } from "@/lib/dates";
import { ResetPasswordDialog } from "./reset-password-dialog";

const TABS = [
  { key: "info", label: "Personal Info", icon: UserCircleIcon },
  { key: "lifecycle", label: "Lifecycle", icon: ArrowPathIcon },
  { key: "contracts", label: "Kontrak", icon: BriefcaseIcon },
  { key: "shifts", label: "Jadwal Shift", icon: ClockIcon },
  { key: "placements", label: "Penempatan", icon: BuildingStorefrontIcon },
  { key: "employment", label: "Employment History", icon: BriefcaseIcon },
  { key: "documents", label: "Dokumen", icon: DocumentTextIcon },
  { key: "attendance", label: "Attendance", icon: ClockIcon },
  { key: "leave", label: "Leave Balance", icon: CalendarDaysIcon },
] as const;

type Tab = (typeof TABS)[number]["key"];

const isTab = (value: string | null): value is Tab => TABS.some((t) => t.key === value);

export function UserDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const canManageUsers = useIamAccess().hasPrefix(IAM.settingsUsers);

  // deep-link tab via ?tab=contracts (dipakai banner pengingat kontrak)
  const searchParams = useSearchParams();
  const [activeTab, setActiveTab] = useState<Tab>(() => {
    const requested = searchParams.get("tab");
    return isTab(requested) ? requested : "info";
  });
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [createAccountOpen, setCreateAccountOpen] = useState(false);
  const [scheduleTypeOpen, setScheduleTypeOpen] = useState(false);
  const [resignOpen, setResignOpen] = useState(false);
  const isHR = useIamAccess().hasPrefix(IAM.hrisKepegawaian);

  const { data: employee, isLoading, refetch: refetchEmployee } = useHRISEmployeeDetail(id);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin w-8 h-8 border-2 border-gray-300 border-t-blue-500 rounded-full" />
      </div>
    );
  }

  if (!employee) {
    return (
      <div className="text-center py-20">
        <p className="text-gray-500">Employee not found</p>
        <Button className="mt-4" onClick={() => router.back()}>
          Back
        </Button>
      </div>
    );
  }

  const tabContent: Record<Tab, ReactNode> = {
    info: <EmployeeInfoTab employee={employee} />,
    lifecycle: <EmployeeLifecycleTab employeeId={id} />,
    contracts: <EmployeeContractsTab employeeId={id} />,
    shifts: <EmployeeShiftsTab employeeId={id} scheduleType={employee.schedule_type ?? "shift"} />,
    placements: <EmployeePlacementsTab employeeId={id} employeeName={employee.full_name} />,
    employment: <EmploymentHistoryTab employeeId={id} />,
    documents: <EmployeeDocumentsTab employeeId={id} />,
    attendance: <EmployeeAttendanceTab employeeId={id} />,
    leave: <LeaveBalanceTab employeeId={id} />,
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2 text-sm text-gray-500">
        <button onClick={() => router.push(EMPLOYEES_ROUTES.list)} className="hover:text-gray-900">
          Employee Directory
        </button>
        <span>/</span>
        <span className="text-gray-900 font-medium">{employee.full_name || "Employee Detail"}</span>
      </div>

      <div className="flex items-center gap-4">
        <Button variant="ghost" size="sm" onClick={() => router.back()} className="gap-1">
          <ArrowLeftIcon className="w-4 h-4" /> Back
        </Button>
      </div>

      <EmployeeProfileHeader
        employee={employee}
        canManageUsers={canManageUsers}
        onCreateAccount={() => setCreateAccountOpen(true)}
        onResetPassword={() => setResetDialogOpen(true)}
        onChangeScheduleType={isHR ? () => setScheduleTypeOpen(true) : undefined}
        onResign={isHR ? () => setResignOpen(true) : undefined}
      />

      {resignOpen ? (
        <ResignDialog
          open
          employeeId={id}
          employeeName={employee.full_name}
          joinDate={employee.join_date.slice(0, 10)}
          today={todayWib()}
          onOpenChange={setResignOpen}
          onDone={() => void refetchEmployee()}
        />
      ) : null}

      {scheduleTypeOpen ? (
        <ScheduleTypeDialog
          open
          employeeId={id}
          employeeName={employee.full_name}
          today={todayWib()}
          onOpenChange={setScheduleTypeOpen}
          onDone={() => void refetchEmployee()}
        />
      ) : null}

      <div className="border-b border-gray-200">
        <div className="flex gap-1 overflow-x-auto">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium whitespace-nowrap border-b-2 transition-colors ${
                activeTab === tab.key
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {tabContent[activeTab]}

      <ResetPasswordDialog
        target={{ id, fullName: employee.full_name }}
        open={resetDialogOpen}
        onOpenChange={setResetDialogOpen}
      />

      {createAccountOpen && (
        <CreateAccountDialog
          employee={{
            id,
            full_name: employee.full_name,
            email: employee.email,
          }}
          onClose={() => setCreateAccountOpen(false)}
        />
      )}
    </div>
  );
}

"use client";

import { BriefcaseIcon } from "@heroicons/react/24/outline";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { formatDate } from "@/lib/format";
import { STATUS_LABELS } from "../../constants";
import { useEmploymentHistory } from "../../queries";
import { EmptyTabCard, TabSpinner } from "./tab-states";

const HISTORY_TYPE_LABELS: Record<string, string> = {
  hire: "Hired",
  promotion: "Promotion",
  transfer: "Transfer",
  demotion: "Demotion",
  status_change: "Status Change",
  salary_change: "Salary Change",
  resign: "Resign",
  terminated: "Diberhentikan",
  end_of_contract: "Kontrak Berakhir",
  contract_activated: "Kontrak Diaktifkan",
  rehire: "Diaktifkan Kembali",
};

const HISTORY_COLORS: Record<string, string> = {
  hire: "bg-green-100 text-green-700",
  promotion: "bg-blue-100 text-blue-700",
  transfer: "bg-purple-100 text-purple-700",
  demotion: "bg-orange-100 text-orange-700",
  status_change: "bg-yellow-100 text-yellow-700",
  salary_change: "bg-teal-100 text-teal-700",
  resign: "bg-red-100 text-red-700",
  terminated: "bg-red-100 text-red-700",
  end_of_contract: "bg-orange-100 text-orange-700",
  contract_activated: "bg-blue-100 text-blue-700",
  rehire: "bg-green-100 text-green-700",
};

function Change({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div>
      <p className="text-xs text-gray-400">{label}</p>
      <p className={strong ? "text-gray-700 font-medium" : "text-gray-700"}>{value}</p>
    </div>
  );
}

export function EmploymentHistoryTab({ employeeId }: { employeeId: string }) {
  const { data: history = [], isLoading } = useEmploymentHistory(employeeId);
  if (isLoading) return <TabSpinner />;

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h3 className="font-semibold text-gray-700">Employment History</h3>
      </div>
      {history.length === 0 ? (
        <EmptyTabCard icon={BriefcaseIcon}>No employment history yet</EmptyTabCard>
      ) : (
        <div className="relative">
          <div className="absolute left-5 top-0 bottom-0 w-0.5 bg-gray-200" />
          <div className="space-y-4">
            {history.map((h) => (
              <div key={h.id} className="relative pl-12">
                <div className="absolute left-3.5 top-3 w-3 h-3 rounded-full border-2 border-white bg-blue-500 shadow" />
                <Card>
                  <CardContent className="p-4">
                    <div className="flex flex-wrap items-center gap-2 mb-2">
                      <Badge
                        className={HISTORY_COLORS[h.change_type] || "bg-gray-100 text-gray-600"}
                      >
                        {HISTORY_TYPE_LABELS[h.change_type] || h.change_type}
                      </Badge>
                      <span className="text-xs text-gray-500">{formatDate(h.effective_date)}</span>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
                      {h.prev_department && (
                        <Change label="From Department" value={h.prev_department.name} />
                      )}
                      {h.new_department && (
                        <Change label="To Department" value={h.new_department.name} strong />
                      )}
                      {h.prev_job_title && (
                        <Change label="From Job Title" value={h.prev_job_title.title} />
                      )}
                      {h.new_job_title && (
                        <Change label="To Job Title" value={h.new_job_title.title} strong />
                      )}
                      {h.prev_employment_status && (
                        <Change
                          label="From Status"
                          value={STATUS_LABELS[h.prev_employment_status]}
                        />
                      )}
                      {h.new_employment_status && (
                        <Change
                          label="To Status"
                          value={STATUS_LABELS[h.new_employment_status]}
                          strong
                        />
                      )}
                    </div>
                    {(h.reason || h.notes) && (
                      <p className="mt-2 text-xs text-gray-500 italic">
                        &ldquo;{h.reason || h.notes}&rdquo;
                      </p>
                    )}
                  </CardContent>
                </Card>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

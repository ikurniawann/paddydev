import { BriefcaseIcon } from "@heroicons/react/24/outline";
import { FormFieldLabel, formComboboxClassName } from "@/components/layout/form-field";
import { FormSectionCard } from "@/components/layout/form-section-card";
import { Combobox } from "@/components/ui/combobox";
import { SCHEDULE_TYPE_OPTIONS, scheduleTypeLabel } from "@/lib/hris/schedule-type";
import type { UserEmployeeFormValues } from "@/lib/hris/users-form";
import type { UserFormLookups } from "../../types";
import { TextField } from "./text-field";

interface EmploymentSectionProps {
  form: UserEmployeeFormValues;
  lookups: UserFormLookups | undefined;
  onChange: (patch: Partial<UserEmployeeFormValues>) => void;
  /** Edit: tipe jadwal hanya ditampilkan (diubah lewat dialog di detail karyawan). */
  isEdit?: boolean;
}

export function EmploymentSection({ form, lookups, onChange, isEdit = false }: EmploymentSectionProps) {
  const departments = lookups?.departments ?? [];
  const selectedDeptName = departments.find((d) => d.id === form.department_id)?.name;
  // posisi hanya dari departemen terpilih
  const positionOptions = form.department_id
    ? (lookups?.positions ?? [])
        .filter((p) => p.department === selectedDeptName)
        .map((p) => ({
          value: p.id,
          label: p.title,
          description: p.department,
        }))
    : [];

  return (
    <FormSectionCard
      icon={BriefcaseIcon}
      title="Employment Details"
      description="Internal organization placement and employment status."
    >
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <TextField
          label="Join Date"
          type="date"
          required
          value={form.join_date}
          onChange={(join_date) => onChange({ join_date })}
        />
        <div>
          <FormFieldLabel required>Employment Status</FormFieldLabel>
          <Combobox
            options={(lookups?.employmentStatuses ?? []).map((o) => ({
              value: o.code,
              label: o.name,
            }))}
            value={form.employment_status}
            onChange={(employment_status) => onChange({ employment_status })}
            placeholder="Select"
            searchPlaceholder="Search status..."
            emptyMessage="No status found"
            className={formComboboxClassName}
          />
        </div>
        <div>
          <FormFieldLabel required>Tipe Jadwal</FormFieldLabel>
          {isEdit ? (
            <p className="flex h-10 items-center text-sm text-gray-700">
              {scheduleTypeLabel(form.schedule_type)}
              <span className="ml-2 text-xs text-gray-400">(ubah lewat &quot;Ubah tipe jadwal&quot; di detail karyawan)</span>
            </p>
          ) : (
            <div role="radiogroup" aria-label="Tipe Jadwal" className="flex h-10 items-center gap-4 text-sm">
              {SCHEDULE_TYPE_OPTIONS.map((o) => (
                <label key={o.value} className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="schedule_type"
                    checked={form.schedule_type === o.value}
                    onChange={() => onChange({ schedule_type: o.value })}
                  />
                  {o.label}
                </label>
              ))}
            </div>
          )}
        </div>
        <div>
          <FormFieldLabel>Department</FormFieldLabel>
          <Combobox
            options={departments.map((d) => ({ value: d.id, label: d.name }))}
            value={form.department_id}
            onChange={(department_id) => onChange({ department_id, job_title_id: "" })}
            placeholder="Select"
            searchPlaceholder="Search department..."
            emptyMessage="No department found"
            allowClear
            className={formComboboxClassName}
          />
        </div>
        <div>
          <FormFieldLabel>Position</FormFieldLabel>
          <Combobox
            options={positionOptions}
            value={form.job_title_id}
            onChange={(job_title_id) => onChange({ job_title_id })}
            placeholder="Select"
            searchPlaceholder="Search position..."
            emptyMessage="No position found"
            disabled={!form.department_id}
            allowClear
            className={formComboboxClassName}
          />
        </div>
        <div>
          <FormFieldLabel>Section</FormFieldLabel>
          <Combobox
            options={(lookups?.sections ?? []).map((s) => ({
              value: s.id,
              label: s.name,
            }))}
            value={form.section_id}
            onChange={(section_id) => onChange({ section_id })}
            placeholder="Select"
            searchPlaceholder="Search section..."
            emptyMessage="No section found"
            allowClear
            className={formComboboxClassName}
          />
        </div>
        <div>
          <FormFieldLabel>Manager</FormFieldLabel>
          <Combobox
            options={(lookups?.managers ?? []).map((m) => ({
              value: m.id,
              label: m.full_name,
              description: m.nip,
            }))}
            value={form.reporting_to}
            onChange={(reporting_to) => onChange({ reporting_to })}
            placeholder="Select"
            searchPlaceholder="Search manager..."
            emptyMessage="No manager found"
            allowClear
            className={formComboboxClassName}
          />
        </div>
      </div>
    </FormSectionCard>
  );
}

import { apiGet, apiPost, apiPatch, apiDelete } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type {
  JobDepartmentOption,
  JobOpening,
  JobOpeningPayload,
  JobPositionOption,
} from "@/lib/recruitment/job-portal-form";

export const fetchJobOpenings = () =>
  apiGet<{ data: JobOpening[] }>("/api/hris/job-openings").then((res) => res.data || []);

export const fetchJobPositions = () =>
  apiGet<{ data: JobPositionOption[] }>("/api/master/positions").then((res) => res.data || []);

export const fetchJobDepartments = () =>
  apiGet<{ data: JobDepartmentOption[] }>("/api/master/departments").then((res) =>
    (res.data || []).filter((department) => department.is_active)
  );

export const saveJobOpening = (payload: JobOpeningPayload, id?: string) =>
  id
    ? apiPatch<{ data: JobOpening }>(`/api/hris/job-openings/${id}`, payload)
    : apiPost<{ data: JobOpening }>("/api/hris/job-openings", payload);

export const deleteJobOpening = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/job-openings/${id}`, reason));

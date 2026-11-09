import { apiGet } from "@/lib/api-client";
import { fetchAllEmployees } from "@/lib/hris/fetch-all-employees";
import type { OrgEmployee } from "@/lib/hris/org-chart";
import type { OrgDepartment } from "./types";

export const fetchOrgEmployees = () => fetchAllEmployees<OrgEmployee>("is_active=true");

export const fetchOrgDepartments = () =>
  apiGet<{ data: (OrgDepartment & { is_active: boolean })[] }>("/api/master/departments").then((res) =>
    (res.data ?? []).filter((d) => d.is_active)
  );

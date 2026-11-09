"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchAllEmployees } from "@/lib/hris/fetch-all-employees";
import { employeeComboOptions, type EmployeeBrief } from "@/lib/hris/loans-view";

const fetchActiveEmployeeOptions = () =>
  fetchAllEmployees<EmployeeBrief>("is_active=true").then(employeeComboOptions);

/** Pilihan combobox karyawan aktif (form pinjaman & lembur). */
export const useEmployeeOptions = () =>
  useQuery({ queryKey: ["hris", "employee-options"], queryFn: fetchActiveEmployeeOptions });

"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import type { CreateUserEmployeeInput, UpdateUserEmployeeInput } from "@/lib/users/schemas";
import {
  createEmployeeContract,
  createEmployeeDocument,
  createUser,
  deleteContractSignedDocument,
  deleteEmployeeContract,
  deleteEmployeeDocument,
  patchEmployeeContract,
  resetUserPassword,
  saveEmployeeShiftPattern,
  updateUser,
  uploadContractSignedDocument,
  type ContractActionInput,
  type CreateContractInput,
} from "./api";
import { usersQueryKeys } from "./query-keys";
import type { EmployeeDocumentInput } from "./types";

function useInvalidateUsers() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: usersQueryKeys.all });
}

export function useCreateEmployeeContract(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateContractInput) => createEmployeeContract(employeeId, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.contracts(employeeId) });
    },
  });
}

export function useContractAction(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ contractId, ...payload }: ContractActionInput & { contractId: string }) =>
      patchEmployeeContract(contractId, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.contracts(employeeId) });
      // aktivasi kontrak mengubah employment_status karyawan
      qc.invalidateQueries({
        queryKey: usersQueryKeys.hrisEmployee(employeeId),
      });
      qc.invalidateQueries({
        queryKey: usersQueryKeys.employmentHistory(employeeId),
      });
    },
  });
}

export function useDeleteEmployeeContract(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus draft kontrak", (contractId: string, reason) => deleteEmployeeContract(contractId, reason)),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.contracts(employeeId) });
    },
  });
}

export function useUploadContractSignedDocument(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ contractId, file }: { contractId: string; file: File }) =>
      uploadContractSignedDocument(contractId, file),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.contracts(employeeId) });
    },
  });
}

export function useDeleteContractSignedDocument(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus scan kontrak bertanda tangan", (contractId: string, reason) => deleteContractSignedDocument(contractId, reason)),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.contracts(employeeId) });
    },
  });
}

export function useCreateUser() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: (payload: CreateUserEmployeeInput) => createUser(payload),
    onSuccess: invalidate,
  });
}

export function useUpdateUser() {
  const invalidate = useInvalidateUsers();
  return useMutation({
    mutationFn: ({ id, ...payload }: UpdateUserEmployeeInput & { id: string }) =>
      updateUser(id, payload),
    onSuccess: invalidate,
  });
}

export function useResetUserPassword() {
  return useMutation({
    mutationFn: (id: string) => resetUserPassword(id),
  });
}

export function useCreateEmployeeDocument(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: Omit<EmployeeDocumentInput, "employee_id">) =>
      createEmployeeDocument({ employee_id: employeeId, ...payload }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.documents(employeeId) });
    },
  });
}

export function useDeleteEmployeeDocument(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus dokumen karyawan", (docId: string, reason) => deleteEmployeeDocument(docId, reason)),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersQueryKeys.documents(employeeId) });
    },
  });
}

export function useSaveEmployeeShiftPattern(employeeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: Parameters<typeof saveEmployeeShiftPattern>[1]) =>
      saveEmployeeShiftPattern(employeeId, payload),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: usersQueryKeys.shiftSchedule(employeeId),
      });
      // Pola mingguan menentukan jadwal: grid jadwal & halaman penempatan ikut segar.
      qc.invalidateQueries({ queryKey: ["hris", "schedules"] });
      qc.invalidateQueries({ queryKey: ["hris", "placements"] });
    },
  });
}

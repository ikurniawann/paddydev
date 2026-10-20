"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { essQueryKeys } from "./query-keys";
import {
  decideOvertime,
  markAnnouncementRead,
  submitClock,
  submitLoan,
  submitMyLeave,
  submitOvertime,
} from "./api";
import type {
  EssAnnouncementItem,
  EssClockPayload,
  EssLeaveForm,
  EssLoanPayload,
  EssOvertimeDecision,
  EssOvertimeForm,
} from "./types";

// Mutasi pengajuan memakai retry: false supaya POST tidak terkirim ganda.

/** Clock-in/out: segarkan absensi hari ini, beranda, dan kalender absensi. */
export function useSubmitClock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: EssClockPayload) => submitClock(payload),
    retry: false,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: essQueryKeys.todayAttendance() });
      void qc.invalidateQueries({ queryKey: essQueryKeys.beranda() });
      void qc.invalidateQueries({ queryKey: attendanceQueryKeys.calendarAll() });
    },
  });
}

export function useSubmitMyLeave() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ form, attachment }: { form: EssLeaveForm; attachment?: string }) =>
      submitMyLeave(form, attachment),
    retry: false,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: essQueryKeys.leaves() });
      qc.invalidateQueries({ queryKey: ["hris", "leaves", "quota"] });
    },
  });
}

export function useSubmitOvertime() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (form: EssOvertimeForm) => submitOvertime(form),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: essQueryKeys.overtime() }),
  });
}

export function useDecideOvertime() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; action: EssOvertimeDecision; rejectionReason?: string }) =>
      decideOvertime(vars.id, vars.action, vars.rejectionReason),
    onSuccess: () => qc.invalidateQueries({ queryKey: essQueryKeys.overtime() }),
  });
}

export function useSubmitLoan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: EssLoanPayload) => submitLoan(payload),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: essQueryKeys.loans() }),
  });
}

/** Tandai dibaca (fire-and-forget) dan perbarui badge feed secara lokal. */
export function useMarkAnnouncementRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => markAnnouncementRead(id),
    onMutate: (id) => {
      qc.setQueryData<EssAnnouncementItem[]>(essQueryKeys.announcements(), (prev) =>
        prev?.map((item) => (item.id === id ? { ...item, is_read: true } : item))
      );
    },
  });
}

export const teamApprovalsQueryKeys = {
  all: ["hris", "team-approvals"] as const,
  // Di bawah "all": keputusan yang meng-invalidate antrean ikut menyegarkan riwayat.
  history: () => [...teamApprovalsQueryKeys.all, "history"] as const,
};

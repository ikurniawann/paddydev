import type { LetterListParams } from "./types";

export const letterKeys = {
  all: ["hris", "disciplinary-letters"] as const,
  list: (p: LetterListParams) => ["hris", "disciplinary-letters", "list", p.employeeId ?? "", p.includeRevoked] as const,
  mine: ["hris", "disciplinary-letters", "mine"] as const,
  lateness: (employeeId: string) => ["hris", "disciplinary-letters", "lateness", employeeId] as const,
};

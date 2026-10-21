export const essQueryKeys = {
  all: ["hris", "ess"] as const,
  me: () => ["hris", "ess", "me"] as const,
  beranda: () => ["hris", "ess", "beranda"] as const,
  todayAttendance: () => ["hris", "ess", "today-attendance"] as const,
  leaves: () => ["hris", "ess", "leaves"] as const,
  overtime: () => ["hris", "ess", "overtime"] as const,
  overtimeDayType: (date: string) => ["hris", "ess", "overtime-day-type", date] as const,
  loans: () => ["hris", "ess", "loans"] as const,
  payslips: () => ["hris", "ess", "payslips"] as const,
  announcements: () => ["hris", "ess", "announcements"] as const,
  announcement: (id: string) => ["hris", "ess", "announcements", id] as const,
  team: () => ["hris", "ess", "team"] as const,
};

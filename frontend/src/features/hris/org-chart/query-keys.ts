export const orgChartQueryKeys = {
  all: ["hris", "org-chart"] as const,
  employees: () => ["hris", "org-chart", "employees"] as const,
  departments: () => ["hris", "org-chart", "departments"] as const,
  tree: (division: string, region: string) => ["hris", "org-chart", "tree", division, region] as const,
  validation: () => ["hris", "org-chart", "validation"] as const,
  gap: () => ["hris", "org-chart", "gap"] as const,
};

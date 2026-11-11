export const orgChartQueryKeys = {
  all: ["hris", "org-chart"] as const,
  employees: () => ["hris", "org-chart", "employees"] as const,
  departments: () => ["hris", "org-chart", "departments"] as const,
  versions: () => ["hris", "org-chart", "versions"] as const,
  tree: (version: string, division: string, region: string) =>
    ["hris", "org-chart", "tree", version, division, region] as const,
  validation: (version: string) => ["hris", "org-chart", "validation", version] as const,
  gap: (version: string) => ["hris", "org-chart", "gap", version] as const,
  vacancy: (version: string, division: string, region: string, state: string) =>
    ["hris", "org-chart", "vacancy", version, division, region, state] as const,
};

export const shiftQueryKeys = {
  all: ["hris", "shifts"] as const,
  list: () => ["hris", "shifts", "list"] as const,
};

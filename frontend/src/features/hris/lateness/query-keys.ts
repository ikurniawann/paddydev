export const latenessQueryKeys = {
  all: ["hris", "lateness"] as const,
  mine: (back: number) => ["hris", "lateness", "me", back] as const,
  team: (back: number) => ["hris", "lateness", "team", back] as const,
};

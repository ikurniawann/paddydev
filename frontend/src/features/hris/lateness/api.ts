import { apiGet, buildListUrl } from "@/lib/api-client";
import type { MyLateness, TeamLateness } from "./types";

export const fetchMyLateness = (back = 0) =>
  apiGet<{ data: MyLateness }>(buildListUrl("/api/hris/me/lateness", { back: back || undefined })).then((res) => res.data);

export const fetchTeamLateness = (back = 0) =>
  apiGet<{ data: TeamLateness }>(buildListUrl("/api/hris/me/team-lateness", { back: back || undefined })).then(
    (res) => res.data
  );

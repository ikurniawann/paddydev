"use client";

import { useQuery } from "@tanstack/react-query";
import type { InboxSection } from "@/lib/desktop/inbox";

const INBOX_KEY = ["os-desktop", "inbox"] as const;

/** Kotak keputusan panel Hari Ini; dimuat ulang setiap panel dibuka. */
export function useDesktopInbox() {
  return useQuery({
    queryKey: INBOX_KEY,
    queryFn: async (): Promise<InboxSection[]> => {
      const res = await fetch("/api/desktop/inbox");
      const json = res.ok ? await res.json() : null;
      return Array.isArray(json?.data?.sections) ? json.data.sections : [];
    },
    staleTime: 0,
    retry: false,
  });
}

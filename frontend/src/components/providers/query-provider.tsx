"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { useState } from "react";
import { DeleteReasonHost } from "@/components/hris/delete-reason-host";

export default function QueryProvider({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        // Data dianggap fresh selama 2 menit
        staleTime: 1000 * 60 * 2,
        // Cache dipertahankan selama 5 menit
        gcTime: 1000 * 60 * 5,
        // Tidak refetch saat window focus (hemat bandwidth)
        refetchOnWindowFocus: false,
        // Retry 2x jika gagal
        retry: 2,
        retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 30000),
      },
      mutations: {
        // Retry 1x untuk mutations
        retry: 1,
      },
    },
  }));

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      <DeleteReasonHost />
      {/* Devtools hanya muncul di development */}
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  );
}

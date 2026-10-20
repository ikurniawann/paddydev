"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { deleteAnnouncement, saveAnnouncement, uploadAnnouncementCover } from "./api";
import { announcementQueryKeys } from "./query-keys";
import type { AnnouncementPayload } from "./types";

export function useSaveAnnouncement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ payload, id }: { payload: AnnouncementPayload; id?: string }) =>
      saveAnnouncement(payload, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: announcementQueryKeys.list() }),
  });
}

export function useDeleteAnnouncement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus pengumuman", (id: string, reason) => deleteAnnouncement(id, reason)),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: announcementQueryKeys.list() }),
  });
}

export const useUploadAnnouncementCover = () => useMutation({ mutationFn: uploadAnnouncementCover });

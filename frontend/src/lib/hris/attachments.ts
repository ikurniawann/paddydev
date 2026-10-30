/** Path file privat → URL, tiap segmen di-encode (nama file bisa berspasi). */
export const privateFileHref = (base: string, path: string) =>
  `${base}/${path.split("/").map(encodeURIComponent).join("/")}`;

/** Lampiran cuti: pemohon, atasan langsung, dan HR/pemegang approve Cuti (EPIC-072 R7). */
export const leaveAttachmentHref = (path: string) => privateFileHref("/api/hris/leaves/attachment", path);

// Banner server-side untuk paddydev.reddie.id setelah cutover: data di sini tidak
// lagi dipakai production. Aktif hanya bila STAGING_BANNER_PRODUCTION_URL diisi.
export function StagingBanner({ productionUrl }: { productionUrl?: string }) {
  if (!productionUrl) return null;
  let url: URL;
  try {
    url = new URL(productionUrl);
  } catch {
    return null;
  }
  if (url.protocol !== "https:") return null;
  return (
    <div
      role="status"
      className="sticky top-0 z-[100] bg-amber-500 px-4 py-2 text-center text-sm font-semibold text-black"
    >
      STAGING — data di sini tidak dipakai. Gunakan{" "}
      <a className="underline" href={`${url.origin}/login`}>
        {url.host}
      </a>
    </div>
  );
}

/**
 * Root domain ke /login untuk host staf (PADDY_LOGIN_ROOT_HOSTS, dipisah koma).
 * Hanya GET/HEAD path "/" persis; host lain tetap menampilkan situs publik.
 * Host member (label DNS "member") tidak pernah dialihkan: portalnya di-rewrite
 * ke /member oleh proxy.
 */
export function loginRootRedirect(
  hostHeader: string,
  pathname: string,
  method: string,
  hosts: string | undefined
): boolean {
  if (pathname !== "/" || (method !== "GET" && method !== "HEAD") || !hosts) return false;
  const host = hostHeader.split(":")[0].toLowerCase();
  if (!host || host.split(".").includes("member")) return false;
  return hosts
    .split(",")
    .map((h) => h.trim().toLowerCase())
    .some((h) => h !== "" && h === host);
}

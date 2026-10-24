import { NextResponse, type NextRequest } from "next/server";
import { updateSession } from "@/lib/auth/middleware";
import { goBackendTarget } from "@/lib/backend-routes";
import { legacyRedirectTarget } from "@/lib/legacy-host-redirect";
import { loginRootRedirect } from "@/lib/login-root-redirect";

/**
 * Serves the member portal at its own hostname.
 *
 *   member.paddy.id/            -> /member
 *   member.paddy.id/coins       -> /member/coins
 *
 * The pages keep living under src/app/member; only the public URL changes.
 * dashboard.paddy.id is untouched and still serves /member too,
 * so nothing breaks while the new hostname is being rolled out.
 *
 * Next.js 16 renamed this file convention from `middleware.ts` to `proxy.ts`
 * and the export from `middleware` to `proxy`. The old names are silently
 * ignored on 16 -- the file exists, nothing runs.
 *
 * Requires the tunnel to pass the original Host through: if the Public
 * Hostname entry sets an "HTTP Host Header" override, every request arrives
 * as the dashboard host and this never fires.
 */

/**
 * True when any DNS label of the host is exactly "member". Matching the label
 * rather than a "member." prefix is what makes this work across environments:
 * production is member.paddy.id (label first), while the dev
 * hostname is dev.paddy.member.wit.id (label in the middle). A prefix check
 * silently served the dashboard on dev.
 */
function isMemberHost(hostHeader: string): boolean {
  const hostname = hostHeader.split(":")[0].toLowerCase();
  return hostname.split(".").includes("member");
}

export async function proxy(request: NextRequest) {
  if (request.method === "GET" || request.method === "HEAD") {
    // PADDY_LEGACY_HOST_REDIRECTS; nama env lama (MIZU_…) tetap dibaca sebagai cadangan.
    const redirects = process.env.PADDY_LEGACY_HOST_REDIRECTS ?? process.env.MIZU_LEGACY_HOST_REDIRECTS;
    const target = legacyRedirectTarget(request.headers.get("host") ?? "", request.nextUrl, redirects);
    if (target) return NextResponse.redirect(target, 308);
  }
  // Host staf (mis. paddydev.reddie.id): mengetik domain langsung ke halaman
  // login, bukan situs publik. 307 supaya browser tidak meng-cache permanen.
  if (loginRootRedirect(request.headers.get("host") ?? "", request.nextUrl.pathname, request.method, process.env.PADDY_LOGIN_ROOT_HOSTS)) {
    const url = request.nextUrl.clone();
    url.pathname = "/login";
    url.search = "";
    return NextResponse.redirect(url, 307);
  }
  // Strangler migration: /api prefixes ported to the Go backend go there
  // first, on every host. Method, query, body and cookies travel with the
  // rewrite; the Go service applies the same auth gate as updateSession.
  const goTarget = goBackendTarget(request.nextUrl.pathname, request.method);
  if (goTarget) {
    return NextResponse.rewrite(new URL(`${goTarget}${request.nextUrl.search}`));
  }

  const host = request.headers.get("host") ?? "";
  if (!isMemberHost(host)) return updateSession(request);

  const { pathname } = request.nextUrl;

  // /api/* tidak boleh di-rewrite (portal memanggil /api/member-portal/*
  // dengan path absolut), TAPI tetap lewat updateSession: gerbang auth +
  // validasi Bearer token Open API (EPIC-042) harus berlaku di host member
  // juga — tanpa ini route API tanpa cek sesi terbuka lewat host member.
  // /api/member-portal ada di daftar publik, jadi portal tidak terganggu.
  if (pathname.startsWith("/api")) {
    return updateSession(request);
  }
  // /member/* is already correct -- links in the app emit absolute /member/...
  // paths, so they must not get prefixed twice.
  if (pathname.startsWith("/member")) {
    return NextResponse.next();
  }

  const url = request.nextUrl.clone();
  url.pathname = pathname === "/" ? "/member" : `/member${pathname}`;
  return NextResponse.rewrite(url);
}

export const config = {
  // Static assets are served from the same origin under the member host too,
  // so they must not be rewritten into /member/_next/...
  matcher: "/((?!_next/static|_next/image|favicon.ico).*)",
};

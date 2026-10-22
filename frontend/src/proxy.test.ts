import { describe, expect, test, vi } from "vitest";
import { NextRequest } from "next/server";
import { proxy } from "./proxy";

// proxy jadi async sejak memanggil updateSession — test lama membaca headers
// dari Promise dan gagal semua; kini di-await (perbaikan 2026-08-23).
async function call(host: string, path: string) {
  const req = new NextRequest(new URL(`https://${host}${path}`), {
    headers: { host },
  });
  const res = await proxy(req);
  const rewritten = res.headers.get("x-middleware-rewrite");
  return rewritten ? new URL(rewritten).pathname : null; // null = passed through
}

const MEMBER = "member.paddy.id";
const MEMBER_DEV = "dev.paddy.member.wit.id";
const DASH = "dashboard.paddy.id";
const DASH_DEV = "dev.paddy.wit.id";

describe("proxy: member hostname", () => {
  test("rewrites the root to /member", async () => {
    expect(await call(MEMBER, "/")).toBe("/member");
  });

  // Regression: the first version matched a "member." prefix, so the dev
  // hostname -- which carries the label in the middle -- silently fell
  // through and served the dashboard.
  test("matches the member label anywhere in the host", async () => {
    expect(await call(MEMBER_DEV, "/")).toBe("/member");
    expect(await call(MEMBER_DEV, "/coins")).toBe("/member/coins");
  });

  test("is case-insensitive", async () => {
    expect(await call("DEV.PADDY.MEMBER.WIT.ID", "/")).toBe("/member");
  });

  test("prefixes a nested path", async () => {
    expect(await call(MEMBER, "/coins")).toBe("/member/coins");
  });

  // The whole portal 404s if these get prefixed: ~30 absolute calls to
  // /api/member-portal/* live in the member pages.
  test("passes /api through untouched", async () => {
    expect(await call(MEMBER, "/api/member-portal/me")).toBeNull();
  });

  // Links in the app emit absolute /member/... paths; prefixing twice would
  // send them to /member/member/...
  test("does not double-prefix an already-correct path", async () => {
    expect(await call(MEMBER, "/member/coins")).toBeNull();
  });
});

describe("proxy: other hostnames", () => {
  test("leaves the dashboard host alone", async () => {
    expect(await call(DASH, "/")).toBeNull();
    expect(await call(DASH, "/member/coins")).toBeNull();
    expect(await call(DASH_DEV, "/")).toBeNull();
  });
});

describe("proxy: Go backend prefixes", () => {
  test("rewrites a listed /api prefix to BACKEND_URL with its query", async () => {
    vi.stubEnv("BACKEND_URL", "http://go-api:8080/");
    vi.resetModules();
    vi.doMock("@/lib/backend-routes", async (importOriginal) => {
      const mod = await importOriginal<typeof import("@/lib/backend-routes")>();
      return {
        ...mod,
        goBackendTarget: (pathname: string, method: string) =>
          mod.goBackendTarget(pathname, method, { prefixes: ["/api/auth/me"] }),
      };
    });
    try {
      const { proxy: proxyWithGo } = await import("./proxy");
      const req = new NextRequest(new URL(`https://${DASH}/api/auth/me?x=1`), {
        method: "GET",
        headers: { host: DASH, cookie: "nuhabit_session=abc" },
      });
      const res = await proxyWithGo(req);
      expect(res.headers.get("x-middleware-rewrite")).toBe(
        "http://go-api:8080/api/auth/me?x=1",
      );

      // Same rewrite on the member host, before its /member logic.
      const member = new NextRequest(new URL(`https://${MEMBER}/api/auth/me`), {
        headers: { host: MEMBER },
      });
      expect(
        (await proxyWithGo(member)).headers.get("x-middleware-rewrite"),
      ).toBe("http://go-api:8080/api/auth/me");
    } finally {
      vi.doUnmock("@/lib/backend-routes");
      vi.unstubAllEnvs();
      vi.resetModules();
    }
  });

  test("sends a route Go does not serve to Next even under a switched prefix", async () => {
    vi.stubEnv("BACKEND_URL", "http://go-api:8080");
    vi.resetModules();
    try {
      const { proxy: proxyWithGo } = await import("./proxy");
      const at = (method: string, path: string) =>
        proxyWithGo(
          new NextRequest(new URL(`https://${MEMBER}${path}`), {
            method,
            headers: { host: MEMBER },
          }),
        );
      expect(
        (await at("PUT", "/api/member-portal/profile")).headers.get(
          "x-middleware-rewrite",
        ),
      ).toBe("http://go-api:8080/api/member-portal/profile");
      expect(
        (await at("DELETE", "/api/member-portal/profile")).headers.get(
          "x-middleware-rewrite",
        ),
      ).toBeNull();
    } finally {
      vi.unstubAllEnvs();
      vi.resetModules();
    }
  });

  test("changes nothing while BACKEND_URL is unset", async () => {
    vi.stubEnv("BACKEND_URL", "");
    try {
      expect(await call(MEMBER, "/api/member-portal/me")).toBeNull();
    } finally {
      vi.unstubAllEnvs();
    }
  });
});

describe("proxy: root ke login per host (PADDY_LOGIN_ROOT_HOSTS)", () => {
  async function redirectOf(host: string, path: string, method = "GET") {
    const req = new NextRequest(new URL(`https://${host}${path}`), { headers: { host }, method });
    const res = await proxy(req);
    return { status: res.status, location: res.headers.get("location") };
  }

  test("host terdaftar: / dialihkan 307 ke /login tanpa query", async () => {
    vi.stubEnv("PADDY_LOGIN_ROOT_HOSTS", "paddydev.reddie.id");
    try {
      const r = await redirectOf("paddydev.reddie.id", "/?utm_source=wa");
      expect(r.status).toBe(307);
      expect(new URL(r.location ?? "").pathname).toBe("/login");
      expect(new URL(r.location ?? "").search).toBe("");
    } finally {
      vi.unstubAllEnvs();
    }
  });

  test("host lain dan env kosong: / tetap situs publik", async () => {
    vi.stubEnv("PADDY_LOGIN_ROOT_HOSTS", "paddydev.reddie.id");
    try {
      expect((await redirectOf("dashboard.paddy.id", "/")).location).toBeNull();
    } finally {
      vi.unstubAllEnvs();
    }
    expect((await redirectOf("paddydev.reddie.id", "/")).location).toBeNull();
  });

  test("host member yang terdaftar tetap di-rewrite ke /member", async () => {
    vi.stubEnv("PADDY_LOGIN_ROOT_HOSTS", "member.paddy.id");
    try {
      expect(await call("member.paddy.id", "/")).toBe("/member");
    } finally {
      vi.unstubAllEnvs();
    }
  });
});

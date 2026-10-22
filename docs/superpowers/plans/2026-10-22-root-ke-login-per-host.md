# Root Domain ke Login per Host — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Saat user mengetik `paddydev.reddie.id` (path `/`), browser langsung diarahkan ke `/login`, tanpa mengubah situs publik Paddy di domain lain.

**Architecture:** Halaman `/` (`frontend/src/app/page.tsx`) sengaja menjadi situs publik Paddy untuk semua orang. Karena itu perubahan dibuat **per hostname** lewat env baru `PADDY_LOGIN_ROOT_HOSTS`: proxy Next (`frontend/src/proxy.ts`) mengalihkan `GET/HEAD /` di host yang terdaftar ke `/login` (307). User yang sudah login lalu diteruskan oleh gerbang yang sudah ada (`/login` + sesi → `/dashboard`). Env kosong = perilaku lama.

**Tech Stack:** Next.js 16 (`proxy.ts`, bukan `middleware.ts`), TypeScript, Vitest.

**Spec:** permintaan user 22-10-2026: "kalau ketika user mengetikan paddydev.reddie.id auto direct ke login apa bisa di ubah dari code?" Tidak ada dokumen spec terpisah; keputusan desain dicatat di bagian Keputusan di bawah.

## Global Constraints

- Situs publik di `/` tetap tampil untuk host yang **tidak** terdaftar (komentar di `frontend/src/app/page.tsx`: "Customers must never land on the staff desktop from here").
- Next 16: konvensi file `proxy.ts` dengan export `proxy`. Jangan membuat `middleware.ts`.
- Redirect memakai **307** (sementara) supaya browser tidak meng-cache permanen bila setting host diubah.
- Host member (label DNS `member`) tidak pernah dialihkan ke login; portal member tetap di-rewrite ke `/member`.
- Tanpa perubahan backend Go dan tanpa migrasi DB.
- Nama env: `PADDY_LOGIN_ROOT_HOSTS`, daftar hostname dipisah koma, tanpa skema/port, contoh `paddydev.reddie.id`.

## Keputusan

- **Hanya path `/` persis.** Path lain di host yang sama (mis. `/os`, `/login`, halaman situs publik lain) tidak diubah, jadi tidak ada redirect berulang.
- **Query string `/`** (mis. `/?utm_source=wa`) dibuang; tujuan selalu `/login` polos.
- **User yang sudah login** diarahkan ke `/login` dulu, lalu gerbang sesi yang sudah ada mengirimnya ke `/dashboard`. Logika sesi tidak diduplikasi.
- **Diatur lewat env, bukan hard-code domain**, supaya host baru (mis. VPS paddyadorable) cukup ditambahkan ke env tanpa rilis kode.

## Review Focus

1. Env tidak di-set atau kosong → `/` tetap situs publik di semua host (produksi aman).
2. Header host dengan port atau huruf besar (`PaddyDev.Reddie.ID:443`) tetap cocok dengan entri env.
3. Path selain `/` di host terdaftar (`/login`, `/os`, `/cabang`) tidak dialihkan, sehingga tidak ada loop.
4. Method selain GET/HEAD ke `/` (mis. `POST /`) tidak dialihkan.
5. Host member yang tidak sengaja dimasukkan ke env tetap di-rewrite ke `/member`, bukan dialihkan ke login.

---

### Task 1: Helper `loginRootRedirect`

**Files:**
- Create: `frontend/src/lib/login-root-redirect.ts`
- Test: `frontend/src/lib/login-root-redirect.test.ts`

**Interfaces:**
- Produces: `loginRootRedirect(hostHeader: string, pathname: string, method: string, hosts: string | undefined): boolean`, yaitu `true` bila request harus dialihkan ke `/login`.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from "vitest";
import { loginRootRedirect } from "./login-root-redirect";

const HOSTS = "paddydev.reddie.id, ops.example.com";

describe("loginRootRedirect", () => {
  it("mengalihkan GET/HEAD / di host terdaftar", () => {
    expect(loginRootRedirect("paddydev.reddie.id", "/", "GET", HOSTS)).toBe(true);
    expect(loginRootRedirect("ops.example.com", "/", "HEAD", HOSTS)).toBe(true);
  });
  it("cocok tanpa peduli port dan huruf besar", () => {
    expect(loginRootRedirect("PaddyDev.Reddie.ID:443", "/", "GET", HOSTS)).toBe(true);
  });
  it("env kosong atau tidak di-set: tidak pernah mengalihkan", () => {
    expect(loginRootRedirect("paddydev.reddie.id", "/", "GET", undefined)).toBe(false);
    expect(loginRootRedirect("paddydev.reddie.id", "/", "GET", "")).toBe(false);
    expect(loginRootRedirect("paddydev.reddie.id", "/", "GET", " , ")).toBe(false);
  });
  it("host lain, path lain, dan method lain tidak dialihkan", () => {
    expect(loginRootRedirect("paddy.id", "/", "GET", HOSTS)).toBe(false);
    expect(loginRootRedirect("paddydev.reddie.id.evil.com", "/", "GET", HOSTS)).toBe(false);
    for (const path of ["/login", "/os", "/cabang", "/dashboard"]) {
      expect(loginRootRedirect("paddydev.reddie.id", path, "GET", HOSTS)).toBe(false);
    }
    expect(loginRootRedirect("paddydev.reddie.id", "/", "POST", HOSTS)).toBe(false);
  });
  it("host member tidak pernah dialihkan walau terdaftar", () => {
    expect(loginRootRedirect("member.paddy.id", "/", "GET", "member.paddy.id")).toBe(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/lib/login-root-redirect.test.ts`
Expected: FAIL with "Failed to resolve import ./login-root-redirect"

- [ ] **Step 3: Write minimal implementation**

```ts
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/lib/login-root-redirect.test.ts`
Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/login-root-redirect.ts frontend/src/lib/login-root-redirect.test.ts
git commit -m "feat(auth): helper root domain ke login per host"
```

### Task 2: Pasang di `proxy.ts` + dokumentasi env

**Files:**
- Modify: `frontend/src/proxy.ts` (fungsi `proxy`, setelah blok legacy redirect, sebelum `goBackendTarget`)
- Modify: `frontend/.env.example` (di bawah `PADDY_LEGACY_HOST_REDIRECTS=`)
- Test: `frontend/src/proxy.test.ts`

**Interfaces:**
- Consumes: `loginRootRedirect(hostHeader, pathname, method, hosts)` dari Task 1.

- [ ] **Step 1: Write the failing test** (tambahkan di akhir `frontend/src/proxy.test.ts`; `vi` sudah di-import di file itu)

```ts
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/proxy.test.ts`
Expected: FAIL pada "host terdaftar: / dialihkan 307 ke /login tanpa query" (status 200, location null)

- [ ] **Step 3: Write minimal implementation**

Di `frontend/src/proxy.ts`, tambah import:

```ts
import { loginRootRedirect } from "@/lib/login-root-redirect";
```

dan di dalam `proxy`, tepat setelah blok `if (request.method === "GET" || request.method === "HEAD") { … legacy … }`:

```ts
  // Host staf (mis. paddydev.reddie.id): mengetik domain langsung ke halaman
  // login, bukan situs publik. 307 supaya browser tidak meng-cache permanen.
  if (loginRootRedirect(request.headers.get("host") ?? "", request.nextUrl.pathname, request.method, process.env.PADDY_LOGIN_ROOT_HOSTS)) {
    const url = request.nextUrl.clone();
    url.pathname = "/login";
    url.search = "";
    return NextResponse.redirect(url, 307);
  }
```

Di `frontend/.env.example`, di bawah baris `PADDY_LEGACY_HOST_REDIRECTS=`:

```
# Host staf yang root domain-nya (/) langsung ke /login, bukan situs publik. Dipisah koma, tanpa skema/port.
# Contoh: paddydev.reddie.id. Kosong = semua host menampilkan situs publik di /.
PADDY_LOGIN_ROOT_HOSTS=
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/proxy.test.ts src/lib/login-root-redirect.test.ts && npx tsc --noEmit -p . && npx eslint src/proxy.ts src/lib/login-root-redirect.ts`
Expected: PASS semua; tsc dan eslint tanpa error.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/proxy.ts frontend/src/proxy.test.ts frontend/.env.example
git commit -m "feat(auth): root domain host staf langsung ke login (PADDY_LOGIN_ROOT_HOSTS)"
```

### Langkah rilis (setelah merge, atas persetujuan user)

1. Tambahkan `PADDY_LOGIN_ROOT_HOSTS=paddydev.reddie.id` ke `~/.config/paddydev/web.env` (file env di luar repo yang dibaca `tools/deploy-paddydev.sh`). Tanpa langkah ini perilaku tidak berubah.
2. Backup DB (`pg_dump -Fc` ke `~/Desktop/paddydev-backups`), lalu deploy dengan `tools/deploy-paddydev.sh` dari worktree main terpisah. Tanpa migrasi.
3. Cek pasca-deploy:
   - `curl -sI https://paddydev.reddie.id/` → `307`, `location: …/login`;
   - `/login` dan `/api/ready` → `200`;
   - di browser, user yang sudah login membuka domain dan berakhir di dashboard;
   - halaman situs publik lain (mis. `/os`) tetap terbuka.
4. Untuk host lain (mis. VPS `paddyadorable.biz.id`), cukup tambahkan hostnya ke env yang sama di server itu.

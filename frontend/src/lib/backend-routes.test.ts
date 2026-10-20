import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  GO_BACKEND_PREFIXES,
  NEXT_ONLY_ROUTES,
  goBackendTarget,
  matchesGoPattern,
  type GoRoute,
} from "./backend-routes";
import goRoutes from "./go-routes.generated.json";

const env = { BACKEND_URL: "http://go-api:8080" };
const prefixes = ["/api/gym", "/api/member-portal/workouts/"];
const routes: GoRoute[] = [
  { module: "t", method: "GET", path: "/api/gym" },
  { module: "t", method: "GET", path: "/api/gym/packages/{id}" },
  { module: "t", method: "POST", path: "/api/member-portal/workouts/{a}/{b}" },
  { module: "t", method: "GET", path: "/api/member-portal/me" },
  { module: "t", method: "*", path: "/api/gym/members/{id}/{sub}" },
];
const opts = { prefixes, routes, env };

describe("matchesGoPattern", () => {
  it.each([
    ["/api/gym/packages/{id}", "/api/gym/packages/1", true],
    ["/api/gym/packages/{id}", "/api/gym/packages/", false],
    ["/api/gym/packages/{id}", "/api/gym/packages/1/extra", false],
    ["/api/files/{rest...}", "/api/files/a/b/c", true],
    ["/api/tree/", "/api/tree/x/y", true],
    ["/api/tree/", "/api/tree", false],
    ["/api/exact/{$}", "/api/exact/", true],
    ["/api/exact/{$}", "/api/exact/x", false],
    ["/api/gym", "/api/gymnastics", false],
  ])("%s vs %s -> %s", (pattern, pathname, want) => {
    expect(matchesGoPattern(pattern, pathname)).toBe(want);
  });
});

describe("goBackendTarget", () => {
  it("returns BACKEND_URL + path when the prefix is switched and Go serves the route", () => {
    expect(goBackendTarget("/api/gym", "GET", opts)).toBe(
      "http://go-api:8080/api/gym",
    );
    expect(goBackendTarget("/api/gym/packages/1", "GET", opts)).toBe(
      "http://go-api:8080/api/gym/packages/1",
    );
    expect(goBackendTarget("/api/gym/packages/1", "HEAD", opts)).toBe(
      "http://go-api:8080/api/gym/packages/1",
    );
    expect(
      goBackendTarget("/api/member-portal/workouts/9/start", "POST", opts),
    ).toBe("http://go-api:8080/api/member-portal/workouts/9/start");
  });

  it("forwards every method for a method-less pattern", () => {
    expect(goBackendTarget("/api/gym/members/7/notes", "PATCH", opts)).toBe(
      "http://go-api:8080/api/gym/members/7/notes",
    );
    expect(goBackendTarget("/api/gym/members/7/notes", "DELETE", opts)).toBe(
      "http://go-api:8080/api/gym/members/7/notes",
    );
  });

  it("leaves a method or path Go does not serve to Next", () => {
    expect(goBackendTarget("/api/gym/packages/1", "DELETE", opts)).toBeNull();
    expect(
      goBackendTarget("/api/gym/packages/1/thumbnail", "POST", opts),
    ).toBeNull();
  });

  it("needs a switched prefix even when Go serves the route", () => {
    expect(goBackendTarget("/api/member-portal/me", "GET", opts)).toBeNull();
    expect(goBackendTarget("/api/gymnastics", "GET", opts)).toBeNull();
  });

  it("is off without BACKEND_URL and outside /api", () => {
    expect(goBackendTarget("/api/gym", "GET", { ...opts, env: {} })).toBeNull();
    expect(
      goBackendTarget("/api/gym", "GET", {
        ...opts,
        env: { BACKEND_URL: "  " },
      }),
    ).toBeNull();
    expect(
      goBackendTarget("/gym", "GET", { ...opts, prefixes: ["/gym"] }),
    ).toBeNull();
  });

  it("drops a trailing slash on BACKEND_URL", () => {
    expect(
      goBackendTarget("/api/gym", "GET", {
        ...opts,
        env: { BACKEND_URL: "http://go:8080//" },
      }),
    ).toBe("http://go:8080/api/gym");
  });

  it("uses the generated manifest by default", () => {
    expect(goBackendTarget("/api/spa/bookings/booking-id/therapist", "PUT", { env })).toBe("http://go-api:8080/api/spa/bookings/booking-id/therapist");
    expect(
      goBackendTarget("/api/gym/credits/no/such/route", "GET", { env }),
    ).toBeNull();
    expect(goBackendTarget("/api/auth/me", "GET", { env })).toBe(
      "http://go-api:8080/api/auth/me",
    );
    expect(goBackendTarget("/api/auth/me", "GET", { env: {} })).toBeNull();
  });

  it.each([
    ["GET", "/api/reports/sales-vs-target"],
    ["GET", "/api/reports/sales-targets"],
    ["POST", "/api/reports/sales-targets"],
    ["PATCH", "/api/reports/sales-targets/target-id"],
    ["DELETE", "/api/reports/sales-targets/target-id"],
    ["GET", "/api/reports/sales-target-years"],
    ["PUT", "/api/reports/sales-target-years/2037"],
    ["DELETE", "/api/reports/sales-target-years/2037"],
  ])("forwards the Sales vs Target endpoint %s %s to Go", (method, pathname) => {
    expect(goBackendTarget(pathname, method, { env })).toBe(
      `http://go-api:8080${pathname}`,
    );
  });
});

describe("Go route manifest", () => {
  const manifest = goRoutes as GoRoute[];

  it("covers every switched prefix with at least one Go route", () => {
    for (const prefix of GO_BACKEND_PREFIXES) {
      expect(
        manifest.some(
          (r) => r.path === prefix || r.path.startsWith(`${prefix}/`),
        ),
        prefix,
      ).toBe(true);
    }
  });

  it("keeps a TypeScript fallback route for every switched Go route", () => {
    const tsPath = (goPath: string) =>
      path.join(
        process.cwd(),
        "src/app",
        goPath
          .replace(/\{\w+\.\.\.\}/g, "[...rest]")
          .replace(/\{\w+\}/g, "[id]"),
        "route.ts",
      );
    const dirExists = (file: string) => {
      // Dynamic segment names differ between Go ({coachId}) and Next ([id]),
      // so match any bracketed directory at each dynamic position.
      const parts = path.relative(process.cwd(), file).split(path.sep);
      let dirs = [process.cwd()];
      for (const part of parts) {
        const next: string[] = [];
        for (const dir of dirs) {
          if (part.startsWith("[")) {
            for (const entry of fs.existsSync(dir) ? fs.readdirSync(dir) : []) {
              if (entry.startsWith("[")) next.push(path.join(dir, entry));
            }
          } else if (fs.existsSync(path.join(dir, part))) {
            next.push(path.join(dir, part));
          }
        }
        dirs = next;
      }
      return dirs.length > 0;
    };
    const switched = manifest.filter((r) =>
      GO_BACKEND_PREFIXES.some(
        (p) => r.path === p || r.path.startsWith(`${p}/`),
      ),
    );
    // Dispatchers: one pattern serving several Next routes whose separate
    // patterns ServeMux rejects as overlapping (workouts/{id}/start next to
    // workouts/sessions/{id}, employees/{id}/contracts next to
    // employees/documents/{doc_id}).
    const sharedPatterns = [
      "/api/member-portal/gym/workouts/{a}/{b}",
      "/api/hris/employees/{id}/{sub}",
      "/api/inventory/{id}/{sub}",
      "/api/purchasing/inventory/{id}/{sub}",
      "/api/public/booking/{a}/{b}",
      "/api/public/shop/{a}/{b}",
    ];
    // Routes born in Go: no TypeScript route ever existed under these
    // prefixes, so there is nothing to fall back to.
    const goNativePrefixes = ["/api/shop/wholesale", "/api/wholesale", "/api/public/site", "/api/site", "/api/public/spa", "/api/spa", "/api/reports", "/api/wa-bot", "/api/wa/meta", "/api/settings/wa-meta", "/api/purchasing/settings",
      // Payroll multi-toko (penempatan karyawan, laporan biaya gaji per toko).
      "/api/hris/branches", "/api/hris/employee-placements", "/api/hris/me/placements",
      "/api/hris/payroll/{id}/store-costs", "/api/hris/payroll/entities",
      // Closing periode absensi + penyesuaian manual HRD.
      "/api/hris/attendance-periods", "/api/hris/attendance-settings", "/api/hris/attendance-adjustments",
      "/api/hris/attendance/manual",
      // Komponen manual slip per run (EPIC-055, rule penggajian Odoo).
      "/api/hris/payroll/{id}/inputs",
      // Go-live HRIS (EPIC-056): Jadwal Kerja per cabang, tipe jadwal,
      // export/import karyawan, cek pemilik email.
      "/api/hris/schedules", "/api/hris/employees/schedule-type", "/api/hris/employees/export",
      "/api/hris/employees/import", "/api/hris/employees/import-template", "/api/users/email-owner",
      // Kuota bulanan Izin Pribadi / Izin Leader (EPIC-058).
      "/api/hris/leave-quota",
      // Hak Akses HRIS (EPIC-061): hanya di Go, tanpa fallback Next.
      "/api/hris/access"];
    const goNative = (p: string) =>
      goNativePrefixes.some((n) => p === n || p.startsWith(`${n}/`));
    const missing = switched
      .filter(
        (r) =>
          !sharedPatterns.includes(r.path) &&
          !goNative(r.path) &&
          !dirExists(tsPath(r.path)),
      )
      .map((r) => `${r.method} ${r.path}`);
    expect(missing).toEqual([]);
  });
});

/** Every exported HTTP method of every Next API route, as "METHOD /api/x/[id]". */
function nextApiRoutes(): string[] {
  const root = path.join(process.cwd(), "src/app");
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.name === "route.ts") {
        const src = fs.readFileSync(full, "utf8");
        const route = "/" + path.relative(root, dir).split(path.sep).join("/");
        for (const m of ["GET", "POST", "PUT", "PATCH", "DELETE"]) {
          const exported =
            new RegExp(
              `export\\s+(const|async function|function)\\s+${m}\\b`,
            ).test(src) || new RegExp(`export\\s*\\{[^}]*\\b${m}\\b`).test(src);
          if (exported) out.push(`${m} ${route}`);
        }
      }
    }
  };
  walk(path.join(root, "api"));
  return out;
}

/** A concrete path for a Next route: "[id]" -> "x1", "[...path]" -> "a/b". */
const samplePath = (route: string) =>
  route.replace(/\[\.\.\.\w+\]/g, "a/b").replace(/\[\w+\]/g, "x1");

describe("routes kept in Next", () => {
  const allNext = nextApiRoutes();

  it("keeps a Go wildcard from taking a Next-only route", () => {
    const nextOnly = ["GET /api/hris/attendance/export"];
    expect(
      goBackendTarget("/api/hris/attendance/export", "GET", { env, nextOnly }),
    ).toBeNull();
    expect(
      goBackendTarget("/api/hris/attendance/export", "HEAD", { env, nextOnly }),
    ).toBeNull();
    expect(
      goBackendTarget("/api/hris/attendance/7", "GET", { env, nextOnly }),
    ).toBe("http://go-api:8080/api/hris/attendance/7");
    expect(goBackendTarget("/api/hris/attendance/export", "GET", { env })).toBe(
      "http://go-api:8080/api/hris/attendance/export",
    );
  });

  it("lists only routes that exist in Next", () => {
    const existing = new Set(allNext);
    expect(NEXT_ONLY_ROUTES.filter((r) => !existing.has(r))).toEqual([]);
  });

  it("decides every Next route under a switched prefix: Go or NEXT_ONLY_ROUTES", () => {
    const undecided = allNext.filter((entry) => {
      const [method, route] = entry.split(" ");
      const switched = GO_BACKEND_PREFIXES.some(
        (p) => route === p || route.startsWith(`${p}/`),
      );
      return (
        switched &&
        !NEXT_ONLY_ROUTES.includes(entry) &&
        goBackendTarget(samplePath(route), method, { env }) === null
      );
    });
    expect(undecided).toEqual([]);
  });
});

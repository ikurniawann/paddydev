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

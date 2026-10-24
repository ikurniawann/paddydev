#!/usr/bin/env node
/**
 * Capture screenshot Manual Book ESS Karyawan (Area Karyawan Paddy HRIS).
 *
 * Satu fungsi per bagian manual (lihat SECTIONS di bawah). Untuk menambah
 * bagian/jenis cuti baru: tulis fungsi baru (atau tambah satu shot di fungsi
 * yang ada), daftarkan di SECTIONS, lalu rujuk nama file PNG-nya di manual.html.
 *
 * Prasyarat (lihat README.md): DB TEST sudah di-seed (seed-demo.sql), Go API +
 * Next dev jalan dengan DB TEST, password demo ada di file DEMO_PASSWORD_FILE.
 *
 *   DEMO_PASSWORD_FILE=/path/pass.txt node capture.cjs            # semua bagian
 *   DEMO_PASSWORD_FILE=/path/pass.txt node capture.cjs cuti lembur # sebagian
 *
 * Variabel opsional: BASE_URL (default http://localhost:3497 — pakai
 * "localhost", bukan 127.0.0.1), TEST_DB, FRONTEND_DIR (folder frontend yang
 * punya node_modules playwright + sharp).
 *
 * Catatan urutan: bagian "absen-masuk" → "absen-pulang" melakukan absen
 * sungguhan untuk Rina hari ini, "luar-area" untuk Andi, dan "pinjaman" membuat
 * Andi menyetujui kasbon. Jalankan ulang cleanup-demo.sql + seed-demo.sql
 * sebelum capture ulang semua bagian.
 */

const path = require("path");
const fs = require("fs");
const { execFileSync } = require("child_process");

const FRONTEND_DIR = process.env.FRONTEND_DIR || "/home/wit/Desktop/paddydev/frontend";
const { chromium } = require(path.join(FRONTEND_DIR, "node_modules/playwright"));
const sharp = require(path.join(FRONTEND_DIR, "node_modules/sharp"));

const BASE_URL = process.env.BASE_URL || "http://localhost:3497";
const TEST_DB = process.env.TEST_DB || "postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable";
const OUT = path.join(__dirname, "screens");
const SLOW = 180_000; // kompilasi pertama Next dev bisa lambat

if (/:55432\b/.test(TEST_DB) || !/paddy_test/.test(TEST_DB)) {
  throw new Error(`TEST_DB harus DB test (paddy_test), bukan ${TEST_DB}`);
}

const PEOPLE = {
  rina: { email: "rina.lestari.demo@example.com", employeeId: "de5a0000-0000-4000-8000-000000000011" },
  andi: { email: "andi.pratama.demo@example.com", employeeId: "de5a0000-0000-4000-8000-000000000012" },
};

/** Koordinat Paddy Store PVJ (hanya diisi sementara untuk popup "di luar area"). */
const PVJ = { lat: -6.889221, lng: 107.595821, radius: 250 };
/** Posisi demo ±400 m di utara toko. */
const FAR_FROM_PVJ = { latitude: -6.8856, longitude: 107.595821 };
/** Posisi demo di Bandung (dipakai bila toko tanpa koordinat — hasilnya "Belum dicek"). */
const BANDUNG = { latitude: -6.9147, longitude: 107.6098 };

const HIDE_DEV_CSS = `
  nextjs-portal, .tsqd-parent-container, .tsqd-open-btn-container { display: none !important; }
  * { caret-color: transparent !important; }
`;

/* ───────────────────────────── helper umum ───────────────────────────── */

function psql(sql) {
  return execFileSync("psql", [TEST_DB, "-Atc", sql], { encoding: "utf8" }).trim();
}

function isoDate(offsetDays) {
  const now = new Date(Date.now() + 7 * 3600_000); // WIB
  now.setUTCDate(now.getUTCDate() + offsetDays);
  return now.toISOString().slice(0, 10);
}

/** Tanggal ke depan berikutnya (≥ offset hari) yang jatuh pada hari ISO tertentu (1=Senin … 7=Minggu). */
function nextWeekday(isoDow, minOffset) {
  for (let i = minOffset; i < minOffset + 8; i++) {
    const d = new Date(`${isoDate(i)}T00:00:00Z`);
    const dow = d.getUTCDay() === 0 ? 7 : d.getUTCDay();
    if (dow === isoDow) return isoDate(i);
  }
  throw new Error("nextWeekday");
}

function password() {
  const file = process.env.DEMO_PASSWORD_FILE;
  if (process.env.DEMO_PASSWORD) return process.env.DEMO_PASSWORD;
  if (!file) throw new Error("Set DEMO_PASSWORD_FILE (file berisi password akun demo)");
  return fs.readFileSync(file, "utf8").trim();
}

async function newContext(browser, { who, geo = BANDUNG } = {}) {
  const ctx = await browser.newContext({
    baseURL: BASE_URL,
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
    locale: "id-ID",
    timezoneId: "Asia/Jakarta",
    permissions: ["camera", "geolocation"],
    geolocation: geo,
  });
  await ctx.addInitScript((css) => {
    const add = () => {
      const s = document.createElement("style");
      s.textContent = css;
      document.documentElement.appendChild(s);
    };
    if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", add);
    else add();
  }, HIDE_DEV_CSS);
  if (who) {
    const res = await ctx.request.post("/api/auth/login", {
      data: { email: PEOPLE[who].email, password: password() },
      timeout: SLOW,
    });
    if (!res.ok()) throw new Error(`login ${who} gagal: ${res.status()} ${await res.text()}`);
  }
  return ctx;
}

async function open(ctx, url, waitText, height = 844) {
  const page = await ctx.newPage();
  await page.setViewportSize({ width: 390, height });
  await page.goto(url, { timeout: SLOW, waitUntil: "domcontentloaded" });
  if (waitText) await page.getByText(waitText, { exact: false }).first().waitFor({ timeout: SLOW });
  await page.waitForLoadState("networkidle", { timeout: SLOW }).catch(() => {});
  await page.waitForTimeout(600);
  return page;
}

/**
 * Gambar kotak oranye + lencana bernomor di atas elemen (callout manual).
 * items: [{ target: Locator, n: 1, pos: "tl"|"tr"|"l"|"r"|"bl"|"br", pad: 4 }]
 */
async function mark(page, items) {
  const boxes = [];
  for (const it of items) {
    const loc = it.target.first();
    await loc.waitFor({ state: "visible", timeout: 30_000 });
    const b = await loc.boundingBox();
    if (!b) throw new Error(`mark ${it.n}: elemen tidak terlihat`);
    boxes.push({ x: b.x, y: b.y, w: b.width, h: b.height, n: it.n, pos: it.pos || "tl", pad: it.pad ?? 4 });
  }
  await page.evaluate((boxes) => {
    let root = document.getElementById("__manual_marks");
    if (!root) {
      root = document.createElement("div");
      root.id = "__manual_marks";
      root.style.cssText = "position:fixed;inset:0;pointer-events:none;z-index:2147483647";
      document.body.appendChild(root);
    }
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const S = 26;
    for (const b of boxes) {
      const x = b.x - b.pad, y = b.y - b.pad, w = b.w + 2 * b.pad, h = b.h + 2 * b.pad;
      const o = document.createElement("div");
      o.className = "__mark_box";
      o.style.cssText = `position:fixed;left:${x}px;top:${y}px;width:${w}px;height:${h}px;` +
        "border:3px solid #ea580c;border-radius:10px;box-shadow:0 0 0 2px rgba(255,255,255,.85)";
      const pos = {
        // Sudut: lencana digeser ke luar kotak supaya tidak menutup label di atas isian.
        tl: [x - S + 8, y - S + 8], tr: [x + w - 8, y - S + 8],
        bl: [x - S + 8, y + h - 8], br: [x + w - 8, y + h - 8],
        l: [x - S - 4, y + h / 2 - S / 2], r: [x + w + 4, y + h / 2 - S / 2],
      }[b.pos];
      const bx = Math.max(2, Math.min(vw - S - 2, pos[0]));
      const by = Math.max(2, Math.min(vh - S - 2, pos[1]));
      const badge = document.createElement("div");
      badge.className = "__mark_badge";
      badge.textContent = String(b.n);
      badge.style.cssText = `position:fixed;left:${bx}px;top:${by}px;width:${S}px;height:${S}px;border-radius:50%;` +
        "background:#ea580c;color:#fff;font:700 15px/26px Arial,Helvetica,sans-serif;text-align:center;" +
        "border:2px solid #fff;box-sizing:content-box;box-shadow:0 1px 4px rgba(0,0,0,.35)";
      root.append(o, badge);
    }
  }, boxes);
}

async function unmark(page) {
  await page.evaluate(() => document.getElementById("__manual_marks")?.remove());
}

/**
 * Simpan potongan layar: gabungan kotak `region` (Locator / array Locator) +
 * semua callout, ditambah margin. PNG dikompres palet (sharp/libimagequant).
 */
async function snap(page, name, region, { pad = 10, maxHeight = 1700 } = {}) {
  const regions = Array.isArray(region) ? region : [region];
  const rects = [];
  for (const r of regions) {
    const b = await r.first().boundingBox();
    if (b) rects.push(b);
  }
  const markRects = await page.evaluate(() =>
    [...document.querySelectorAll("#__manual_marks > div")].map((e) => {
      const r = e.getBoundingClientRect();
      return { x: r.x, y: r.y, width: r.width, height: r.height };
    })
  );
  rects.push(...markRects);
  if (!rects.length) throw new Error(`snap ${name}: region kosong`);
  const vp = page.viewportSize();
  let x0 = Math.min(...rects.map((r) => r.x)) - pad;
  let y0 = Math.min(...rects.map((r) => r.y)) - pad;
  let x1 = Math.max(...rects.map((r) => r.x + r.width)) + pad;
  let y1 = Math.max(...rects.map((r) => r.y + r.height)) + pad;
  x0 = Math.max(0, x0); y0 = Math.max(0, y0);
  x1 = Math.min(vp.width, x1); y1 = Math.min(vp.height, y1, y0 + maxHeight);
  const buf = await page.screenshot({ clip: { x: x0, y: y0, width: x1 - x0, height: y1 - y0 } });
  const file = path.join(OUT, `${name}.png`);
  await sharp(buf).png({ palette: true, quality: 90, effort: 10, compressionLevel: 9 }).toFile(file);
  console.log(`  ✓ ${name}.png  ${Math.round(fs.statSync(file).size / 1024)} KB`);
}

async function shot(page, name, region, items = [], opts) {
  await mark(page, items);
  await snap(page, name, region, opts);
  await unmark(page);
}

/** Dialog berjudul `title` (string/RegExp). Popover juga ber-role dialog, jadi selalu disaring judul. */
const dialog = (page, title) => page.getByRole("dialog").filter({ has: page.getByRole("heading", { name: title }) });
const card = (page, heading) =>
  page.locator("div.rounded-xl, div.rounded-2xl").filter({ has: page.getByText(heading, { exact: true }) }).last();

/** Kamera palsu Chromium → Ambil Foto → Gunakan Foto Ini (selfie absen). */
async function takeSelfie(page, { shotBefore, shotAfter } = {}) {
  const dlg = dialog(page, /Selfie/);
  await dlg.getByRole("button", { name: "Ambil Foto" }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(1500); // video kamera palsu mulai berjalan
  if (shotBefore) {
    await shot(page, shotBefore, dlg, [
      { target: dlg.locator("video"), n: 1, pos: "tl" },
      { target: dlg.getByRole("button", { name: "Ambil Foto" }), n: 2, pos: "l" },
    ]);
  }
  await dlg.getByRole("button", { name: "Ambil Foto" }).click();
  await dlg.getByRole("button", { name: "Gunakan Foto Ini" }).waitFor({ timeout: 10_000 });
  if (shotAfter) {
    await shot(page, shotAfter, dlg, [
      { target: dlg.getByRole("button", { name: "Gunakan Foto Ini" }), n: 1, pos: "tr" },
      { target: dlg.getByRole("button", { name: "Ulangi" }), n: 2, pos: "br" },
    ]);
  }
  await dlg.getByRole("button", { name: "Gunakan Foto Ini" }).click();
}

/** Pilih opsi pada Combobox (tombol role=combobox → daftar opsi). */
async function pickCombo(page, combo, label) {
  await combo.click();
  await page.getByRole("option", { name: label, exact: true }).click();
  await page.waitForTimeout(300);
}

/* ───────────────────────────── bagian manual ───────────────────────────── */

/** 1. Masuk ke aplikasi: halaman login, Beranda, menu Lainnya. */
async function masuk(browser) {
  const anon = await newContext(browser);
  const page = await open(anon, "/login", "Kata sandi");
  await page.getByPlaceholder("nama@perusahaan.com").fill(PEOPLE.rina.email);
  await page.locator('input[type="password"]').fill("contohsandi");
  const form = page.locator("form").first();
  await shot(page, "01-login", [page.getByText("Masuk.", { exact: true }), form], [
    { target: page.getByPlaceholder("nama@perusahaan.com"), n: 1, pos: "tl" },
    { target: page.locator('input[type="password"]'), n: 2, pos: "tl" },
    { target: page.getByRole("button", { name: "Masuk", exact: true }), n: 3, pos: "tl" },
  ]);
  await anon.close();

  const ctx = await newContext(browser, { who: "rina" });
  const home = await open(ctx, "/dashboard/me", "Absensi Hari Ini", 2100);
  const absensi = card(home, "Absensi Hari Ini");
  const stats = home.locator("div.grid").filter({ has: home.getByText("Sisa Cuti Tahunan") }).first();
  await shot(home, "00-sampul", [absensi, stats], []); // gambar bersih untuk sampul
  await shot(home, "01-beranda", [home.locator("div.bg-gradient-to-br").first(), absensi, stats], [
    { target: absensi, n: 1, pos: "tl" },
    { target: stats.locator("a").nth(0), n: 2, pos: "tl" },
    { target: stats.locator("a").nth(1), n: 3, pos: "tr" },
    { target: stats.locator("a").nth(2), n: 4, pos: "bl" },
    { target: stats.locator("a").nth(3), n: 5, pos: "br" },
  ]);
  const status = card(home, "Status Pengajuan Saya");
  const quick = home.locator("div").filter({ has: home.getByText("Aksi Cepat", { exact: true }) }).last();
  await shot(home, "01-beranda-aksi", [status, quick], [
    { target: status, n: 1, pos: "tl" },
    { target: quick.locator("div.grid"), n: 2, pos: "tl" },
  ]);

  // Menu lengkap: tombol "Lainnya" di bilah bawah.
  await home.setViewportSize({ width: 390, height: 844 });
  await home.waitForTimeout(400);
  const nav = home.locator("nav.fixed").first();
  await shot(home, "01-bilah-bawah", nav, [
    { target: nav.locator(":scope > *").filter({ hasText: "Area Karyawan" }), n: 1, pos: "tl", pad: 2 },
    { target: nav.locator(":scope > *").filter({ hasText: "Lainnya" }), n: 2, pos: "tr", pad: 2 },
  ], { pad: 16 });
  await nav.getByText("Lainnya").click();
  await home.getByText("Perjalanan Dinas", { exact: true }).waitFor();
  await home.waitForTimeout(700);
  const sheet = home.getByRole("dialog").filter({ has: home.getByPlaceholder("Cari halaman") });
  const ess = sheet.locator("div").filter({ has: home.getByText("Perjalanan Dinas", { exact: true }) })
    .filter({ has: home.getByText("Slip Gaji", { exact: true }) }).last();
  await shot(home, "01-menu-lainnya", [sheet.getByPlaceholder("Cari halaman"), ess], [
    { target: sheet.getByPlaceholder("Cari halaman").locator(".."), n: 1, pos: "tl", pad: 2 },
    { target: ess, n: 2, pos: "tl" },
  ]);
  await ctx.close();
}

/** 2. Absen Masuk (Clock In) — Rina, toko tanpa koordinat → "Belum dicek". */
async function absenMasuk(browser) {
  const ctx = await newContext(browser, { who: "rina", geo: BANDUNG });
  const page = await open(ctx, "/dashboard/me/absensi", "Shift hari ini", 1200);
  const head = page.locator("div.bg-gradient-to-r").first();
  const select = head.getByRole("combobox").first();
  await shot(page, "02-absensi", head, [
    { target: head.getByText("Shift hari ini", { exact: false }), n: 1, pos: "tl" },
    { target: select, n: 2, pos: "tl" },
    { target: head.getByRole("button", { name: "Clock In" }), n: 3, pos: "tl" },
    { target: head.getByText("Foto selfie dan GPS wajib", { exact: false }), n: 4, pos: "bl" },
  ]);

  await select.click();
  const list = page.getByRole("listbox");
  await list.waitFor();
  await page.waitForTimeout(400);
  await shot(page, "02-pilih-toko", [select, list], [{ target: list, n: 1, pos: "tl" }]);
  await page.getByRole("option", { name: /Paddy Store PVJ/ }).click();
  await page.waitForTimeout(300);

  await head.getByRole("button", { name: "Clock In" }).click();
  await takeSelfie(page, { shotBefore: "02-selfie", shotAfter: "02-selfie-konfirmasi" });
  await head.getByText("Masuk", { exact: false }).locator("b").first().waitFor({ timeout: 30_000 }).catch(() => {});
  await page.getByText("Tepat waktu").or(page.getByText(/Terlambat \d+ mnt/)).first().waitFor({ timeout: 30_000 });
  await page.waitForTimeout(800);
  const toast = page.locator("[data-sonner-toast]").first();
  const status = head.locator("div.flex-wrap").filter({ hasText: "Masuk" }).last();
  await shot(page, "02-berhasil", [head], [
    { target: status.locator("span").first(), n: 1, pos: "l" },
    { target: status.getByText(/Tepat waktu|Terlambat \d+ mnt/).first(), n: 2, pos: "tr" },
    { target: status.getByText("Masuk: ", { exact: false }), n: 3, pos: "r" },
  ]);
  if (await toast.isVisible().catch(() => false)) {
    await shot(page, "02-notifikasi", toast, [], { pad: 6 });
  }
  await ctx.close();
}

/** 3. Di luar area toko — Andi, koordinat PVJ diisi sementara, posisi ±400 m. */
async function luarArea(browser) {
  psql(`UPDATE configuration.branches SET lat = ${PVJ.lat}, lng = ${PVJ.lng}, geofence_radius_m = ${PVJ.radius} WHERE code = 'PD-PVJ'`);
  try {
    const ctx = await newContext(browser, { who: "andi", geo: FAR_FROM_PVJ });
    const page = await open(ctx, "/dashboard/me/absensi", "Shift hari ini", 1000);
    const head = page.locator("div.bg-gradient-to-r").first();
    await head.getByRole("button", { name: "Clock In" }).click();
    await takeSelfie(page);
    const dlg = page.getByRole("dialog").filter({ hasText: "Anda di luar area toko" });
    await dlg.waitFor({ timeout: 30_000 });
    await page.waitForTimeout(500);
    await dlg.locator("textarea").fill("Antar pesanan pelanggan ke Jl. Sukajadi");
    await shot(page, "03-popup-luar-area", dlg, [
      { target: dlg.getByText("Posisi Anda", { exact: false }), n: 1, pos: "l" },
      { target: dlg.locator("textarea"), n: 2, pos: "l" },
      { target: dlg.getByText("Minimal", { exact: false }), n: 3, pos: "l" },
      { target: dlg.getByRole("button", { name: "Tetap absen" }), n: 4, pos: "tr" },
    ]);
    await dlg.getByRole("button", { name: "Tetap absen" }).click();
    await page.getByText("Di luar area", { exact: false }).first().waitFor({ timeout: 30_000 });
    await page.waitForTimeout(800);
    await shot(page, "03-hasil-luar-area", head, [
      { target: head.getByText("Masuk: ", { exact: false }), n: 1, pos: "tl" },
    ]);
    await ctx.close();
  } finally {
    psql("UPDATE configuration.branches SET lat = NULL, lng = NULL, geofence_radius_m = NULL WHERE code = 'PD-PVJ'");
  }
}

/** 4. Absen Pulang (Clock Out) — Rina, setelah absenMasuk. */
async function absenPulang(browser) {
  const ctx = await newContext(browser, { who: "rina", geo: BANDUNG });
  const page = await open(ctx, "/dashboard/me/absensi", "Shift hari ini", 1200);
  const head = page.locator("div.bg-gradient-to-r").first();
  const out = head.getByRole("button", { name: "Clock Out" });
  await out.waitFor();
  await page.waitForFunction(() =>
    [...document.querySelectorAll("button")].some((b) => b.textContent.includes("Clock Out") && !b.disabled), null, { timeout: 30_000 });
  await shot(page, "04-clock-out", head, [{ target: out, n: 1, pos: "tl" }]);
  await out.click();
  await takeSelfie(page);
  await page.getByText("Absensi hari ini selesai").waitFor({ timeout: 30_000 });
  await page.waitForTimeout(800);
  await shot(page, "04-selesai", head, [
    { target: head.getByText("Pulang", { exact: false }).first(), n: 1, pos: "tl" },
    { target: head.getByText("Absensi hari ini selesai"), n: 2, pos: "br" },
  ]);
  await ctx.close();
}

/** 5. Riwayat absensi: kalender bulan ini + detail satu hari. */
async function riwayat(browser) {
  const lateDate = psql(`SELECT date FROM hris.attendance WHERE employee_id = '${PEOPLE.rina.employeeId}' AND is_late ORDER BY date DESC LIMIT 1`);
  const ctx = await newContext(browser, { who: "rina" });
  const page = await open(ctx, "/dashboard/me/absensi", "Shift hari ini", 1700);
  // Kartu kalender = leluhur terdekat tombol "Bulan sebelumnya" yang memuat grid 7 kolom.
  const cal = page.getByRole("button", { name: "Bulan sebelumnya" })
    .locator("xpath=ancestor::div[.//div[contains(@class,'grid-cols-7')]][1]");
  await cal.scrollIntoViewIfNeeded();
  await page.getByText("Hadir", { exact: true }).first().waitFor();
  await page.waitForTimeout(600);
  // Bulan berjalan; bila hari terlambat ada di bulan lalu, mundur satu bulan.
  const thisMonth = isoDate(0).slice(0, 7);
  if (lateDate && lateDate.slice(0, 7) !== thisMonth) {
    await page.getByRole("button", { name: "Bulan sebelumnya" }).click();
    await page.waitForTimeout(1200);
  }
  const day = String(Number(lateDate.slice(8, 10)));
  const cell = cal.locator("div.cursor-pointer").filter({
    has: page.locator("span.rounded-full", { hasText: new RegExp(`^${day}$`) }),
  }).first();
  await shot(page, "05-kalender", cal, [
    { target: page.getByRole("button", { name: "Bulan sebelumnya" }), n: 1, pos: "l" },
    { target: cell, n: 2, pos: "tr" },
    { target: cal.locator("div.flex-wrap").filter({ hasText: "Hadir" }).filter({ hasText: "Jadwal shift" }).last(), n: 3, pos: "tl" },
  ], { maxHeight: 1600 });
  await cell.click();
  const dlg = page.getByRole("dialog").filter({ hasText: "Waktu dalam WIB" });
  await dlg.waitFor();
  await page.waitForTimeout(600);
  await shot(page, "05-detail-hari", dlg, []);
  await ctx.close();
}

/** 6. Izin & Cuti: halaman, daftar jenis, form + pratinjau, Izin Pribadi, Izin Leader (Andi). */
async function cuti(browser) {
  const ctx = await newContext(browser, { who: "rina" });
  const page = await open(ctx, "/dashboard/me/cuti", "Riwayat Pengajuan", 1300);
  // Sementara: di main 5035ae5 kartu Riwayat melebar keluar layar HP bila ada
  // "Alasan ditolak" (item grid tanpa min-w-0). Untuk foto manual, tampilkan
  // sesuai desain. Hapus baris ini setelah aplikasi diperbaiki.
  await page.addStyleTag({ content: "main .grid > div { min-width: 0; }" });
  const quota = page.locator("div.rounded-xl").filter({ hasText: "Kuota Cuti Tahunan" }).first();
  const history = page.locator("div.rounded-xl").filter({ hasText: "Riwayat Pengajuan" }).first();
  const rejected = history.locator("li").filter({ hasText: "Ditolak" }).first();
  await shot(page, "06-cuti", [quota, history], [
    { target: quota.locator("p").filter({ hasText: "hari tersisa" }), n: 1, pos: "tl" },
    { target: page.getByRole("button", { name: /Ajukan Izin/ }), n: 2, pos: "tl" },
    { target: rejected, n: 3, pos: "tl" },
    { target: history.locator("li").filter({ hasText: "Menunggu" }).first().locator("span").last(), n: 4, pos: "l" },
  ]);

  await page.getByRole("button", { name: /Ajukan Izin/ }).click();
  const dlg = dialog(page, "Ajukan Izin / Cuti");
  await dlg.getByText("Ajukan Izin / Cuti").waitFor();
  await page.waitForTimeout(500);
  const combo = dlg.getByRole("combobox").first();
  await combo.click();
  const options = page.getByRole("listbox").last();
  await options.waitFor();
  await page.waitForTimeout(400);
  await shot(page, "06-jenis", [combo, options], [{ target: options, n: 1, pos: "tl" }]);
  await page.getByRole("option", { name: "Cuti Tahunan", exact: true }).click();

  // Jumat s.d. Senin: 4 hari kalender, akhir pekan tidak memotong jatah.
  const fri = nextWeekday(5, 10);
  const mon = isoDate(Math.round((Date.parse(fri) - Date.parse(isoDate(0))) / 86_400_000) + 3);
  const dates = dlg.locator('input[type="date"]');
  await dates.nth(0).fill(fri);
  await dates.nth(1).fill(mon);
  await dlg.getByPlaceholder("cth. keperluan keluarga di luar kota").fill("Liburan keluarga ke Yogyakarta");
  await dlg.getByText("Memotong jatah", { exact: false }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(400);
  const preview = dlg.locator("div.rounded-lg").filter({ hasText: "Memotong jatah" }).first();
  await shot(page, "06-form-cuti", dlg, [
    { target: combo, n: 1, pos: "tl" },
    { target: dates.nth(0), n: 2, pos: "tl" },
    { target: dates.nth(1), n: 3, pos: "tr" },
    { target: dlg.getByPlaceholder("cth. keperluan keluarga di luar kota"), n: 4, pos: "tl" },
    { target: dlg.locator('input[type="file"]'), n: 5, pos: "tl" },
    { target: preview, n: 6, pos: "tl" },
    { target: dlg.getByRole("button", { name: "Kirim Pengajuan" }), n: 7, pos: "tr" },
  ]);

  // Izin Pribadi: satu hari, kuota bulanan, memotong 1 hari THP.
  await pickCombo(page, combo, "Izin Pribadi");
  await dates.nth(0).fill(isoDate(6));
  await dlg.getByText("Ajukan Izin / Cuti").click(); // lepas fokus dari input tanggal
  await dlg.getByText("Sisa kuota Izin Pribadi", { exact: false }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(400);
  await shot(page, "06-form-izin-pribadi", dlg, [
    { target: combo, n: 1, pos: "tl" },
    { target: dates.nth(1), n: 2, pos: "tr" },
    { target: dlg.getByText("Sisa kuota Izin Pribadi", { exact: false }), n: 3, pos: "tl" },
  ]);
  await dlg.getByRole("button", { name: "Batal" }).click();
  await ctx.close();

  // Izin Leader hanya muncul untuk Supervisor ke atas (Andi = Store Manager).
  const actx = await newContext(browser, { who: "andi" });
  const ap = await open(actx, "/dashboard/me/cuti", "Riwayat Pengajuan", 1300);
  await ap.getByRole("button", { name: /Ajukan Izin/ }).click();
  const adlg = dialog(ap, "Ajukan Izin / Cuti");
  await adlg.getByText("Ajukan Izin / Cuti").waitFor();
  await ap.waitForTimeout(800);
  const acombo = adlg.getByRole("combobox").first();
  await pickCombo(ap, acombo, "Izin Leader");
  await adlg.locator('input[type="date"]').nth(0).fill(isoDate(6));
  await adlg.getByText("Sisa kuota Izin Leader", { exact: false }).waitFor({ timeout: 30_000 });
  await acombo.click();
  const aoptions = ap.getByRole("listbox").last();
  await aoptions.waitFor();
  await ap.waitForTimeout(400);
  await shot(ap, "06-izin-leader", [acombo, aoptions, adlg.getByText("Sisa kuota Izin Leader", { exact: false })], [
    { target: ap.getByRole("option", { name: "Izin Leader", exact: true }), n: 1, pos: "r" },
  ]);
  await actx.close();
}

/** 7. Pinjaman & Kasbon: riwayat, form, antrean Manager (Andi), status "Disetujui Manager". */
async function pinjaman(browser) {
  const ctx = await newContext(browser, { who: "rina" });
  let page = await open(ctx, "/dashboard/me/pinjaman", "Riwayat Pinjaman", 1100);
  const history = page.locator("div.rounded-xl").filter({ hasText: "Riwayat Pinjaman" }).first();
  const header = page.locator("div.border-b").filter({ hasText: "Ajukan Pinjaman" }).first();
  await shot(page, "07-pinjaman", [header, history], [
    { target: page.getByRole("button", { name: "Ajukan Pinjaman" }), n: 1, pos: "tl" },
    { target: history.locator("li").filter({ hasText: "Menunggu" }).first(), n: 2, pos: "tl" },
    { target: history.locator("li").filter({ hasText: "Berjalan" }).first(), n: 3, pos: "bl" },
  ]);

  await page.getByRole("button", { name: "Ajukan Pinjaman" }).click();
  const dlg = dialog(page, "Ajukan Pinjaman / Kasbon");
  await dlg.getByText("Ajukan Pinjaman / Kasbon").waitFor();
  await page.waitForTimeout(500);
  await dlg.locator('input[type="number"]').first().fill("750000");
  await dlg.getByPlaceholder("cth. biaya sekolah anak").fill("Biaya berobat orang tua");
  await dlg.getByText("Perkiraan cicilan", { exact: false }).waitFor();
  await shot(page, "07-form-kasbon", dlg, [
    { target: dlg.getByRole("combobox").nth(0), n: 1, pos: "tl" },
    { target: dlg.locator('input[type="number"]').first(), n: 2, pos: "tl" },
    { target: dlg.getByRole("combobox").nth(1), n: 3, pos: "tr" },
    { target: dlg.getByText("Perkiraan cicilan", { exact: false }), n: 4, pos: "tl" },
    { target: dlg.getByRole("button", { name: "Kirim Pengajuan" }), n: 5, pos: "tr" },
  ]);
  await dlg.getByRole("button", { name: "Batal" }).click();

  // Atasan (Manager ke atas) menyetujui tahap 1.
  const actx = await newContext(browser, { who: "andi" });
  const ap = await open(actx, "/dashboard/me/pinjaman", "Persetujuan Kasbon Tim", 1100);
  const team = ap.locator("div.rounded-xl").filter({ hasText: "Persetujuan Kasbon Tim" }).first();
  const row = team.locator("li").first();
  await shot(ap, "07-persetujuan-tim", team, [
    { target: row.locator("div.min-w-0"), n: 1, pos: "tl" },
    { target: row.getByRole("button", { name: "Setujui" }), n: 2, pos: "tl" },
    { target: row.getByRole("button", { name: "Tolak" }), n: 3, pos: "tr" },
  ]);
  await row.getByRole("button", { name: "Setujui" }).click();
  await ap.getByText("menunggu persetujuan HR", { exact: false }).first().waitFor({ timeout: 30_000 });
  await actx.close();

  await page.close();
  page = await open(ctx, "/dashboard/me/pinjaman", "Disetujui Manager", 1100);
  const h2 = page.locator("div.rounded-xl").filter({ hasText: "Riwayat Pinjaman" }).first();
  await shot(page, "07-disetujui-manager", h2, [
    { target: h2.getByText("Disetujui Manager", { exact: true }), n: 1, pos: "tl", pad: 3 },
  ]);
  await ctx.close();
}

/** 8. Perjalanan Dinas: riwayat, form, antrean atasan (Andi). */
async function perjalananDinas(browser) {
  const ctx = await newContext(browser, { who: "rina" });
  const page = await open(ctx, "/dashboard/me/perjalanan-dinas", "Selama dinas disetujui", 1000);
  await page.getByText("Gudang Pusat Cikarang", { exact: false }).waitFor({ timeout: SLOW });
  const header = page.locator("div.border-b").filter({ hasText: "Selama dinas disetujui" }).first();
  const list = page.locator("div.rounded-xl").filter({ has: page.locator("li") }).last();
  await shot(page, "08-dinas", [header, list], [
    { target: page.getByRole("button", { name: "Ajukan", exact: true }), n: 1, pos: "tl" },
    { target: list.locator("li").filter({ hasText: "Menunggu" }).first(), n: 2, pos: "tl" },
    { target: list.locator("li").filter({ hasText: "Disetujui" }).first(), n: 3, pos: "bl" },
  ]);

  await page.getByRole("button", { name: "Ajukan", exact: true }).click();
  const dlg = dialog(page, "Ajukan Perjalanan Dinas");
  await dlg.getByText("Ajukan Perjalanan Dinas").waitFor();
  await page.waitForTimeout(500);
  const dates = dlg.locator('input[type="date"]');
  await dates.nth(0).fill(isoDate(14));
  await dates.nth(1).fill(isoDate(15));
  await dlg.getByPlaceholder("Mis. Gudang Cikarang").fill("Paddy Store Kopo");
  await dlg.locator("textarea").fill("Membantu stock opname akhir bulan");
  await dlg.getByText("tidak dicek radius toko", { exact: false }).waitFor();
  await shot(page, "08-form-dinas", dlg, [
    { target: dates.nth(0), n: 1, pos: "tl" },
    { target: dates.nth(1), n: 2, pos: "tr" },
    { target: dlg.getByText("tidak dicek radius toko", { exact: false }), n: 3, pos: "tl" },
    { target: dlg.getByPlaceholder("Mis. Gudang Cikarang"), n: 4, pos: "tl" },
    { target: dlg.locator("textarea"), n: 5, pos: "tl" },
    { target: dlg.getByRole("button", { name: "Kirim pengajuan" }), n: 6, pos: "tr" },
  ]);
  await dlg.getByRole("button", { name: "Batal" }).click();
  await ctx.close();

  const actx = await newContext(browser, { who: "andi" });
  const ap = await open(actx, "/dashboard/me/perjalanan-dinas", "Persetujuan Perjalanan Dinas Tim", 1000);
  const team = ap.locator("div.rounded-xl").filter({ hasText: "Persetujuan Perjalanan Dinas Tim" }).first();
  const row = team.locator("li").first();
  await shot(ap, "08-persetujuan-tim", team, [
    { target: row.locator("div.min-w-0"), n: 1, pos: "tl" },
    { target: row.getByRole("button", { name: "Setujui" }), n: 2, pos: "bl" },
    { target: row.getByRole("button", { name: "Tolak" }), n: 3, pos: "br" },
  ]);
  await actx.close();
}

/** 9. Lembur: halaman (penugasan + riwayat) dan form pengajuan. */
async function lembur(browser) {
  const ctx = await newContext(browser, { who: "rina" });
  const page = await open(ctx, "/dashboard/me/lembur", "Riwayat Lembur", 1200);
  const header = page.locator("div.border-b").filter({ hasText: "Ajukan Lembur" }).first();
  const assign = page.locator("div.rounded-xl").filter({ hasText: "Penugasan Lembur dari Perusahaan" }).first();
  const history = page.locator("div.rounded-xl").filter({ hasText: "Riwayat Lembur" }).first();
  await shot(page, "09-lembur", [header, assign, history], [
    { target: page.getByRole("button", { name: "Ajukan Lembur" }), n: 1, pos: "tl" },
    { target: assign.locator("li").first(), n: 2, pos: "tl" },
    { target: history.locator("li").first(), n: 3, pos: "tl" },
  ]);
  await page.getByRole("button", { name: "Ajukan Lembur" }).click();
  const dlg = dialog(page, "Ajukan Lembur");
  await dlg.getByText("Ajukan Lembur").waitFor();
  await page.waitForTimeout(500);
  await dlg.locator('input[type="date"]').fill(nextWeekday(5, 2));
  await dlg.locator('input[type="time"]').nth(0).fill("17:00");
  await dlg.locator('input[type="time"]').nth(1).fill("19:00");
  await dlg.getByPlaceholder("cth. closing stok bulanan").fill("Bongkar kiriman stok baru");
  await shot(page, "09-form-lembur", dlg, [
    { target: dlg.locator('input[type="date"]'), n: 1, pos: "tl" },
    { target: dlg.locator('input[type="time"]').nth(0), n: 2, pos: "tl" },
    { target: dlg.locator('input[type="time"]').nth(1), n: 3, pos: "tr" },
    { target: dlg.getByPlaceholder("cth. closing stok bulanan"), n: 4, pos: "tl" },
    { target: dlg.getByText("Lembur yang disetujui", { exact: false }), n: 5, pos: "tl" },
  ]);
  await dlg.getByRole("button", { name: "Batal" }).click();
  await ctx.close();
}

/** 10. Slip Gaji: daftar slip + rincian. */
async function slipGaji(browser) {
  const ctx = await newContext(browser, { who: "rina" });
  const page = await open(ctx, "/dashboard/me/slip-gaji", "Take home pay", 1100);
  const item = page.locator("li button").first();
  await shot(page, "10-slip-gaji", [page.locator("div.border-b").filter({ hasText: "Slip Gaji" }).first(), item], [
    { target: item, n: 1, pos: "tl" },
  ]);
  await item.click();
  const dlg = dialog(page, /Slip Gaji —/);
  await dlg.getByText("Unduh PDF").waitFor({ timeout: 30_000 });
  await page.setViewportSize({ width: 390, height: 1500 });
  await page.waitForTimeout(800);
  await shot(page, "10-slip-rincian", dlg, [
    { target: dlg.getByText("Unduh PDF"), n: 1, pos: "tl" },
  ], { maxHeight: 2400 });
  await ctx.close();
}

/** 11. Kalau ada masalah: contoh pesan salah kata sandi. */
async function masalah(browser) {
  const anon = await newContext(browser);
  const page = await open(anon, "/login", "Kata sandi");
  await page.getByPlaceholder("nama@perusahaan.com").fill(PEOPLE.rina.email);
  await page.locator('input[type="password"]').fill("sandi-salah");
  await page.getByRole("button", { name: "Masuk", exact: true }).click();
  const err = page.getByText(/salah|Invalid|gagal/i).first();
  await err.waitFor({ timeout: 30_000 });
  await page.waitForTimeout(500);
  const form = page.locator("form").first();
  await shot(page, "11-gagal-masuk", [form], [{ target: err, n: 1, pos: "tl" }]);
  await anon.close();
}

/* ───────────────────────────── jalankan ───────────────────────────── */

const SECTIONS = {
  masuk,
  "absen-masuk": absenMasuk,
  "luar-area": luarArea,
  "absen-pulang": absenPulang,
  riwayat,
  cuti,
  pinjaman,
  "perjalanan-dinas": perjalananDinas,
  lembur,
  "slip-gaji": slipGaji,
  masalah,
};

/**
 * Video kamera palsu: ilustrasi siluet (bukan wajah orang sungguhan) supaya
 * foto selfie di manual tidak berupa pola uji hijau Chromium. Ditulis sebagai
 * Y4M (I420) dari SVG lewat sharp.
 */
async function fakeCameraFile() {
  const W = 640, H = 480;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}">
    <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fce7f3"/><stop offset="1" stop-color="#e0e7ff"/></linearGradient></defs>
    <rect width="${W}" height="${H}" fill="url(#g)"/>
    <rect x="40" y="60" width="120" height="300" rx="10" fill="#f5d0e6"/><rect x="480" y="60" width="120" height="300" rx="10" fill="#dbe4ff"/>
    <rect x="52" y="90" width="96" height="14" rx="4" fill="#fbcfe8"/><rect x="52" y="150" width="96" height="14" rx="4" fill="#fbcfe8"/>
    <rect x="492" y="90" width="96" height="14" rx="4" fill="#c7d2fe"/><rect x="492" y="150" width="96" height="14" rx="4" fill="#c7d2fe"/>
    <ellipse cx="320" cy="500" rx="190" ry="150" fill="#db2777"/>
    <circle cx="320" cy="205" r="92" fill="#f2c4a8"/>
    <path d="M228 200 Q232 100 320 98 Q408 100 412 200 Q395 140 320 138 Q245 140 228 200Z" fill="#3f2a2a"/>
    <circle cx="290" cy="210" r="8" fill="#3f2a2a"/><circle cx="350" cy="210" r="8" fill="#3f2a2a"/>
    <path d="M292 250 Q320 272 348 250" stroke="#9f4a3a" stroke-width="6" fill="none" stroke-linecap="round"/>
  </svg>`;
  const { data } = await sharp(Buffer.from(svg)).removeAlpha().raw().toBuffer({ resolveWithObject: true });
  const Y = Buffer.alloc(W * H), U = Buffer.alloc((W / 2) * (H / 2)), V = Buffer.alloc((W / 2) * (H / 2));
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
    const i = (y * W + x) * 3, r = data[i], g = data[i + 1], b = data[i + 2];
    Y[y * W + x] = Math.round(0.257 * r + 0.504 * g + 0.098 * b + 16);
    if (y % 2 === 0 && x % 2 === 0) {
      const j = (y / 2) * (W / 2) + x / 2;
      U[j] = Math.round(-0.148 * r - 0.291 * g + 0.439 * b + 128);
      V[j] = Math.round(0.439 * r - 0.368 * g - 0.071 * b + 128);
    }
  }
  const frame = Buffer.concat([Buffer.from("FRAME\n"), Y, U, V]);
  const file = path.join(require("os").tmpdir(), `manual-ess-camera-${process.pid}.y4m`);
  fs.writeFileSync(file, Buffer.concat([Buffer.from(`YUV4MPEG2 W${W} H${H} F10:1 Ip A1:1 C420jpeg\n`), ...Array(10).fill(frame)]));
  return file;
}

(async () => {
  const wanted = process.argv.slice(2);
  const names = wanted.length ? wanted : Object.keys(SECTIONS);
  for (const n of names) if (!SECTIONS[n]) throw new Error(`Bagian tidak dikenal: ${n} (pilihan: ${Object.keys(SECTIONS).join(", ")})`);
  fs.mkdirSync(OUT, { recursive: true });
  const camera = await fakeCameraFile();
  // Chromium penuh (bukan headless-shell) + --lang=id supaya input tanggal/file
  // tampil seperti di HP berbahasa Indonesia (dd/mm/tttt, "Pilih File").
  const browser = await chromium.launch({
    channel: process.env.PW_CHANNEL || "chromium",
    args: ["--lang=id", "--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream",
      `--use-file-for-fake-video-capture=${camera}`],
    env: { ...process.env, LANG: "id_ID.UTF-8", LANGUAGE: "id" },
  });
  let failed = 0;
  try {
    for (const n of names) {
      console.log(`▶ ${n}`);
      try {
        await SECTIONS[n](browser);
      } catch (e) {
        failed++;
        console.error(`✗ ${n}: ${e.message.split("\n")[0]}`);
        if (process.env.DEBUG) console.error(e);
      }
    }
  } finally {
    await browser.close();
    fs.rmSync(camera, { force: true });
  }
  if (failed) process.exit(1);
})();

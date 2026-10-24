#!/usr/bin/env node
/**
 * Bangun PDF Manual Book ESS Karyawan dari manual.html (lihat README.md).
 *
 *   node build-pdf.cjs [output.pdf]
 *
 * Langkah: (1) cetak isi (tanpa sampul) dengan nomor halaman di footer,
 * (2) cari halaman tiap penanda "BAGIAN NN" lewat pdftotext lalu isi daftar
 * isi, cetak ulang sampai nomornya stabil, (3) cetak sampul tanpa footer,
 * (4) gabungkan dengan pdfunite. Butuh poppler-utils (pdftotext, pdfunite,
 * pdfinfo) dan Playwright + Chromium dari FRONTEND_DIR.
 * Nomor halaman: sampul tidak bernomor, halaman daftar isi = 1.
 */

const path = require("path");
const fs = require("fs");
const os = require("os");
const { execFileSync } = require("child_process");
const KEEP = process.env.KEEP_TMP;

const FRONTEND_DIR = process.env.FRONTEND_DIR || "/home/wit/Desktop/paddydev/frontend";
const { chromium } = require(path.join(FRONTEND_DIR, "node_modules/playwright"));

const OUT = path.resolve(process.argv[2] || path.join(os.tmpdir(), "Manual-Book-ESS-Karyawan-Paddy-v2.pdf"));
const HTML = path.join(__dirname, "manual.html");
const TMP = fs.mkdtempSync(path.join(os.tmpdir(), "manual-ess-"));

const FOOTER = `<div style="width:100%;font-size:7.5pt;color:#8a8a94;padding:0 14mm;display:flex;justify-content:space-between;font-family:'Ubuntu Sans','Noto Sans',Arial,sans-serif">
  <span>Manual Book ESS Karyawan · Paddy HRIS · Versi 2</span><span>Halaman <span class="pageNumber"></span> dari <span class="totalPages"></span></span></div>`;

const PDF_BODY = {
  format: "A4",
  printBackground: true,
  displayHeaderFooter: true,
  headerTemplate: "<span></span>",
  footerTemplate: FOOTER,
  margin: { top: "14mm", bottom: "16mm", left: "14mm", right: "14mm" },
};

function pageCount(file) {
  return Number(/Pages:\s+(\d+)/.exec(execFileSync("pdfinfo", [file], { encoding: "utf8" }))[1]);
}

/** { "01": 2, ... } — halaman pertama tempat "BAGIAN NN" muncul. */
function sectionPages(file) {
  const map = {};
  const n = pageCount(file);
  for (let i = 1; i <= n; i++) {
    const text = execFileSync("pdftotext", ["-f", String(i), "-l", String(i), file, "-"], { encoding: "utf8" });
    // Huruf berjarak (letter-spacing) bisa terbaca "B AG I A N 0 1": buang spasi dulu.
    for (const m of text.replace(/[ \t]+/g, "").matchAll(/BAGIAN(\d\d)/g)) if (!map[m[1]]) map[m[1]] = i;
  }
  return map;
}

(async () => {
  const browser = await chromium.launch({ channel: process.env.PW_CHANNEL || "chromium" });
  try {
    const page = await browser.newPage();
    await page.goto(`file://${HTML}`, { waitUntil: "load" });
    await page.evaluate(async () => {
      await document.fonts.ready;
      await Promise.all([...document.images].map((img) => (img.complete ? null : new Promise((r) => (img.onload = img.onerror = r)))));
    });
    const broken = await page.evaluate(() => [...document.images].filter((i) => !i.naturalWidth).map((i) => i.getAttribute("src")));
    if (broken.length) throw new Error(`Gambar tidak ditemukan: ${broken.join(", ")} (jalankan capture.cjs dulu)`);

    // Isi + daftar isi (diulang sampai nomor halaman stabil).
    await page.evaluate(() => document.body.classList.add("no-cover"));
    const body = path.join(TMP, "body.pdf");
    let map = {};
    for (let pass = 1; pass <= 4; pass++) {
      await page.evaluate((m) => {
        document.querySelectorAll("[data-toc]").forEach((el) => { el.textContent = m[el.dataset.toc] ?? "–"; });
      }, map);
      fs.writeFileSync(body, await page.pdf(PDF_BODY));
      const next = sectionPages(body);
      const stable = JSON.stringify(next) === JSON.stringify(map);
      map = next;
      if (stable) break;
    }
    const missing = (await page.evaluate(() => [...document.querySelectorAll("[data-toc]")].map((e) => e.dataset.toc)))
      .filter((k) => !map[k]);
    if (missing.length) throw new Error(`Penanda BAGIAN tidak ditemukan di PDF: ${missing.join(", ")}`);

    // Sampul tanpa footer dan tanpa margin.
    await page.evaluate(() => { document.body.classList.remove("no-cover"); document.body.classList.add("cover-only"); });
    const cover = path.join(TMP, "cover.pdf");
    fs.writeFileSync(cover, await page.pdf({ format: "A4", printBackground: true, margin: { top: 0, bottom: 0, left: 0, right: 0 }, pageRanges: "1" }));

    execFileSync("pdfunite", [cover, body, OUT]);
    console.log(`PDF: ${OUT}`);
    console.log(`Halaman: ${pageCount(OUT)} (sampul + ${pageCount(body)} halaman isi)`);
    console.log(`Daftar isi: ${Object.entries(map).map(([k, v]) => `${Number(k)}→${v}`).join("  ")}`);
  } finally {
    await browser.close();
    if (!KEEP) fs.rmSync(TMP, { recursive: true, force: true }); else console.log("tmp:", TMP);
  }
})().catch((e) => {
  console.error(e.message);
  process.exit(1);
});

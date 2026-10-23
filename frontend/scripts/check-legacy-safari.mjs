// Pastikan bundle klien bisa di-parse Safari 15 (iOS 15). Sintaks yang lebih baru
// (class static block, regex lookbehind) membuat satu chunk gagal dimuat dan
// halaman yang dirender di klien (mis. /login) tampil putih kosong.
// Jalankan setelah `next build`: node scripts/check-legacy-safari.mjs
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { parse } from "acorn";

const root = new URL("../.next/static/chunks/", import.meta.url).pathname;
const files = [];
(function walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path);
    else if (name.endsWith(".js")) files.push(path);
  }
})(root);

// Safari 15 mendukung class field & private field (ES2022), tetapi belum
// class static block (baru Safari 16.4) — cari node-nya di AST.
function findStaticBlock(node) {
  if (!node || typeof node.type !== "string") return null;
  if (node.type === "StaticBlock") return node;
  for (const value of Object.values(node)) {
    for (const child of Array.isArray(value) ? value : [value]) {
      if (child && typeof child === "object") {
        const found = findStaticBlock(child);
        if (found) return found;
      }
    }
  }
  return null;
}

const failures = [];
for (const file of files) {
  const code = readFileSync(file, "utf8");
  // swagger-ui-react (regex lookbehind di generator contoh JSON Schema) hanya
  // dimuat di /api/purchasing/docs — dokumentasi API internal, bukan halaman karyawan.
  if (code.includes("swaggerUIRedirectOauth")) continue;
  try {
    const staticBlock = findStaticBlock(
      parse(code, { ecmaVersion: 2022, sourceType: "script", allowHashBang: true, allowReturnOutsideFunction: true }),
    );
    if (staticBlock) {
      failures.push(`${file.slice(root.length)}: class static block — ${code.slice(staticBlock.start - 60, staticBlock.start + 40)}`);
      continue;
    }
  } catch (err) {
    failures.push(`${file.slice(root.length)}: ${err.message} — ${code.slice(Math.max(0, err.pos - 60), err.pos + 40)}`);
    continue;
  }
  const lookbehind = code.match(/.{0,40}\(\?<[=!].{0,40}/);
  if (lookbehind) failures.push(`${file.slice(root.length)}: regex lookbehind — ${lookbehind[0]}`);
}

if (failures.length > 0) {
  console.error(`${failures.length} dari ${files.length} chunk tidak didukung Safari 15:\n${failures.join("\n")}`);
  process.exit(1);
}
console.log(`${files.length} chunk OK untuk Safari 15.`);

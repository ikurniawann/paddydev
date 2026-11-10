# EPIC-101: Bagan Organisasi Draft 6 — Master Data Fase 1 (`MODULE-HRIS`)

status: done
environment: dev
retries: 0

## Goal
Permintaan user (10-11-2026): epic struktur organisasi berdasarkan analisa **EPIC-HRIS-ORG-01**. Sumbernya `Org_Chart_Paddy_2026_-_Draft_6_1.xlsx` (SmartArt, diubah 05-11-2026). Analisa itu sudah dipetakan ke struktur HRIS yang ada.

Artefak klien di `docs/hris/org-draft6/`:
- `paddy_org_positions_seed.csv`: 150 node (149 posisi + 1 grup `Support`).
- `paddy_store_master_draft.csv`: 33 toko.
- `paddy_org_smartart_import.py`: importer referensi; xlsx sumbernya tidak disertakan.
- `title_aliases.csv`: peta jabatan HRIS → Draft 6, disusun dari data 10-11.

## Struktur yang ada vs usulan (staging = salinan prod 10-11)
| Usulan analisa | Yang sudah ada | Keputusan |
|---|---|---|
| `org_units` (8 divisi + 7 region) | `hris.departments` (15, berhierarki `parent_department_id`) | Divisi dan region disimpan sebagai kolom node bagan; departemen tidak diubah |
| `job_titles` (102) | `hris.positions` (233 judul; 111 dipakai 313 karyawan aktif) | Katalog tetap. Peta alias menghubungkan judul HRIS ke judul Draft 6 (66 cocok persis, 43 lewat alias) |
| `job_levels` L0–L8 | Teks level di `hris.positions.level` (C-Level … Staff), dipakai approval EPIC-091/092 | Level Draft 6 hanya disimpan di node bagan; level jabatan karyawan tidak berubah |
| `locations` (33 toko) | `configuration.branches`: 33 toko Draft 6 ada semua; 8 toko aktif lain tidak ada di bagan (BXST, MCCM, PPBI, PPCN, PPCY, PPMT, TWMS, TWSP) | Kode toko node dicocokkan ke branch lewat kode tanpa `ODOO-` |
| `positions` (slot) + `reports_to` posisi | Garis lapor antar orang `employees.reporting_to` (306/318), dipakai approval live | Slot bagan disimpan. Approval **tidak** dipindah ke posisi |
| `employee_positions` | Belum ada | Fase 1 hanya mencocokkan karyawan ke node saat laporan, tanpa menyimpan penugasan |
| Halaman org chart | `/dashboard/hris/org-chart` (menu `hris.organization.org-chart`) berisi pohon karyawan dari `reporting_to` | Tambah tab **Bagan Draft 6** dan **Selisih** |

## Ruling (keputusan user 10-11)
- **R1 — Pakai struktur yang ada + tabel bagan.**
  - Tabel baru `hris.org_chart_versions`, `hris.org_chart_nodes`, `hris.org_title_aliases`.
  - `hris.employees`, `hris.positions`, `hris.departments`, `configuration.branches`, dan alur approval **tidak diubah**.
- **R2 — Fase 1 saja:**
  - skema + seed Draft 6;
  - validasi BR-01..05, 07, 09, 10;
  - tab Bagan (lihat saja);
  - laporan selisih bagan vs data karyawan (JSON + Excel untuk HR).

  Ditunda ke epic berikutnya: importer generik Draft 7 (ORG-06), approve versi, approver berbasis posisi (ORG-04), dan dashboard vacancy (ORG-08).
- **R3 — Peta alias ditinjau HR.**
  - Judul HRIS dipetakan ke judul Draft 6 lewat `org_title_aliases` (kolom region untuk Store Supervisor).
  - Karyawan tidak dipindah otomatis.
  - Alias tebakan ditandai `needs_review` (13) dan ikut muncul di laporan.
- **R4 — Level hanya di bagan** sampai klien menjawab Q1 (grade resmi).

## Ruling (tanpa tanya)
- **R5 — Seed setia ke CSV klien.**
  - `title` = `normalized_title`, `raw_title` disimpan.
  - `needs_confirm` = `level_confidence = needs_confirm`, atau flag `HIGHLIGHTED_NO_LEGEND` / `DUPLICATE_CANDIDATE` / `AMBIGUOUS_ABBR` (20 node).
  - `status = planned` untuk `HIGHLIGHTED_NO_LEGEND` (9), selain itu `active`.
  - Q5 terjawab oleh data: alias `HR Operations & Compensation Staff` → `HR Operations & Com Staff`. Node tetap sesuai CSV.
- **R6 — Kode toko node disimpan sebagai array teks (`store_codes`)**, dicocokkan ke branch saat dibaca: `upper(regexp_replace(code, '^ODOO-', ''))`. Ini tetap berlaku sesudah HR merapikan kode lewat EPIC-100.
- **R7 — Seed lewat migrasi data.**
  - SQL dibuat oleh `tools/orgchart/draft6_seed_sql.py` dari CSV, idempoten (`ON CONFLICT DO NOTHING`).
  - Hanya mengisi tabel baru, jadi aman berjalan di staging dan production saat deploy.
- **R8 — Akses:** tab Bagan dan Selisih memakai menu yang sudah ada `hris.organization` (`withMenu(iam.HrisOrganization)`). Tidak ada menu baru.
- **R9 — Pencocokan karyawan → node (laporan):**
  1. Judul kanonis = alias, atau judul jabatan itu sendiri.
  2. Kandidat = node dengan judul sama (case-insensitive) dan region alias, bila ada.
  3. Bila ada lebih dari satu kandidat, saring dengan toko penempatan utama aktif.
  4. Sisa lebih dari satu = "ambigu".

  Garis lapor dicek terhadap node induk (melewati node grup). Perbedaan hanya dilaporkan bila node induk punya pemegang.

## Business rules yang divalidasi (dari analisa §6)
| BR | Aturan | Hasil Draft 6 |
|---|---|---|
| BR-01 | Tepat satu root | lulus (CEO, EXE-001) |
| BR-02 | Tanpa siklus | lulus |
| BR-03 | Induk ada di versi yang sama | lulus (FK komposit) |
| BR-04 | Rank anak > induk; sama = peringatan, lebih senior = error | **3 peringatan**: HRG-004, HRG-010, MKT-004 |
| BR-05 | Store Leader / Shopkeeper / Perfumer / Patcher / Penjahit / Store Support di RTL wajib ≥ 1 kode toko | lulus |
| BR-07 | Tidak ada judul hanya `Intern` / `Freelance` | lulus |
| BR-09 | Node grup tidak punya level atau tipe kepegawaian | lulus (`SUP-001`) |
| BR-10 | `OUTSOURCE` wajib vendor | lulus (Security, Red Guard) |

## Tasks
Rencana: `docs/superpowers/plans/2026-11-10-bagan-organisasi-draft6.md`.
- [x] T1 Skema bagan berversi + seed Draft 6 (migrasi data dari CSV klien) — done
- [x] T2 Validasi BR + pencocokan karyawan → node (domain) — done
- [x] T3 API versi, pohon, validasi, laporan selisih JSON + Excel — done
- [x] T4 Tab Bagan Draft 6 + Selisih di halaman Struktur Organisasi, gate, QA staging — done

## Acceptance Criteria
- [x] Seed Draft 6:
  - 1 versi `Draft 6` (draft) dengan 150 node, 149 posisi, 33 kode toko unik, root CEO;
  - 20 node `needs_confirm`, 9 node `planned`;
  - 43 alias.
  - Seed yang dijalankan dua kali hasilnya sama.
- [x] Validasi Draft 6: 0 error dan 3 peringatan BR-04.
- [x] `GET /api/hris/org-chart/tree?division=RTL&region=Makassar` → 4 node.
- [x] Laporan selisih di staging (JSON + Excel) berisi:
  - per karyawan aktif: node bagan atau alasannya;
  - posisi bagan tanpa pemegang;
  - jabatan HRIS di luar bagan;
  - toko aktif di luar bagan.
- [x] Tab **Bagan Draft 6**: pohon bisa dilipat, filter divisi/region, badge status (active/planned) dan "perlu konfirmasi" dengan legenda, kode toko, nama pemegang. Bisa dipakai dengan keyboard dan di HP 360 px.
- [x] Data karyawan, jabatan, departemen, toko, dan approval tidak berubah.
- [x] Gate hijau: Go `./internal/...`, vitest, tsc, eslint, dan CI PR.

## Pertanyaan klien yang masih terbuka (dari analisa §9)
Q1 grade resmi, Q2 kepala Support, Q3 arti warna, Q4 judul fungsi, Q6 approver staf, Q7 Inventory Control ganda, Q9 entitas legal, Q10 headcount, Q11 toko tanpa Store Leader, Q12 template tabular. Q5 terjawab oleh data (Compensation). Q8 sebagian terjawab: nama, alamat, dan koordinat 33 toko sudah ada di `configuration.branches` (EPIC-069); arti prefix brand belum.

## Automation Log
- 2026-11-10: Analisa klien diterima. Pencocokan dengan staging (read-only):
  - 33/33 toko ada;
  - 66/102 judul cocok persis;
  - 53 judul yang dipakai karyawan tidak ada di Draft 6;
  - 8 jabatan Coordinator/Lead berlevel Staff di HRIS.

  Keputusan user lewat AskUserQuestion: pakai struktur yang ada + tabel bagan, Fase 1, alias ditinjau HR, level hanya di bagan. Rencana ditulis. Branch `feat/struktur-organisasi` dari main `6567c297`.
- 2026-11-10: **Implementasi Native** di branch `feat/struktur-organisasi`:
  - T1 `1bc7b77f`: tabel `hris.org_chart_versions`, `hris.org_chart_nodes`, `hris.org_title_aliases`; seed dibuat oleh `tools/orgchart/draft6_seed_sql.py` dan terbukti idempoten (dijalankan dua kali);
  - T2 `77978703`: `domain.ValidateOrgChart` dan `MatchOrgChart`;
  - T3 `3f157483`: 4 endpoint `GET /api/hris/org-chart/*` dengan `withMenu(iam.HrisOrganization)`;
  - T4 `14359b9d`: tab **Bagan Draft 6** dan **Selisih**; isi halaman lama dipindah utuh ke tab **Struktur Aktual**.
- Review akhir (opus): Critical 0, Important 2, Minor 7.
  - Diperbaiki di `ac181353`, masing-masing dengan tes RED→GREEN:
    - atasan yang belum terpetakan membuat bawahannya salah dilaporkan "atasan berbeda";
    - error muat tampil sebagai data kosong;
    - loop pada bagan bersiklus (dinaikkan ke Important karena bisa membuat server tergantung).
  - QA staging menemukan 5 karyawan tanpa jabatan tampil sebagai baris kosong; diperbaiki di `d2940267`.
  - Minor ditunda:
    - jabatan dengan satu kandidat tidak dicek tokonya;
    - kunci alias tidak memadatkan spasi ganda;
    - pola ARIA tree belum lengkap (navigasi panah);
    - label jumlah posisi ikut menghitung node grup;
    - tidak ada tes posisi di bawah node grup.
- Gate:
  - Go: gofmt 0, vet, `./internal/...` 137 ok;
  - frontend: vitest org-chart 8/8, tsc 0, eslint bersih.
- Staging 15.08 (image `local-20261110-150853`, backup `/mnt/hdd1/backup/paddydev/staging-before-e101b-20261110-150852.dump`):
  - versi Draft 6 berisi 150 node; validasi 0 error dan 3 peringatan (HRG-004, HRG-010, MKT-004);
  - tree Makassar berisi 4 node; filter `division=XXX` → 200;
  - halaman 200.
  - **Selisih** (318 karyawan aktif): 268 cocok, 22 ambigu, 28 di luar bagan (12 Warehouse Staff, 5 tanpa jabatan), 116 beda atasan, 37 lewat alias yang perlu ditinjau, 18 posisi tanpa pemegang. 8 toko aktif di luar bagan: BXST, MCCM, PPBI, PPCN, PPCY, PPMT, TWMS, TWSP.
  - Beda atasan terbesar: shopkeeper yang di HRIS langsung melapor ke Store Supervisor, padahal di bagan ke Store Leader; juga staf gudang yang langsung ke Warehouse Supervisor, padahal di bagan ke Coordinator. Ini data nyata untuk ditinjau HR, bukan galat.
  - Laporan HR: `.share/org-draft6-20261110/selisih-bagan.xlsx`.
  - Checksum `hris.employees`, `hris.positions`, `hris.departments`, `configuration.branches`, `hris.employee_placements` sebelum dan sesudah deploy identik: data tidak berubah. Sesi uji dihapus.

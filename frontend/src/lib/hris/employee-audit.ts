import { scheduleTypeLabel, type ScheduleType } from "./schedule-type";

const ACTIONS: Record<string, string> = {
  "employee.create": "Karyawan ditambahkan",
  "employee.update": "Data diubah",
  "employee.resign": "Resign",
  "employee.rehire": "Diaktifkan kembali",
  "employee.email_released": "Email dilepas",
  "employee.schedule_type": "Tipe jadwal diubah",
  "employee.import": "Diubah lewat import",
  "schedule.fixed_apply": "Jadwal tetap diterapkan",
  "schedule.bulk_upload": "Jadwal diunggah",
};

const FIELDS: Record<string, string> = {
  nip: "NIP", full_name: "Nama Lengkap", email: "Email", phone: "No. HP", join_date: "Tanggal Masuk",
  end_date: "Tanggal Keluar", employment_status: "Status Kepegawaian", schedule_type: "Tipe Jadwal",
  is_active: "Aktif", department_id: "Departemen", section_id: "Seksi", job_title_id: "Jabatan",
  reporting_to: "Atasan", ktp: "NIK (KTP)", npwp: "NPWP", birth_date: "Tanggal Lahir", gender: "Jenis Kelamin",
  marital_status: "Status Pernikahan", address: "Alamat", city: "Kota", province: "Provinsi",
  postal_code: "Kode Pos", bank_name: "Nama Bank", bank_account: "No. Rekening",
  bpjs_tk: "No. BPJS Ketenagakerjaan", bpjs_kesehatan: "No. BPJS Kesehatan",
  emergency_contact_name: "Kontak Darurat (Nama)", emergency_contact_phone: "Kontak Darurat (No. HP)",
  emergency_contact_relationship: "Kontak Darurat (Hubungan)", notes: "Catatan",
  effective_from: "Berlaku Mulai", resign_type: "Jenis", reason: "Alasan", branch: "Cabang", branch_id: "Cabang Penempatan",
  reactivate_login: "Akun Login Diaktifkan", from: "Dari", to: "Sampai", days: "Jumlah Hari", released_to: "Dipindah ke Karyawan Baru",
};

const STATUS: Record<string, string> = {
  probation: "Masa Percobaan", contract: "Kontrak (PKWT)", permanent: "Tetap (PKWTT)", intern: "Magang",
  freelance: "Freelance", daily_worker: "Pekerja Harian", resigned: "Resign", terminated: "Diberhentikan",
  resign: "Resign", end_of_contract: "Kontrak berakhir",
};

export const auditActionLabel = (a: string) => ACTIONS[a] ?? a;
export const auditFieldLabel = (f: string) => FIELDS[f] ?? f;

export function auditValue(field: string, v: unknown): string {
  if (v === null || v === undefined || v === "") return "—";
  if (typeof v === "boolean") return v ? "Ya" : "Tidak";
  const s = String(v);
  if (field === "schedule_type") return scheduleTypeLabel(s as ScheduleType);
  if (field === "employment_status" || field === "resign_type") return STATUS[s] ?? s;
  if (/^\d{4}-\d{2}-\d{2}/.test(s)) return `${s.slice(8, 10)}/${s.slice(5, 7)}/${s.slice(0, 4)}`;
  return s;
}

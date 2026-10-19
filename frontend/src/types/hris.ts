// ============================================================
// HRIS Module - TypeScript Types
// Fase 0: Foundation
// ============================================================

// ============================================================
// ENUMS
// ============================================================

export type EmploymentStatus =
  | 'probation'
  | 'contract'
  | 'permanent'
  | 'internship'
  | 'resigned'
  | 'terminated'
  | 'suspended';

export type GenderType = 'male' | 'female';

export type MaritalStatusType = 'single' | 'married' | 'divorced' | 'widowed';

// ============================================================
// DEPARTMENT
// ============================================================

export interface Department {
  id: string;
  name: string;
  code: string;
  description: string | null;
  brand_id: string | null;
  parent_department_id: string | null;
  cost_center: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  // Joined fields
  parent_department?: Department;
  sub_departments?: Department[];
  brand?: Brand;
}

export interface DepartmentWithRelations extends Department {
  employee_count?: number;
  parent_department?: Department;
  sub_departments?: Department[];
}

// ============================================================
// EMPLOYEE
// ============================================================

export interface Employee {
  // Core Identity
  id: string;
  user_id: string | null;
  old_staff_id: string | null;
  
  // Personal Information
  full_name: string;
  nip: string;
  ktp: string | null;
  npwp: string | null;
  email: string;
  phone: string;
  birth_date: string | null;
  gender: GenderType | null;
  marital_status: MaritalStatusType | null;
  address: string | null;
  city: string | null;
  province: string | null;
  postal_code: string | null;
  
  // Employment Information
  join_date: string;
  end_date: string | null;
  employment_status: EmploymentStatus;
  is_active: boolean;
  /** Tipe jadwal (EPIC-056): shift = Jadwal Kerja, non_shift = jadwal tetap. */
  schedule_type?: "shift" | "non_shift";
  
  // Organization Structure
  department_id: string | null;
  section_id: string | null;
  job_title_id: string | null;
  reporting_to: string | null;
  
  // Compensation & Benefits
  bank_name: string | null;
  bank_account: string | null;
  bpjs_tk: string | null;
  bpjs_kesehatan: string | null;
  
  // Emergency Contact
  emergency_contact_name: string | null;
  emergency_contact_phone: string | null;
  emergency_contact_relationship: string | null;
  
  // Metadata
  photo_url: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string;
  
  // Relasi (hanya ada saat data di-join dari query dengan embed)
  department?: Department | null;
  section?: Section | null;
  job_title?: Position | null;
  manager?: Employee | null;
}

export interface EmployeeWithRelations extends Employee {
  // Joined fields
  direct_reports?: Employee[];
  user?: User;
}

// ============================================================
// EXISTING TYPES (Re-export dari index.ts)
// ============================================================

import type { Brand, Position, User } from './index';

// Section dari staff module (existing table)
export interface Section {
  id: string;
  brand_id: string;
  name: string;
  code: string;
  description: string | null;
  color: string;
  is_active: boolean;
  created_at: string;
}

// Staff dari staff module (existing table - untuk backward compatibility)
export interface Staff {
  id: string;
  full_name: string;
  employee_code: string;
  email: string | null;
  phone: string | null;
  position_id: string | null;
  brand_id: string;
  hire_date: string;
  status: 'active' | 'inactive' | 'resigned';
  photo_url: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string;
}

// ============================================================
// PROMOTION (Talent Pool → Employee Integration)
// ============================================================

export interface PromotionRequest {
  candidate_id: string;
  join_date?: string;
  employment_status?: EmploymentStatus;
  department_id?: string;
  reporting_to?: string;
}

export interface PromotionResponse {
  employee_id: string;
  nip: string;
  message: string;
}

// ============================================================
// ORGANIZATION CHART
// ============================================================

export interface OrgNode {
  id: string;
  name: string;
  type: 'department' | 'employee';
  children?: OrgNode[];
  // Employee-specific
  job_title?: string;
  email?: string;
  photo_url?: string;
  is_active?: boolean;
  // Department-specific
  code?: string;
  employee_count?: number;
}

// ============================================================
// API REQUEST/RESPONSE TYPES
// ============================================================

export interface EmployeeListFilters {
  search?: string;
  department_id?: string;
  section_id?: string;
  employment_status?: EmploymentStatus;
  is_active?: boolean;
  page?: number;
  limit?: number;
  sort_by?: 'full_name' | 'join_date' | 'nip' | 'created_at';
  sort_order?: 'asc' | 'desc';
}

export interface EmployeeCreateData {
  nip?: string;
  full_name: string;
  email: string;
  phone: string;
  join_date: string;
  employment_status: EmploymentStatus;
  department_id?: string;
  section_id?: string;
  job_title_id?: string;
  reporting_to?: string;
  ktp?: string;
  npwp?: string;
  birth_date?: string;
  gender?: GenderType;
  marital_status?: MaritalStatusType;
  address?: string;
  city?: string;
  province?: string;
  postal_code?: string;
  bank_name?: string;
  bank_account?: string;
  bpjs_tk?: string;
  bpjs_kesehatan?: string;
  emergency_contact_name?: string;
  emergency_contact_phone?: string;
  emergency_contact_relationship?: string;
  photo_url?: string;
  notes?: string;
}

export interface EmployeeUpdateData extends Partial<EmployeeCreateData> {
  nip?: string;
  end_date?: string;
  is_active?: boolean;
  updated_at?: string;
}

export interface ApiResponse<T> {
  data?: T;
  error?: string;
  message?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  per_page: number;
}

// ============================================================
// FORM TYPES (React Hook Form)
// ============================================================

export interface EmployeeFormData {
  full_name: string;
  email: string;
  phone: string;
  join_date: string;
  employment_status: EmploymentStatus;
  department_id?: string;
  section_id?: string;
  job_title_id?: string;
  reporting_to?: string;
  ktp?: string;
  npwp?: string;
  birth_date?: string;
  gender?: GenderType;
  marital_status?: MaritalStatusType;
  address?: string;
  city?: string;
  province?: string;
  postal_code?: string;
  bank_name?: string;
  bank_account?: string;
  bpjs_tk?: string;
  bpjs_kesehatan?: string;
  emergency_contact_name?: string;
  emergency_contact_phone?: string;
  emergency_contact_relationship?: string;
  photo_url?: string;
  notes?: string;
}

export interface PromoteCandidateFormData {
  join_date: string;
  employment_status: EmploymentStatus;
  department_id?: string;
  reporting_to?: string;
}

// ============================================================
// HELPER FUNCTIONS
// ============================================================

export const EMPLOYMENT_STATUS_LABELS: Record<EmploymentStatus, string> = {
  probation: 'Probation',
  contract: 'Kontrak',
  permanent: 'Tetap',
  internship: 'Magang',
  resigned: 'Mengundurkan Diri',
  terminated: 'Diberhentikan',
  suspended: 'Diskorsing',
};

export const EMPLOYMENT_STATUS_COLORS: Record<EmploymentStatus, string> = {
  probation: 'bg-yellow-500',
  contract: 'bg-blue-500',
  permanent: 'bg-green-500',
  internship: 'bg-purple-500',
  resigned: 'bg-gray-500',
  terminated: 'bg-red-500',
  suspended: 'bg-orange-500',
};

export const GENDER_LABELS: Record<GenderType, string> = {
  male: 'Laki-laki',
  female: 'Perempuan',
};

export const MARITAL_STATUS_LABELS: Record<MaritalStatusType, string> = {
  single: 'Belum Menikah',
  married: 'Menikah',
  divorced: 'Cerai Hidup',
  widowed: 'Cerai Mati',
};

// ============================================================
// FASE 1 - CORE HR OPERATIONS TYPES
// ============================================================

// Attendance Types
export type AttendanceStatus = 'present' | 'late' | 'absent' | 'half-day' | 'remote';

export interface AttendanceLocation {
  latitude: number;
  longitude: number;
  accuracy?: number;
  address?: string;
  ip_address?: string;
  user_agent?: string;
}

export interface Attendance {
  id: string;
  employee_id: string;
  date: string;
  clock_in: string | null;
  clock_out: string | null;
  clock_in_location: AttendanceLocation | null;
  clock_out_location: AttendanceLocation | null;
  work_hours: number | null;
  break_minutes: number;
  status: AttendanceStatus;
  is_late: boolean;
  late_minutes: number;
  is_overtime: boolean;
  overtime_hours: number;
  validated_by: string | null;
  validated_at: string | null;
  validation_notes: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string;
}

export interface AttendanceWithEmployee extends Attendance {
  employee?: Employee;
}

// Leave Types
export type LeaveType =
  | 'annual'
  | 'sick'
  | 'maternity'
  | 'paternity'
  | 'unpaid'
  | 'emergency'
  | 'pilgrimage'
  | 'menstrual'
  | 'marriage'
  | 'bereavement'
  | 'personal';

export type LeaveStatus = 'pending' | 'approved' | 'rejected' | 'cancelled';

export interface Leave {
  id: string;
  employee_id: string;
  leave_type: LeaveType;
  start_date: string;
  end_date: string;
  total_days: number;
  reason: string;
  attachment_url: string | null;
  status: LeaveStatus;
  approved_by: string | null;
  approved_at: string | null;
  rejection_reason: string | null;
  created_at: string;
  updated_at: string;
}

export interface LeaveWithEmployee extends Leave {
  employee?: Employee;
  approver?: Employee;
}

// Leave Balance Types
export interface LeaveBalance {
  id: string;
  employee_id: string;
  year: number;
  annual_leave_total: number;
  annual_leave_used: number;
  annual_leave_remaining: number;
  sick_leave_used: number;
  unpaid_leave_used: number;
  maternity_leave_used: number;
  paternity_leave_used: number;
  created_at: string;
  updated_at: string;
}

export interface LeaveBalanceWithEmployee extends LeaveBalance {
  employee?: Employee;
}

// Onboarding Types
export type OnboardingCategory = 'admin' | 'it' | 'hr' | 'manager' | 'general';

export interface OnboardingTask {
  id: string;
  employee_id: string;
  task_name: string;
  category: OnboardingCategory;
  description: string | null;
  priority: number; // 1=high, 2=medium, 3=low
  due_date: string | null;
  due_days_after_join: number;
  completed: boolean;
  completed_at: string | null;
  completed_by: string | null;
  completion_notes: string | null;
  assigned_to: string | null;
  created_at: string;
}

export interface OnboardingTaskWithEmployee extends OnboardingTask {
  employee?: Employee;
  assignee?: Employee;
}

// Offboarding Types
export type ResignationType = 'voluntary' | 'termination' | 'layoff' | 'end_of_contract';
export type OffboardingStatus = 'submitted' | 'notice_period' | 'exit_interview' | 'completed';

export interface OffboardingChecklist {
  id: string;
  employee_id: string;
  resignation_type: ResignationType;
  resignation_date: string;
  last_working_day: string;
  reason: string | null;
  status: OffboardingStatus;
  exit_interview_date: string | null;
  exit_interview_conducted_by: string | null;
  exit_interview_notes: string | null;
  final_payroll_date: string | null;
  final_payroll_amount: number | null;
  final_payroll_notes: string | null;
  asset_return_status: Record<string, boolean>; // {laptop: true, id_card: false}
  clearance_hrd: boolean;
  clearance_hrd_notes: string | null;
  clearance_it: boolean;
  clearance_it_notes: string | null;
  clearance_finance: boolean;
  clearance_finance_notes: string | null;
  clearance_manager: boolean;
  clearance_manager_notes: string | null;
  completed_at: string | null;
  completed_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface OffboardingChecklistWithEmployee extends OffboardingChecklist {
  employee?: Employee;
  interviewer?: Employee;
}

// Employee Schedule Types
export type ShiftType = 'morning' | 'afternoon' | 'night' | 'flexible' | 'custom';

export interface EmployeeSchedule {
  id: string;
  employee_id: string;
  day_of_week: number; // 0=Sunday, 6=Saturday
  start_time: string; // TIME
  end_time: string; // TIME
  shift_type: ShiftType;
  break_minutes: number;
  is_off: boolean;
  overtime_allowed: boolean;
  max_overtime_hours: number;
  effective_from: string;
  effective_to: string | null;
  created_at: string;
  updated_at: string;
}

export interface EmployeeScheduleWithEmployee extends EmployeeSchedule {
  employee?: Employee;
}

// ============================================================
// LABELS & COLORS FOR FASE 1
// ============================================================

export const ATTENDANCE_STATUS_LABELS: Record<AttendanceStatus, string> = {
  present: 'Hadir',
  late: 'Terlambat',
  absent: 'Alpha',
  'half-day': 'Setengah Hari',
  remote: 'Remote',
};

export const ATTENDANCE_STATUS_COLORS: Record<AttendanceStatus, string> = {
  present: 'bg-green-100 text-green-800',
  late: 'bg-yellow-100 text-yellow-800',
  absent: 'bg-red-100 text-red-800',
  'half-day': 'bg-blue-100 text-blue-800',
  remote: 'bg-purple-100 text-purple-800',
};

export const LEAVE_TYPE_LABELS: Record<LeaveType, string> = {
  annual: 'Cuti Tahunan',
  sick: 'Sakit',
  maternity: 'Melahirkan',
  paternity: 'Cuti Ayah',
  unpaid: 'Cuti Tanpa Upah',
  emergency: 'Cuti Darurat',
  pilgrimage: 'Cuti Haji/Umrah',
  menstrual: 'Cuti Haid',
  marriage: 'Cuti Pernikahan',
  bereavement: 'Cuti Duka Cita',
  personal: 'Izin Pribadi',
};

export const LEAVE_STATUS_LABELS: Record<LeaveStatus, string> = {
  pending: 'Menunggu Persetujuan',
  approved: 'Disetujui',
  rejected: 'Ditolak',
  cancelled: 'Dibatalkan',
};

export const LEAVE_STATUS_COLORS: Record<LeaveStatus, string> = {
  pending: 'bg-yellow-100 text-yellow-800',
  approved: 'bg-green-100 text-green-800',
  rejected: 'bg-red-100 text-red-800',
  cancelled: 'bg-gray-100 text-gray-800',
};

export const ONBOARDING_CATEGORY_LABELS: Record<OnboardingCategory, string> = {
  admin: 'Administrasi',
  it: 'IT & Equipment',
  hr: 'HRD',
  manager: 'Manager',
  general: 'Umum',
};

export const ONBOARDING_PRIORITY_LABELS: Record<number, string> = {
  1: 'Tinggi',
  2: 'Sedang',
  3: 'Rendah',
};

export const RESIGNATION_TYPE_LABELS: Record<ResignationType, string> = {
  voluntary: 'Mengundurkan Diri',
  termination: 'PHK',
  layoff: 'Layoff',
  'end_of_contract': 'Akhir Kontrak',
};

export const OFFBOARDING_STATUS_LABELS: Record<OffboardingStatus, string> = {
  submitted: 'Diajukan',
  notice_period: 'Masa Pemberitahuan',
  exit_interview: 'Exit Interview',
  completed: 'Selesai',
};

export const SHIFT_TYPE_LABELS: Record<ShiftType, string> = {
  morning: 'Pagi',
  afternoon: 'Siang',
  night: 'Malam',
  flexible: 'Fleksibel',
  custom: 'Custom',
};

// Helper function untuk format NIP display
export const formatNIP = (nip: string): string => {
  // EMP-2026-00001 → EMP-2026-00001 (already formatted)
  return nip;
};

// Helper function untuk calculate tenure
export const calculateTenure = (joinDate: string): string => {
  const join = new Date(joinDate);
  const now = new Date();
  const years = now.getFullYear() - join.getFullYear();
  const months = now.getMonth() - join.getMonth();
  
  if (years > 0) {
    return `${years} tahun ${Math.max(0, months)} bulan`;
  } else if (months > 0) {
    return `${months} bulan`;
  } else {
    return 'Baru bergabung';
  }
};

// calculateLeaveDays() dihapus di EPIC-036 Fase D. Ia hanya mengecualikan akhir
// pekan, sehingga cuti yang melewati tanggal merah tetap memotong jatah — bug
// yang sama dengan calculateBusinessDays di endpoint cuti. Penggantinya
// describeLeaveDays() di src/lib/hris/holidays.ts, yang butuh HolidayIndex
// (ambil lewat fetchHolidayIndex di client, loadHolidayIndex di server).

// Helper function untuk get GPS location
export const getCurrentLocation = async (): Promise<AttendanceLocation | null> => {
  if (!navigator.geolocation) {
    return null;
  }
  
  return new Promise((resolve) => {
    navigator.geolocation.getCurrentPosition(
      (position) => {
        resolve({
          latitude: position.coords.latitude,
          longitude: position.coords.longitude,
          accuracy: position.coords.accuracy,
        });
      },
      () => {
        resolve(null); // Permission denied or error
      },
      {
        enableHighAccuracy: true,
        timeout: 10000,
        maximumAge: 0,
      }
    );
  });
};

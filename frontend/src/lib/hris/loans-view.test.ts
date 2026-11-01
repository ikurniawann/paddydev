import { describe, expect, it } from "vitest";
import {
  EMPTY_LOAN_FORM,
  LOAN_STATUS_LABELS,
  LOAN_TYPE_OPTIONS,
  employeeComboOptions,
  loanPaidPercent,
  loanPayload,
  loanSummary,
  loanDeductionLabel,
  loanPayrollPeriodLabel,
  loanTenorOptions,
  loanTermsText,
  loanTypeLabel,
  tenorForType,
  previewInstallment,
  validateLoanForm,
} from "./loans-view";

describe("loanTypeLabel", () => {
  it("label dikenal dan fallback ke kode", () => {
    expect(loanTypeLabel("kasbon")).toBe("Kasbon (Salary Advance)");
    expect(loanTypeLabel("lain")).toBe("lain");
  });
});

describe("employeeComboOptions", () => {
  it("menambahkan NIP bila ada", () => {
    expect(
      employeeComboOptions([
        { id: "1", full_name: "Ani", nip: "007" },
        { id: "2", full_name: "Budi", nip: null },
      ])
    ).toEqual([
      { value: "1", label: "Ani (007)" },
      { value: "2", label: "Budi" },
    ]);
  });
});

describe("previewInstallment", () => {
  it("tanpa bunga: pokok dibagi tenor, dibulatkan", () => {
    expect(previewInstallment({ ...EMPTY_LOAN_FORM, principal_amount: "1000000", tenor_months: "3" })).toBe(
      333_333
    );
  });

  it("bunga flat per bulan", () => {
    expect(
      previewInstallment({
        ...EMPTY_LOAN_FORM,
        principal_amount: "1000000",
        tenor_months: "10",
        interest_rate: "1",
      })
    ).toBe(110_000);
  });

  it("null bila pokok atau tenor kosong", () => {
    expect(previewInstallment(EMPTY_LOAN_FORM)).toBeNull();
    expect(previewInstallment({ ...EMPTY_LOAN_FORM, principal_amount: "500", tenor_months: "" })).toBeNull();
  });
});

describe("validateLoanForm & loanPayload", () => {
  it("karyawan wajib, lalu jumlah dan tenor", () => {
    expect(validateLoanForm(EMPTY_LOAN_FORM)).toBe("Pilih karyawan");
    expect(validateLoanForm({ ...EMPTY_LOAN_FORM, employee_id: "e1" })).toBe(
      "Jumlah pinjaman dan tenor wajib diisi"
    );
    expect(validateLoanForm({ ...EMPTY_LOAN_FORM, employee_id: "e1", principal_amount: "100" })).toBe(
      "Lampirkan minimal 1 dokumen pendukung kasbon"
    );
    expect(
      validateLoanForm({ ...EMPTY_LOAN_FORM, employee_id: "e1", principal_amount: "100", attachments: ["a.pdf"] })
    ).toBeNull();
  });

  it("payload berisi angka dan keperluan kosong dihilangkan", () => {
    expect(
      loanPayload({ ...EMPTY_LOAN_FORM, employee_id: "e1", principal_amount: "100", interest_rate: "" })
    ).toEqual({
      employee_id: "e1",
      loan_type: "kasbon",
      principal_amount: 100,
      interest_rate: 0,
      tenor_months: 2,
      purpose: undefined,
      attachments: [],
      collateral_attachments: [],
    });
  });

  it("jaminan hanya dikirim untuk Pinjaman Karyawan", () => {
    const form = { ...EMPTY_LOAN_FORM, attachments: ["d.pdf"], collateral_attachments: ["j.pdf"] };
    expect(loanPayload(form).collateral_attachments).toEqual([]);
    expect(loanPayload({ ...form, loan_type: "loan" }).collateral_attachments).toEqual(["j.pdf"]);
  });
});

describe("ringkasan & progres", () => {
  it("menghitung pending dan sisa pinjaman berjalan", () => {
    expect(
      loanSummary([
        { status: "pending", remaining_balance: "500" },
        { status: "approved", remaining_balance: "1000" },
        { status: "approved", remaining_balance: 250 },
        { status: "paid_off", remaining_balance: 0 },
      ])
    ).toEqual({ pendingCount: 1, activeTotal: 1250 });
  });

  it("pending HR: manager_approved dan pending tanpa manager; pending dengan manager belum", () => {
    expect(
      loanSummary([
        { status: "manager_approved", remaining_balance: 0, manager_approver_id: "m1" },
        { status: "pending", remaining_balance: 0, manager_approver_id: null },
        { status: "pending", remaining_balance: 0 },
        { status: "pending", remaining_balance: 0, manager_approver_id: "m1" },
      ])
    ).toEqual({ pendingCount: 3, activeTotal: 0 });
  });

  it("persentase terbayar dibulatkan dan aman saat total 0", () => {
    expect(loanPaidPercent({ paid_amount: "1", remaining_balance: "2" })).toBe(33);
    expect(loanPaidPercent({ paid_amount: 0, remaining_balance: 0 })).toBe(0);
    expect(loanPaidPercent({ paid_amount: 100, remaining_balance: 0 })).toBe(100);
  });
});

describe("kasbon EPIC-060", () => {
  it("Pinjaman Darurat tidak bisa diajukan tapi labelnya tetap untuk riwayat", () => {
    expect(LOAN_TYPE_OPTIONS.map((o) => o.value)).toEqual(["kasbon", "loan"]);
    expect(loanTypeLabel("emergency")).toBe("Pinjaman Darurat");
  });
  it("tenor kasbon 1 atau 2, default 2", () => {
    expect(loanTenorOptions("kasbon").map((o) => o.value)).toEqual(["1", "2"]);
    expect(EMPTY_LOAN_FORM.tenor_months).toBe("2");
    expect(
      validateLoanForm({ ...EMPTY_LOAN_FORM, employee_id: "e", principal_amount: "100000", tenor_months: "3" })
    ).toBe("Cicilan kasbon hanya 1 atau 2 kali");
  });
  it("status antara manager", () => {
    expect(LOAN_STATUS_LABELS.manager_approved).toBe("Disetujui Manager");
  });
});

describe("prosedur kasbon & Pinjaman Karyawan (EPIC-076)", () => {
  it("label jenis loan = Pinjaman Karyawan", () => {
    expect(loanTypeLabel("loan")).toBe("Pinjaman Karyawan");
  });

  it("cicilan Pinjaman Karyawan 1–6; tenor di luar batas ikut maksimum jenis", () => {
    expect(loanTenorOptions("loan").map((o) => o.value)).toEqual(["1", "2", "3", "4", "5", "6"]);
    expect(tenorForType("kasbon", "6")).toBe("2");
    expect(tenorForType("loan", "2")).toBe("2");
    expect(tenorForType("loan", "12")).toBe("6");
    const base = { ...EMPTY_LOAN_FORM, employee_id: "e", principal_amount: "100000", loan_type: "loan" };
    expect(validateLoanForm({ ...base, tenor_months: "7" })).toBe("Cicilan Pinjaman Karyawan 1 sampai 6 kali");
    expect(validateLoanForm({ ...base, tenor_months: "6" })).toBe("Lampirkan minimal 1 dokumen jaminan (surat berharga)");
    expect(validateLoanForm({ ...base, tenor_months: "6", collateral_attachments: ["j.pdf"] })).toBeNull();
  });

  it("syarat per jenis", () => {
    expect(loanTermsText("kasbon")).toContain("masa kerja minimal 3 bulan");
    expect(loanTermsText("kasbon")).toContain("maks 50% THP · 1–2 kali cicilan");
    expect(loanTermsText("loan")).toContain("masa kerja minimal 12 bulan");
    expect(loanTermsText("loan")).toContain("maks 200% THP · 1–6 kali cicilan");
    expect(loanTermsText("loan")).toContain("jaminan (surat berharga)");
  });

  it("periode potong 21–20", () => {
    expect(loanPayrollPeriodLabel("2026-11-20")).toBe("November 2026");
    expect(loanPayrollPeriodLabel("2026-11-21")).toBe("Desember 2026");
    expect(loanPayrollPeriodLabel("2026-12-21")).toBe("Januari 2027");
    expect(loanPayrollPeriodLabel("")).toBe("");
  });

  it("bulan potong: tersimpan, atau perkiraan dari tanggal pengajuan WIB", () => {
    expect(loanDeductionLabel({ first_installment_month: 12, first_installment_year: 2026 })).toBe("Desember 2026");
    // 20 Nov 23.30 WIB = 16.30 UTC
    expect(
      loanDeductionLabel({ first_installment_month: null, first_installment_year: null, created_at: "2026-11-20T16:30:00Z" })
    ).toBe("November 2026 (perkiraan)");
    expect(
      loanDeductionLabel({ first_installment_month: null, first_installment_year: null, created_at: "2026-11-20T17:30:00Z" })
    ).toBe("Desember 2026 (perkiraan)");
  });
});

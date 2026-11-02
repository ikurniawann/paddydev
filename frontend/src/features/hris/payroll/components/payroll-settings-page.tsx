"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ArrowLeftIcon, Cog6ToothIcon } from "@heroicons/react/24/outline";
import { toast } from "sonner";
import {
  BPJS_KES_FIELDS,
  BPJS_TK_FIELDS,
  BRACKET_FIELDS,
  COMPONENT_SWITCHES,
  LAINNYA_FIELDS,
  LATE_MODE_OPTIONS,
  LOAN_FIELDS,
  PPH21_BPJS_BASIS_OPTIONS,
  PPH21_METHOD_OPTIONS,
  PTKP_FIELDS,
  SETTINGS_KEYS,
  TAPERA_FIELDS,
  TAX_KEYS,
  TOGGLE_FIELDS,
  formStateToPayload,
  isSwitchOn,
  parseLateTiers,
  rowToFormState,
  type FieldDef,
  type FormState,
  type LateTierRule,
} from "@/lib/payroll/ui-settings-form";
import { AttendancePeriodSettingsCard } from "@/features/hris/attendance";
import { usePayrollSettings } from "../queries";
import { useSavePayrollSettings } from "../mutations";

interface FieldGridProps {
  fields: FieldDef[];
  state: FormState;
  onChange: (key: string, value: string) => void;
  /** Bagian dimatikan lewat saklar: input redup, nilai tetap tersimpan. */
  disabled?: boolean;
  /** Kunci yang tetap aktif walau bagian dimatikan. */
  alwaysEnabled?: string[];
}

function FieldGrid({ fields, state, onChange, disabled = false, alwaysEnabled = [] }: FieldGridProps) {
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4">
      {fields.map((field) => (
        <div key={field.key}>
          <label htmlFor={field.key} className="mb-1 block text-xs font-medium text-gray-600">
            {field.label}
            {field.suffix ? (
              <span className="ml-1 text-gray-400">({field.suffix})</span>
            ) : null}
          </label>
          <Input
            id={field.key}
            type="number"
            step="any"
            disabled={disabled && !alwaysEnabled.includes(field.key)}
            value={state[field.key] ?? ""}
            onChange={(e) => onChange(field.key, e.target.value)}
          />
        </div>
      ))}
    </div>
  );
}

function LateTierEditor({ value, onChange }: { value: string; onChange: (json: string) => void }) {
  const tiers = parseLateTiers(value);
  const update = (index: number, patch: Partial<LateTierRule>) =>
    onChange(JSON.stringify(tiers.map((t, i) => (i === index ? { ...t, ...patch } : t))));
  const num = (v: string) => (v === "" ? undefined : Math.max(0, Number(v)));
  return (
    <div className="mt-4">
      <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">Tingkat potongan per bulan</p>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-gray-500">
              <th className="py-1 pr-2 font-medium">Telat dari (kali)</th>
              <th className="py-1 pr-2 font-medium">Sampai (kali)</th>
              <th className="py-1 pr-2 font-medium">Potong (hari)</th>
              <th className="py-1 font-medium">Dasar per hari</th>
            </tr>
          </thead>
          <tbody>
            {tiers.map((tier, i) => (
              <tr key={i}>
                <td className="py-1 pr-2">
                  <Input type="number" min={1} value={tier.min} onChange={(e) => update(i, { min: num(e.target.value) ?? 1 })} />
                </td>
                <td className="py-1 pr-2">
                  <Input
                    type="number"
                    min={1}
                    placeholder="tanpa batas"
                    value={tier.max ?? ""}
                    onChange={(e) => update(i, { max: num(e.target.value) })}
                  />
                </td>
                <td className="py-1 pr-2">
                  <Input type="number" min={0} step="any" value={tier.days} onChange={(e) => update(i, { days: num(e.target.value) ?? 0 })} />
                </td>
                <td className="py-1">
                  <select
                    value={tier.basis}
                    onChange={(e) => update(i, { basis: e.target.value as LateTierRule["basis"] })}
                    className="w-full rounded-md border border-gray-200 px-2 py-2 text-sm"
                  >
                    <option value="thp">Take home pay ÷ pembagi</option>
                    <option value="base">Gaji pokok ÷ pembagi</option>
                  </select>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function PayrollSettingsPage() {
  const router = useRouter();
  const currentYear = new Date().getFullYear();
  const [taxYear, setTaxYear] = useState(currentYear);

  const { data, isLoading } = usePayrollSettings(taxYear);
  const saveMutation = useSavePayrollSettings();

  // Nilai tampilan = data tersimpan + edit lokal yang belum disimpan.
  const [settingsEdits, setSettingsEdits] = useState<FormState>({});
  const [taxEdits, setTaxEdits] = useState<FormState>({});

  const settingsForm: FormState = {
    ...rowToFormState(data?.settings ?? null, SETTINGS_KEYS),
    ...settingsEdits,
  };
  const taxForm: FormState = {
    ...rowToFormState(data?.tax_config ?? null, TAX_KEYS),
    ...taxEdits,
  };

  const pphOn = isSwitchOn(settingsForm, "pph21_enabled");
  const bpjsTkOn = isSwitchOn(settingsForm, "bpjs_tk_enabled");
  const bpjsKesOn = isSwitchOn(settingsForm, "bpjs_kes_enabled");

  const handleSettingsChange = (key: string, value: string) =>
    setSettingsEdits((prev) => ({ ...prev, [key]: value }));

  const handleTaxChange = (key: string, value: string) =>
    setTaxEdits((prev) => ({ ...prev, [key]: value }));

  const handleSave = async () => {
    const settingsPayload = formStateToPayload(settingsEdits);
    const taxPayload = formStateToPayload(taxEdits);

    if (
      Object.keys(settingsPayload).length === 0 &&
      Object.keys(taxPayload).length === 0
    ) {
      toast.error("Tidak ada perubahan untuk disimpan");
      return;
    }

    try {
      await saveMutation.mutateAsync({
        ...(Object.keys(settingsPayload).length > 0
          ? { settings: settingsPayload }
          : {}),
        ...(Object.keys(taxPayload).length > 0
          ? { tax_config: { tax_year: taxYear, ...taxPayload } }
          : {}),
      });
      setSettingsEdits({});
      setTaxEdits({});
      toast.success("Pengaturan payroll tersimpan");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menyimpan pengaturan");
    }
  };

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => router.push("/dashboard/hris/payroll")}
          >
            <ArrowLeftIcon className="mr-1 h-4 w-4" />
            Kembali
          </Button>
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-bold">
              <Cog6ToothIcon className="h-6 w-6 text-pink-600" />
              Pengaturan Payroll
            </h1>
            <p className="text-sm text-gray-500">
              Sumber kebenaran tarif BPJS, PTKP, dan bracket PPh21 — dipakai
              langsung oleh kalkulasi payroll. Kosongkan kolom untuk memakai
              nilai default.
            </p>
          </div>
        </div>
        <Button
          onClick={handleSave}
          disabled={saveMutation.isPending || isLoading}
          className="bg-pink-600 hover:bg-pink-700"
        >
          {saveMutation.isPending ? "Menyimpan..." : "Simpan Pengaturan"}
        </Button>
      </div>

      {/* Cut-off & closing periode absensi: rentang absensi yang dibaca payroll. */}
      <AttendancePeriodSettingsCard />

      {isLoading ? (
        <Card>
          <CardContent className="py-10 text-center text-gray-500">
            Memuat pengaturan…
          </CardContent>
        </Card>
      ) : (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Komponen yang Dihitung</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {COMPONENT_SWITCHES.map((sw) => (
                <div key={sw.key}>
                  <label className="flex cursor-pointer items-start gap-2">
                    <input
                      type="checkbox"
                      className="mt-1 h-4 w-4 shrink-0 rounded border-gray-300"
                      checked={isSwitchOn(settingsForm, sw.key)}
                      onChange={(e) => handleSettingsChange(sw.key, e.target.checked ? "true" : "false")}
                    />
                    <span className="min-w-0 text-sm font-medium text-gray-700">{sw.label}</span>
                  </label>
                  {sw.key === "pph21_enabled" && (
                    <p className="ml-6 mt-1 text-xs text-gray-500">
                      Bila PPh 21 dinyalakan kembali di tengah tahun, perhitungan Desember (Pasal 17) menyesuaikan
                      pajak setahun penuh.
                    </p>
                  )}
                </div>
              ))}
              <p className="text-xs text-gray-500">
                Nonaktif: tidak dipotong dari gaji dan tidak ada iuran perusahaan. Berlaku untuk payroll yang dihitung
                setelah disimpan.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                BPJS Ketenagakerjaan
              </CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGrid
                fields={BPJS_TK_FIELDS}
                state={settingsForm}
                onChange={handleSettingsChange}
                disabled={!bpjsTkOn}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">BPJS Kesehatan</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGrid
                fields={BPJS_KES_FIELDS}
                state={settingsForm}
                onChange={handleSettingsChange}
                disabled={!bpjsKesOn}
                alwaysEnabled={["bpjs_kes_subsidy_amount"]}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Tapera</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGrid
                fields={TAPERA_FIELDS}
                state={settingsForm}
                onChange={handleSettingsChange}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">THR, Lembur & Gajian</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGrid
                fields={LAINNYA_FIELDS}
                state={settingsForm}
                onChange={handleSettingsChange}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Limitasi Pinjaman Karyawan</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGrid
                fields={LOAN_FIELDS}
                state={settingsForm}
                onChange={handleSettingsChange}
              />
              <p className="mt-3 text-xs text-gray-500">
                Divalidasi saat pengajuan dan approval pinjaman. Cicilan
                pinjaman approved otomatis terpotong di payroll mulai bulan
                setelah approval; saldo berkurang saat run ditandai dibayar.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Potongan Keterlambatan</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Mode Potongan
                  </label>
                  <Select
                    value={settingsForm.late_deduction_mode || "off"}
                    onValueChange={(value) =>
                      handleSettingsChange("late_deduction_mode", value)
                    }
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Pilih mode" />
                    </SelectTrigger>
                    <SelectContent>
                      {LATE_MODE_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Tarif Potongan{" "}
                    <span className="text-gray-400">
                      (Rp — per menit / per kejadian sesuai mode)
                    </span>
                  </label>
                  <Input
                    type="number"
                    step="any"
                    value={settingsForm.late_deduction_amount ?? ""}
                    onChange={(e) =>
                      handleSettingsChange("late_deduction_amount", e.target.value)
                    }
                  />
                </div>
              </div>
              {settingsForm.late_deduction_mode === "tier" && (
                <>
                  <div className="mt-3 max-w-xs">
                    <label className="mb-1 block text-xs font-medium text-gray-600">
                      Telat dihitung bila minimal <span className="text-gray-400">(menit)</span>
                    </label>
                    <Input
                      type="number"
                      min={0}
                      value={settingsForm.late_min_minutes ?? ""}
                      placeholder="0"
                      onChange={(e) => handleSettingsChange("late_min_minutes", e.target.value)}
                    />
                  </div>
                  <LateTierEditor
                    value={settingsForm.late_tier_rules ?? ""}
                    onChange={(json) => handleSettingsChange("late_tier_rules", json)}
                  />
                </>
              )}
              <p className="mt-3 text-xs text-gray-500">
                Menit keterlambatan diambil dari absensi v2 (dihitung terhadap
                shift saat clock-in). Mode nonaktif = tidak ada potongan. Mode bertingkat
                menghitung jumlah hari telat sebulan; tarif di atas tidak dipakai. Potongan
                manual (komponen &quot;Override Potongan Telat&quot; di run) menggantikan hitungan otomatis.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">PPh 21 — Metode</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div>
                  <label htmlFor="pph21_method" className="mb-1 block text-xs font-medium text-gray-600">Metode pemotongan</label>
                  <select
                    id="pph21_method"
                    disabled={!pphOn}
                    value={settingsForm.pph21_method || "annualized"}
                    onChange={(e) => handleSettingsChange("pph21_method", e.target.value)}
                    className="w-full rounded-md border border-gray-200 px-3 py-2 text-sm disabled:bg-gray-50 disabled:opacity-50"
                  >
                    {PPH21_METHOD_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label htmlFor="pph21_bpjs_basis" className="mb-1 block text-xs font-medium text-gray-600">BPJS yang ditambahkan ke dasar TER</label>
                  <select
                    id="pph21_bpjs_basis"
                    value={settingsForm.pph21_bpjs_basis || "both"}
                    onChange={(e) => handleSettingsChange("pph21_bpjs_basis", e.target.value)}
                    className="w-full rounded-md border border-gray-200 px-3 py-2 text-sm disabled:bg-gray-50 disabled:opacity-50"
                    disabled={!pphOn || (settingsForm.pph21_method || "annualized") !== "ter"}
                  >
                    {PPH21_BPJS_BASIS_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              <p className="mt-3 text-xs text-gray-500">
                TER: Januari–November PPh 21 = bruto bulan itu × tarif TER (kategori A/B/C dari PTKP). Desember:
                PPh 21 setahun (Pasal 17) dikurangi yang sudah dipotong Januari–November di entitas yang sama.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Lembur & Potongan Izin</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {TOGGLE_FIELDS.map((t) => {
                const fallback = t.key === "overtime_requires_clockout" ? "true" : "false";
                const checked = (settingsForm[t.key] || fallback) === "true";
                return (
                  <label key={t.key} className="flex cursor-pointer items-start gap-2">
                    <input
                      type="checkbox"
                      className="mt-1 h-4 w-4 rounded border-gray-300"
                      checked={checked}
                      onChange={(e) => handleSettingsChange(t.key, e.target.checked ? "true" : "false")}
                    />
                    <span>
                      <span className="block text-sm font-medium text-gray-700">{t.label}</span>
                      <span className="block text-xs text-gray-500">{t.help}</span>
                    </span>
                  </label>
                );
              })}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle className="text-base">
                PPh 21 — PTKP & Bracket (per Tahun Pajak)
              </CardTitle>
              <div className="flex items-center gap-2">
                <label className="text-xs font-medium text-gray-600">
                  Tahun Pajak
                </label>
                <Input
                  type="number"
                  className="w-24"
                  value={taxYear}
                  onChange={(e) => {
                    const y = Number(e.target.value);
                    if (Number.isInteger(y) && y >= 2000 && y <= 2100) {
                      setTaxYear(y);
                      setTaxEdits({});
                    }
                  }}
                />
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">
                  PTKP Tahunan
                </p>
                <FieldGrid
                  fields={PTKP_FIELDS}
                  state={taxForm}
                  onChange={handleTaxChange}
                  disabled={!pphOn}
                />
              </div>
              <div>
                <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">
                  Bracket Progresif (batas kumulatif) & Biaya Jabatan
                </p>
                <FieldGrid
                  fields={BRACKET_FIELDS}
                  state={taxForm}
                  onChange={handleTaxChange}
                  disabled={!pphOn}
                />
              </div>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}

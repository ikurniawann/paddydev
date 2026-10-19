"use client";

import type { ScheduleGrid as Grid } from "../types";

/** Grid read-only karyawan × tanggal untuk satu cabang. */
export function ScheduleGrid({ grid }: { grid: Grid }) {
  const offDay = (d: Grid["dates"][number]) => d.holiday !== null || new Date(`${d.date}T00:00:00Z`).getUTCDay() === 0;
  return (
    <div className="space-y-3">
      <div className="overflow-x-auto rounded-xl border border-gray-200/70 bg-white">
        <table className="w-full text-xs">
          <thead>
            <tr className="bg-gray-50 text-gray-500">
              <th className="sticky left-0 z-10 bg-gray-50 px-3 py-2 text-left">Karyawan</th>
              {grid.dates.map((d) => (
                <th
                  key={d.date}
                  title={d.holiday ?? undefined}
                  className={`whitespace-nowrap px-2 py-2 text-center font-medium ${offDay(d) ? "bg-danger-soft text-danger" : ""}`}
                >
                  {d.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {grid.employees.map((e) => (
              <tr key={e.id} className="border-t border-gray-100">
                <td className="sticky left-0 z-10 bg-white px-3 py-1.5">
                  <p className="font-medium text-gray-800">{e.name}</p>
                  <p className="text-[11px] text-gray-400">
                    {e.nip}
                    {e.position ? ` · ${e.position}` : ""}
                  </p>
                </td>
                {e.cells.map((c) => (
                  <td
                    key={c.date}
                    title={c.code === "" ? "Belum ada jadwal" : c.source === "weekly" ? "Dari pola mingguan" : undefined}
                    className={`px-2 py-1.5 text-center font-mono ${
                      c.code === "L" ? "bg-gray-100 text-gray-500" : c.code === "-" ? "bg-gray-50 text-gray-300" : ""
                    } ${c.source === "weekly" ? "underline decoration-dotted" : ""}`}
                  >
                    {c.code === "" ? <span className="text-danger">—</span> : c.code}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-gray-500">
        Kode: {grid.shifts.map((s) => `${s.code} ${s.start_time.slice(0, 5)}–${s.end_time.slice(0, 5)}`).join(" · ")} · L = Libur ·
        - = tidak ditempatkan · garis titik = dari pola mingguan
      </p>
    </div>
  );
}

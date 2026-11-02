"use client";

import { Popover as PopoverPrimitive } from "@base-ui/react/popover";
import { useState } from "react";
import { canFollow, joinShiftCodes, shiftTone } from "../schedule-cells";
import type { ScheduleShift } from "../types";

export const hhmm = (t: string) => t.slice(0, 5);

interface Props {
  /** Elemen tempat pemilih menempel; null = tertutup. */
  anchor: Element | null;
  title: string;
  shifts: ScheduleShift[];
  onPick: (code: string) => void;
  onClose: () => void;
}

function Option({
  code,
  name,
  detail,
  onClick,
  disabled = false,
}: {
  code: string;
  name: string;
  detail?: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  const label = code ? `${code} · ${name}` : name;
  return (
    <button
      type="button"
      aria-label={detail ? `${label} ${detail}` : label}
      onClick={onClick}
      disabled={disabled}
      className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-gray-100 focus-visible:bg-gray-100 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
    >
      <span className={`inline-flex min-w-9 justify-center rounded px-1.5 py-0.5 font-mono text-xs font-semibold ${shiftTone(code)}`}>
        {code || "—"}
      </span>
      <span className="flex-1 text-gray-800">{name}</span>
      {detail ? <span className="text-xs text-gray-500">{detail}</span> : null}
    </button>
  );
}

/**
 * Pemilih shift satu kotak / banyak kotak: kode shift, Libur, atau kosongkan.
 * Setelah shift dipilih bisa menambah shift kedua (EPIC-080): kode "KODE1+KODE2",
 * shift kedua harus mulai setelah shift pertama selesai.
 */
export function ShiftPicker({ anchor, title, shifts, onPick, onClose }: Props) {
  /** Kode shift pertama bila sedang memilih shift kedua. */
  const [firstCode, setFirstCode] = useState<string | null>(null);
  const first = firstCode ? shifts.find((s) => s.code === firstCode) : undefined;

  function close() {
    setFirstCode(null);
    onClose();
  }

  function pick(code: string) {
    setFirstCode(null);
    onPick(code);
  }

  return (
    <PopoverPrimitive.Root open={!!anchor} onOpenChange={(open) => (!open ? close() : undefined)}>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Positioner anchor={anchor} side="bottom" align="start" sideOffset={4} className="isolate z-[9999]">
          <PopoverPrimitive.Popup className="max-h-80 w-72 overflow-y-auto rounded-2xl border border-border bg-popover p-2 text-sm text-popover-foreground shadow-float outline-hidden">
            {first ? (
              <>
                <p className="px-2 pb-1.5 pt-1 text-xs font-semibold text-gray-500">{`Shift kedua setelah ${first.code} (${hhmm(first.start_time)}–${hhmm(first.end_time)})`}</p>
                {shifts.map((s) => (
                  <Option
                    key={s.code}
                    code={s.code}
                    name={s.name}
                    detail={`${hhmm(s.start_time)}–${hhmm(s.end_time)}`}
                    disabled={!canFollow(first, s)}
                    onClick={() => pick(joinShiftCodes(first.code, s.code))}
                  />
                ))}
                <div className="my-1 border-t border-gray-100" />
                <button
                  type="button"
                  onClick={() => setFirstCode(null)}
                  className="w-full rounded-lg px-2 py-1.5 text-left text-xs text-gray-600 hover:bg-gray-100 focus-visible:bg-gray-100 focus-visible:outline-none"
                >
                  Kembali
                </button>
              </>
            ) : (
              <>
                <p className="px-2 pb-1.5 pt-1 text-xs font-semibold text-gray-500">{title}</p>
                {shifts.map((s) => (
                  <div key={s.code} className="flex items-center gap-1">
                    <div className="min-w-0 flex-1">
                      <Option
                        code={s.code}
                        name={s.name}
                        detail={`${hhmm(s.start_time)}–${hhmm(s.end_time)}`}
                        onClick={() => pick(s.code)}
                      />
                    </div>
                    <button
                      type="button"
                      aria-label={`Tambah shift kedua setelah ${s.code}`}
                      title="Tambah shift kedua"
                      onClick={() => setFirstCode(s.code)}
                      className="shrink-0 rounded-md border border-gray-200 px-1.5 py-1 text-[11px] font-medium text-gray-600 hover:bg-gray-100 focus-visible:bg-gray-100 focus-visible:outline-none"
                    >
                      + Shift 2
                    </button>
                  </div>
                ))}
                <Option code="L" name="Libur" onClick={() => pick("L")} />
                <div className="my-1 border-t border-gray-100" />
                <Option code="" name="Kosongkan (ikut pola mingguan)" onClick={() => pick("")} />
              </>
            )}
          </PopoverPrimitive.Popup>
        </PopoverPrimitive.Positioner>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}

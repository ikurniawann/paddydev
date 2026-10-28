"use client";

import { Popover as PopoverPrimitive } from "@base-ui/react/popover";
import { shiftTone } from "../schedule-cells";
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

function Option({ code, name, detail, onClick }: { code: string; name: string; detail?: string; onClick: () => void }) {
  const label = code ? `${code} · ${name}` : name;
  return (
    <button
      type="button"
      aria-label={detail ? `${label} ${detail}` : label}
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-gray-100 focus-visible:bg-gray-100 focus-visible:outline-none"
    >
      <span className={`inline-flex min-w-9 justify-center rounded px-1.5 py-0.5 font-mono text-xs font-semibold ${shiftTone(code)}`}>
        {code || "—"}
      </span>
      <span className="flex-1 text-gray-800">{name}</span>
      {detail ? <span className="text-xs text-gray-500">{detail}</span> : null}
    </button>
  );
}

/** Pemilih shift satu kotak / banyak kotak: kode shift, Libur, atau kosongkan. */
export function ShiftPicker({ anchor, title, shifts, onPick, onClose }: Props) {
  return (
    <PopoverPrimitive.Root open={!!anchor} onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Positioner anchor={anchor} side="bottom" align="start" sideOffset={4} className="isolate z-[9999]">
          <PopoverPrimitive.Popup className="max-h-80 w-72 overflow-y-auto rounded-2xl border border-border bg-popover p-2 text-sm text-popover-foreground shadow-float outline-hidden">
            <p className="px-2 pb-1.5 pt-1 text-xs font-semibold text-gray-500">{title}</p>
            {shifts.map((s) => (
              <Option
                key={s.code}
                code={s.code}
                name={s.name}
                detail={`${hhmm(s.start_time)}–${hhmm(s.end_time)}`}
                onClick={() => onPick(s.code)}
              />
            ))}
            <Option code="L" name="Libur" onClick={() => onPick("L")} />
            <div className="my-1 border-t border-gray-100" />
            <Option code="" name="Kosongkan (ikut pola mingguan)" onClick={() => onPick("")} />
          </PopoverPrimitive.Popup>
        </PopoverPrimitive.Positioner>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}

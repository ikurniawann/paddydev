"use client";

import * as React from "react";
import { Check, ChevronsUpDown, Search } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

export interface ComboboxOption {
  value: string;
  label: string;
  description?: string;
  style?: React.CSSProperties;
}

export interface ComboboxProps {
  options: ComboboxOption[];
  value?: string;
  onChange: (value: string) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
  disabled?: boolean;
  className?: string;
  /** Extra classes for the dropdown panel (e.g. min-w for long labels). */
  contentClassName?: string;
  allowClear?: boolean;
  /**
   * Pencarian dilakukan pemanggil (mis. ke server): teks yang diketik dilaporkan
   * di sini dan `options` dipakai apa adanya tanpa disaring lagi di sini.
   */
  onSearchChange?: (search: string) => void;
}

export function Combobox({
  options,
  value,
  onChange,
  placeholder = "Pilih opsi...",
  searchPlaceholder = "Cari...",
  emptyMessage = "Tidak ditemukan.",
  disabled = false,
  className,
  contentClassName,
  allowClear = false,
  onSearchChange,
}: ComboboxProps) {
  const [open, setOpen] = React.useState(false);
  const [searchValue, setSearchValue] = React.useState("");

  const selectedOption = React.useMemo(
    () => options.find((opt) => opt.value === value),
    [options, value]
  );

  const filteredOptions = React.useMemo(() => {
    if (!searchValue || onSearchChange) return options;
    const search = searchValue.toLowerCase();
    return options.filter(
      (opt) =>
        opt.label.toLowerCase().includes(search) ||
        opt.value.toLowerCase().includes(search) ||
        opt.description?.toLowerCase().includes(search)
    );
  }, [options, searchValue, onSearchChange]);

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setSearchValue("");
          onSearchChange?.("");
        }
      }}
    >
      <PopoverTrigger
        data-slot="select-trigger"
        type="button"
        role="combobox"
        aria-expanded={open}
        className={cn(
          "flex h-9 w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-gray-200/80 bg-transparent px-3 text-sm outline-none transition-colors hover:border-gray-300 hover:bg-muted focus-visible:border-pink-200 focus-visible:ring-1 focus-visible:ring-pink-100 disabled:pointer-events-none disabled:opacity-50",
          className
        )}
        disabled={disabled}
      >
        <span
          className={cn("min-w-0 truncate", !selectedOption && "text-muted-foreground")}
          style={selectedOption?.style}
        >
          {selectedOption ? selectedOption.label : placeholder}
        </span>
        <div className="flex items-center gap-2">
          {allowClear && value && (
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation();
                onChange("");
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  e.stopPropagation();
                  onChange("");
                }
              }}
              className="text-muted-foreground hover:text-foreground"
            >
              ×
            </span>
          )}
          <ChevronsUpDown className="h-4 w-4 shrink-0 opacity-50" />
        </div>
      </PopoverTrigger>
      <PopoverContent
        className={cn(
          "z-[9999] w-auto min-w-(--anchor-width) max-w-[min(28rem,calc(100vw-2rem))] border border-gray-200/80 bg-white p-0 shadow-xl ring-1 ring-gray-200/60",
          contentClassName
        )}
        align="start"
        sideOffset={4}
      >
        <Command shouldFilter={false} className="bg-white">
          <div className="flex items-center border-b border-gray-200 px-3 py-2.5">
            <Search className="mr-2 h-4 w-4 shrink-0 text-gray-400" />
            <input
              data-combobox-search="true"
              value={searchValue}
              onChange={(e) => {
                setSearchValue(e.target.value);
                onSearchChange?.(e.target.value);
              }}
              placeholder={searchPlaceholder}
              className="w-full border-0! bg-transparent text-sm text-gray-900 shadow-none! outline-none! ring-0! placeholder:text-gray-400 focus:border-0! focus:shadow-none! focus:outline-none! focus:ring-0!"
              style={{ border: 0, boxShadow: "none", outline: "none" }}
            />
          </div>
          <CommandList className="max-h-75 overflow-y-auto bg-white p-1">
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            <CommandGroup>
              {filteredOptions.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  onSelect={() => {
                    onChange(allowClear && option.value === value ? "" : option.value);
                    setOpen(false);
                    setSearchValue("");
                    onSearchChange?.("");
                  }}
                  className="hover:bg-gray-100 data-[selected=true]:bg-gray-200 cursor-pointer py-2 px-3 bg-white rounded-sm items-start"
                >
                  <span
                    className="min-w-0 flex-1 wrap-break-word whitespace-normal leading-snug"
                    style={option.style}
                  >
                    {option.label}
                  </span>
                  {option.description && (
                    <span className="ml-2 shrink-0 pt-0.5 text-xs text-muted-foreground">
                      {option.description}
                    </span>
                  )}
                  <Check
                    className={cn(
                      "mt-0.5 ml-auto h-4 w-4 shrink-0",
                      value === option.value ? "opacity-100" : "opacity-0"
                    )}
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

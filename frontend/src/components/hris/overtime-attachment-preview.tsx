"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight, Download, FileText } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { overtimeAttachmentHref } from "@/lib/hris/overtime-rules";
import { cn } from "@/lib/utils";

const isPdf = (path: string) => path.toLowerCase().endsWith(".pdf");

/**
 * Dokumentasi bukti lembur (EPIC-066): tombol "Dokumentasi n" membuka modal
 * pratinjau — gambar tampil langsung, PDF di dalam bingkai — tanpa tab baru.
 */
export function OvertimeAttachmentButtons({
  paths,
  className,
  buttonClassName = "text-xs font-medium text-pink-600 hover:underline",
}: {
  paths: string[];
  className?: string;
  buttonClassName?: string;
}) {
  const [index, setIndex] = useState<number | null>(null);
  if (paths.length === 0) return null;
  const current = index === null ? null : paths[index];
  const label = index === null ? "" : `Dokumentasi ${index + 1}`;

  return (
    <>
      <span className={cn("flex flex-wrap gap-x-3 gap-y-1", className)}>
        {paths.map((path, i) => (
          <button key={path} type="button" className={buttonClassName} onClick={() => setIndex(i)}>
            Dokumentasi {i + 1}
          </button>
        ))}
      </span>
      <Dialog open={current !== null} onOpenChange={(open) => !open && setIndex(null)}>
        <DialogContent className="flex max-h-[92vh] flex-col gap-3 p-4 sm:max-w-4xl">
          <DialogHeader className="pr-8">
            <DialogTitle className="flex items-center gap-2 text-base">
              <FileText className="h-5 w-5 text-gray-500" />
              {label}
              {paths.length > 1 && (
                <span className="text-xs font-normal text-gray-500">
                  {(index ?? 0) + 1} / {paths.length}
                </span>
              )}
            </DialogTitle>
          </DialogHeader>
          {current && (
            <div className="min-h-[50vh] flex-1 overflow-hidden rounded-lg bg-gray-50">
              {isPdf(current) ? (
                <iframe src={overtimeAttachmentHref(current)} title={label} className="h-[72vh] w-full bg-white" />
              ) : (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={overtimeAttachmentHref(current)}
                  alt={label}
                  className="mx-auto max-h-[72vh] w-auto max-w-full object-contain"
                />
              )}
            </div>
          )}
          <div className="flex items-center justify-between gap-2">
            <div className="flex gap-2">
              {paths.length > 1 && (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    aria-label="Sebelumnya"
                    disabled={index === 0}
                    onClick={() => setIndex((i) => (i === null ? i : Math.max(0, i - 1)))}
                  >
                    <ChevronLeft className="h-4 w-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    aria-label="Berikutnya"
                    disabled={index === paths.length - 1}
                    onClick={() => setIndex((i) => (i === null ? i : Math.min(paths.length - 1, i + 1)))}
                  >
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </>
              )}
            </div>
            {current && (
              <a href={overtimeAttachmentHref(current)} download className={cn(buttonVariants({ size: "sm" }))}>
                <Download className="mr-2 h-4 w-4" />
                Unduh
              </a>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

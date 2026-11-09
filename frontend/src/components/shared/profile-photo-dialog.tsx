"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { essQueryKeys } from "@/features/hris/ess/query-keys";
import { deleteProfilePhoto, uploadProfilePhoto, validateProfilePhoto } from "@/lib/profile-photo";
import { UserAvatar } from "./user-avatar";

/** Ubah foto profil dari popup akun (EPIC-087 R6). */
export function ProfilePhotoDialog({
  open,
  onOpenChange,
  name,
  currentUrl,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  name: string;
  currentUrl: string | null;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [busy, setBusy] = useState(false);
  // Setiap kali dialog ditutup, sesi bertambah: pratinjau dari pembacaan file
  // sesi sebelumnya (selesai setelah ditutup) tidak ditampilkan.
  const [session, setSession] = useState(0);
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (!open) setSession(session + 1);
  }
  const [picked, setPicked] = useState<{ session: number; url: string } | null>(null);
  const preview = picked && picked.session === session ? picked.url : null;

  function close(next: boolean) {
    onOpenChange(next);
  }

  function handleFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    const problem = validateProfilePhoto(file);
    if (problem) {
      toast.error(problem);
      return;
    }
    const reader = new FileReader();
    const started = session;
    reader.onloadend = () => setPicked({ session: started, url: reader.result as string });
    reader.readAsDataURL(file);
  }

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await action();
      toast.success(done);
      router.refresh(); // top bar, popup, drawer (render server)
      void queryClient.invalidateQueries({ queryKey: essQueryKeys.beranda() }); // Beranda ESS
      close(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menyimpan foto profil");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogPanel size="xs">
        <DialogPanelHeader>
          <DialogPanelTitle>Ubah foto profil</DialogPanelTitle>
          <DialogPanelDescription>JPG, PNG, atau WebP, maksimal 5 MB. Foto dipotong persegi otomatis.</DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody className="flex flex-col items-center gap-4">
          {preview ? (
            // eslint-disable-next-line @next/next/no-img-element -- pratinjau data URL lokal
            <img src={preview} alt="Pratinjau foto profil" className="size-28 rounded-full object-cover" />
          ) : (
            <UserAvatar name={name} src={currentUrl} size={112} className="bg-accent text-3xl font-bold text-accent-foreground" />
          )}
          <label className="cursor-pointer rounded-full border border-border px-4 py-2 text-sm font-medium hover:bg-surface">
            Pilih foto
            <input type="file" accept="image/jpeg,image/png,image/webp" className="sr-only" onChange={handleFile} aria-label="Pilih foto" />
          </label>
        </DialogPanelBody>
        <DialogFooter className="gap-2">
          {currentUrl && !preview && (
            <Button type="button" variant="outline" disabled={busy} onClick={() => run(deleteProfilePhoto, "Foto profil dihapus")}>
              Hapus foto
            </Button>
          )}
          <Button type="button" disabled={!preview || busy} onClick={() => preview && run(() => uploadProfilePhoto(preview), "Foto profil diperbarui")}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Simpan"}
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}

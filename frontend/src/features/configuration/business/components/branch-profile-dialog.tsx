"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { LocationPickerField } from "@/components/maps/location-picker-field";
import { Button } from "@/components/ui/button";
import { Dialog, DialogFooter, DialogPanel, DialogPanelBody, DialogPanelDescription, DialogPanelForm, DialogPanelHeader, DialogPanelTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { ImageUrlField, ItemListField, LabeledField, LinesField, LongText } from "@/features/site/admin/fields";
import { parseLocationText, type LatLngPoint } from "@/lib/maps/coords";
import { fetchBranchProfile, saveBranchProfile, type BranchProfile } from "../api";

type Accordions = BranchProfile["accordions"];

const ACCORDIONS: { key: keyof Accordions; label: string }[] = [
  { key: "facilities", label: "Fasilitas" },
  { key: "parking", label: "Parkir & akses" },
  { key: "team", label: "Tim coach" },
  { key: "community", label: "Komunitas" },
];

const str = (v: string | null) => v ?? "";
const orNull = (v: string) => v.trim() || null;
const orNumber = (v: string) => (v.trim() === "" ? null : Number(v));

interface Form {
  slug: string;
  is_public: boolean;
  address: string;
  city: string;
  postcode: string;
  lat: string;
  lng: string;
  phone: string;
  email: string;
  instagram: string;
  directions: string;
  hero_image_url: string;
  benefits: BranchProfile["benefits"];
  accordions: Accordions;
  extras: BranchProfile["extras"];
  testimonials: BranchProfile["testimonials"];
}

function toForm(p: BranchProfile): Form {
  return {
    slug: p.slug,
    is_public: p.is_public,
    address: str(p.address),
    city: str(p.city),
    postcode: str(p.postcode),
    lat: p.lat === null ? "" : String(p.lat),
    lng: p.lng === null ? "" : String(p.lng),
    phone: str(p.phone),
    email: str(p.email),
    instagram: str(p.instagram),
    directions: str(p.directions),
    hero_image_url: str(p.hero_image_url),
    benefits: p.benefits,
    accordions: p.accordions,
    extras: p.extras,
    testimonials: p.testimonials,
  };
}

function toBody(f: Form) {
  return {
    slug: f.slug.trim(),
    is_public: f.is_public,
    address: orNull(f.address),
    city: orNull(f.city),
    postcode: orNull(f.postcode),
    lat: orNumber(f.lat),
    lng: orNumber(f.lng),
    phone: orNull(f.phone),
    email: orNull(f.email),
    instagram: orNull(f.instagram),
    directions: orNull(f.directions),
    hero_image_url: orNull(f.hero_image_url),
    benefits: f.benefits,
    accordions: f.accordions,
    extras: f.extras,
    testimonials: f.testimonials,
  };
}

/** Settings → Business → branch row → "Profil publik": what /locations/[slug] shows. */
export function BranchProfileDialog({ branchId, branchName, onClose }: { branchId: string | null; branchName: string; onClose(): void }) {
  const profile = useQuery({
    queryKey: ["business", "branch-profile", branchId],
    queryFn: () => fetchBranchProfile(branchId!),
    enabled: branchId !== null,
  });

  return (
    <Dialog open={branchId !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogPanel size="lg">
        {branchId && profile.data ? (
          <ProfileForm key={branchId} branchId={branchId} branchName={branchName} initial={profile.data} onClose={onClose} />
        ) : (
          <DialogPanelBody>
            <p className="py-8 text-center text-sm text-muted-foreground">{profile.error ? profile.error.message : "Memuat profil…"}</p>
          </DialogPanelBody>
        )}
      </DialogPanel>
    </Dialog>
  );
}

function ProfileForm({ branchId, branchName, initial, onClose }: { branchId: string; branchName: string; initial: BranchProfile; onClose(): void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<Form>(() => toForm(initial));
  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }));
  const setPoint = (p: LatLngPoint) => setForm((f) => ({ ...f, lat: String(p.lat), lng: String(p.lng) }));
  // "-6.2, 106.8" yang disalin dari Google Maps: isi lat dan lng sekaligus (input angka menolak teks itu).
  const pasteCoords = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const p = parseLocationText(e.clipboardData.getData("text"));
    if (!p) return;
    e.preventDefault();
    setPoint(p);
  };

  const save = useMutation({
    mutationFn: (f: Form) => saveBranchProfile(branchId, toBody(f)),
    onSuccess: (p) => {
      queryClient.setQueryData(["business", "branch-profile", branchId], p);
      toast.success("Profil cabang disimpan", { description: p.is_public ? `Tampil di /locations/${p.slug}` : "Belum dipublikasikan" });
      onClose();
    },
    onError: (error) => toast.error("Profil gagal disimpan", { description: error.message }),
  });

  return (
    <DialogPanelForm
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(form);
      }}
    >
      <DialogPanelHeader>
        <DialogPanelTitle>Profil publik: {branchName}</DialogPanelTitle>
        <DialogPanelDescription>Halaman cabang di situs. Aktifkan &quot;Tampil di situs&quot; agar muncul di /locations.</DialogPanelDescription>
      </DialogPanelHeader>
      <DialogPanelBody className="space-y-4">
        <div className="flex items-center justify-between rounded-2xl bg-surface-2 px-4 py-3">
          <span className="text-sm font-medium">Tampil di situs</span>
          <Switch checked={form.is_public} onCheckedChange={(v) => set("is_public", v)} />
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <LabeledField label="Slug" hint="Alamat halaman: /locations/<slug>">
            <Input value={form.slug} onChange={(e) => set("slug", e.target.value)} className="font-mono" required pattern="[a-z0-9]+(-[a-z0-9]+)*" />
          </LabeledField>
          <LabeledField label="Kota">
            <Input value={form.city} onChange={(e) => set("city", e.target.value)} />
          </LabeledField>
          <LabeledField label="Alamat">
            <Input value={form.address} onChange={(e) => set("address", e.target.value)} />
          </LabeledField>
          <LabeledField label="Kode pos">
            <Input value={form.postcode} onChange={(e) => set("postcode", e.target.value)} />
          </LabeledField>
          <LabeledField label="Latitude">
            <Input type="number" step="any" min={-90} max={90} value={form.lat} onChange={(e) => set("lat", e.target.value)} onPaste={pasteCoords} />
          </LabeledField>
          <LabeledField label="Longitude">
            <Input type="number" step="any" min={-180} max={180} value={form.lng} onChange={(e) => set("lng", e.target.value)} onPaste={pasteCoords} />
          </LabeledField>
          <LabeledField label="Telepon">
            <Input value={form.phone} onChange={(e) => set("phone", e.target.value)} />
          </LabeledField>
          <LabeledField label="Email">
            <Input type="email" value={form.email} onChange={(e) => set("email", e.target.value)} />
          </LabeledField>
          <LabeledField label="Instagram">
            <Input value={form.instagram} onChange={(e) => set("instagram", e.target.value)} placeholder="@nuhabit.bandung" />
          </LabeledField>
        </div>
        {/* Bukan LabeledField: <label> meneruskan klik di peta ke kontrol pertama di dalamnya. */}
        <div className="flex min-w-0 flex-col">
          <span className="mb-1.5 text-sm font-medium text-foreground">Titik di peta</span>
          <LocationPickerField
            lat={form.lat}
            lng={form.lng}
            onPick={setPoint}
            note="Dipakai peta situs dan geofence absensi; lingkaran biru memakai radius bawaan (radius per toko diatur di HRIS)."
          />
        </div>
        <LabeledField label="Gambar hero">
          <ImageUrlField value={form.hero_image_url} onChange={(v) => set("hero_image_url", v)} />
        </LabeledField>
        <LabeledField label="Petunjuk arah">
          <LongText value={form.directions} onChange={(v) => set("directions", v)} rows={3} />
        </LabeledField>
        <LabeledField label="Keunggulan">
          <ItemListField
            fields={[{ key: "title", label: "Judul", kind: "text" }, { key: "text", label: "Teks", kind: "long" }]}
            items={form.benefits}
            onChange={(items) => set("benefits", items as BranchProfile["benefits"])}
            addLabel="Tambah keunggulan"
          />
        </LabeledField>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {ACCORDIONS.map(({ key, label }) => (
            <LabeledField key={key} label={label} hint="Satu baris per poin.">
              <LinesField value={form.accordions[key]} onChange={(lines) => set("accordions", { ...form.accordions, [key]: lines })} />
            </LabeledField>
          ))}
        </div>
        <LabeledField label="Layanan ekstra">
          <ItemListField
            fields={[{ key: "name", label: "Nama", kind: "text" }, { key: "blurb", label: "Keterangan", kind: "long" }]}
            items={form.extras}
            onChange={(items) => set("extras", items as BranchProfile["extras"])}
            addLabel="Tambah layanan"
          />
        </LabeledField>
        <LabeledField label="Testimoni">
          <ItemListField
            fields={[{ key: "name", label: "Nama", kind: "text" }, { key: "role", label: "Peran", kind: "text" }, { key: "quote", label: "Kutipan", kind: "long" }]}
            items={form.testimonials}
            onChange={(items) => set("testimonials", items as BranchProfile["testimonials"])}
            addLabel="Tambah testimoni"
          />
        </LabeledField>
      </DialogPanelBody>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose} disabled={save.isPending}>
          Batal
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? "Menyimpan…" : "Simpan"}
        </Button>
      </DialogFooter>
    </DialogPanelForm>
  );
}

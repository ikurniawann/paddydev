import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { AutoCheckoutPage } from "@/features/hris/auto-checkout";

// HRIS → Kepegawaian → Checkout otomatis (EPIC-080): HR mengatur aturan per jabatan dan lokasi.
// Menu yang sama dengan API (rute Go memakai awalan kepegawaian); pengatur toko memakai tab di Jadwal Toko.
export default async function HrisAutoCheckoutPage() {
  await requireIamPage(IAM.hrisKepegawaian);
  return <AutoCheckoutPage />;
}

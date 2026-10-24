import type { Metadata } from "next";
import { Toaster } from "sonner";
import "./globals.css";
import { brandName, brandOsName } from "@/lib/branding";
import "quill/dist/quill.snow.css";
import QueryProvider from "@/components/providers/query-provider";
import { ThemeProvider } from "@/components/providers/theme-provider";
import { ThemeScript } from "@/components/providers/theme-script";
import { ActivityLogProvider } from "@/contexts/ActivityLogContext";
import { ErrorBoundary } from "@/components/error-boundary";
import { StagingBanner } from "@/components/layout/staging-banner";

// Font remote dimuat lewat <link>: `@import url(...)` di globals.css dibuang
// bundler CSS Next, sehingga Roundo/Nunito tidak pernah termuat.
const FONT_STYLESHEETS = [
  "https://api.fontshare.com/v2/css?f[]=roundo@400,500,600,700&display=swap",
  "https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300..800;1,9..40,300..800&family=Figtree:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600;700&family=IBM+Plex+Sans:wght@400;500;600;700&family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600;700&family=Karla:wght@400;500;600;700&family=Lato:wght@400;700&family=Lora:wght@400;500;600;700&family=Manrope:wght@400;500;600;700&family=Merriweather:wght@400;700&family=Mulish:wght@400;500;600;700&family=Noto+Sans:wght@400;500;600;700&family=Nunito:wght@400;500;600;700;800;900&family=Open+Sans:wght@400;500;600;700&family=Outfit:wght@400;500;600;700&family=Playfair+Display:ital,wght@0,500;0,600;0,700;1,500;1,600&family=Plus+Jakarta+Sans:wght@400;500;600;700&family=Poppins:wght@400;500;600;700&family=Public+Sans:wght@400;500;600;700&family=Roboto:wght@400;500;700&family=Rubik:wght@400;500;600;700&family=Sora:wght@400;500;600;700&family=Source+Sans+3:wght@400;500;600;700&family=Source+Serif+4:wght@400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&family=Urbanist:wght@400;500;600;700&family=Work+Sans:wght@400;500;600;700&display=swap",
];

export const metadata: Metadata = {
  title: brandName(),
  description: `${brandOsName()} — ERP terintegrasi untuk operasional bisnis`,
  // Nama file "paddy-*" (bukan favicon-64.png lama) agar browser yang pernah
  // menyimpan favicon merek lama di domain ini memuat ulang ikon Paddy.
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "48x48 32x32 16x16" },
      { url: "/brand/paddy-favicon-32.png", sizes: "32x32", type: "image/png" },
      { url: "/brand/paddy-favicon-16.png", sizes: "16x16", type: "image/png" },
    ],
    shortcut: "/favicon.ico",
    apple: "/brand/paddy-apple-touch-icon.png",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    // ThemeScript mutates data-theme + brand CSS vars on <html> before React
    // hydrates; suppress the expected attribute mismatch (same pattern as next-themes).
    <html lang="id" suppressHydrationWarning>
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link rel="preconnect" href="https://api.fontshare.com" />
        <link rel="preconnect" href="https://cdn.fontshare.com" crossOrigin="anonymous" />
        {FONT_STYLESHEETS.map((href) => (
          <link key={href} rel="stylesheet" href={href} />
        ))}
        <ThemeScript />
      </head>
      <body suppressHydrationWarning>
        <StagingBanner productionUrl={process.env.STAGING_BANNER_PRODUCTION_URL} />
        <ErrorBoundary>
          <ThemeProvider>
            <QueryProvider>
              <ActivityLogProvider>{children}</ActivityLogProvider>
              {/* Satu-satunya Toaster aplikasi; halaman cukup memanggil toast() dari sonner. */}
              <Toaster position="bottom-right" />
            </QueryProvider>
          </ThemeProvider>
        </ErrorBoundary>
      </body>
    </html>
  );
}

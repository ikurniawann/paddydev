import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Emits .next/standalone with a self-contained server.js and only the
  // node_modules files actually traced as reachable — the runtime image then
  // needs no package manager and none of the devDependencies.
  // NOT output: "export": this app has ~580 route handlers plus rewrites()
  // below, none of which a static export supports.
  output: "standalone",
  reactCompiler: true,
  allowedDevOrigins: ["192.168.0.109", "192.168.18.29", "localhost", "192.168.11.234"],
  typescript: {
    ignoreBuildErrors: true,
  },
  // tesseract.js spawn worker dari file di node_modules — tidak boleh di-bundle;
  // pdfkit baca file font AFM dari node_modules saat runtime; sharp modul
  // native (libvips) — bila di-bundle, binary @img/sharp-libvips-* tidak
  // ikut ter-trace ke standalone dan runtime-nya gagal (insiden ekspor
  // absensi 2026-08-29: ketiga format 500 di production).
  serverExternalPackages: ["tesseract.js", "unpdf", "mammoth", "pdfkit", "sharp", "exceljs"],
  turbopack: {
    root: __dirname,
  },
  experimental: {
    // src/proxy.ts rewrites ported /api routes to the Go service. Next cuts
    // proxied requests at 30 s by default, which would drop the desktop and
    // AI assistant SSE streams and slow reports; EventSource reconnects
    // after the hour.
    proxyTimeout: 60 * 60 * 1000,
    // proxy.ts makes Next buffer every request body, and only the first
    // 10 MB by default; data room files go up to 100 MB (plus multipart
    // overhead), whether Next or the Go service handles the route.
    proxyClientMaxBodySize: "110mb",
  },
  async rewrites() {
    return {
      beforeFiles: [
        // ── Raw Material: master data ─────────────────────────────────────
        { source: "/dashboard/raw-material/units", destination: "/dashboard/items/units" },
        {
          source: "/dashboard/raw-material/categories",
          destination: "/dashboard/items/raw-material/categories",
        },
        { source: "/dashboard/raw-material/materials", destination: "/dashboard/items/raw-materials" },
        {
          source: "/dashboard/raw-material/materials/bom/:path*",
          destination: "/dashboard/items/raw-material/materials/bom/:path*",
        },
        {
          source: "/dashboard/raw-material/materials/:path*",
          destination: "/dashboard/items/raw-materials/:path*",
        },
        // ── Raw Material: inventory ─────────────────────────────────────────
        {
          source: "/dashboard/raw-material/inventory/stock",
          destination: "/dashboard/inventory/stock",
        },
        {
          source: "/dashboard/raw-material/inventory/opname",
          destination: "/dashboard/inventory/opname",
        },
        {
          source: "/dashboard/raw-material/inventory/opname/:path*",
          destination: "/dashboard/inventory/opname/:path*",
        },
        {
          source: "/dashboard/raw-material/inventory/adjustment",
          destination: "/dashboard/inventory/adjustment",
        },
        {
          source: "/dashboard/raw-material/inventory/adjustment/:path*",
          destination: "/dashboard/inventory/adjustment/:path*",
        },
        {
          source: "/dashboard/raw-material/inventory/transfers",
          destination: "/dashboard/inventory/transfers",
        },
        // ── Raw Material: purchasing invoice → Accounting Account Payable ───
        {
          source: "/dashboard/raw-material/purchasing/invoice/po/:path*",
          destination: "/dashboard/accounting/accounts-payable/po/:path*",
        },
        {
          source: "/dashboard/raw-material/purchasing/invoice",
          destination: "/dashboard/accounting/accounts-payable",
        },
        {
          source: "/dashboard/purchasing/vendor-payments",
          destination: "/dashboard/accounting/accounts-payable",
        },
        {
          source: "/dashboard/purchasing/invoice/po/:path*",
          destination: "/dashboard/accounting/accounts-payable/po/:path*",
        },
        {
          source: "/dashboard/raw-material/purchasing/:path*",
          destination: "/dashboard/purchasing/:path*",
        },
        // ── Raw Material: approval & production ─────────────────────────────
        {
          source: "/dashboard/raw-material/approval/:path*",
          destination: "/dashboard/purchasing/approval/:path*",
        },
        {
          source: "/dashboard/raw-material/production/:path*",
          destination: "/dashboard/items/raw-material/production/:path*",
        },
        // ── Product: master data ────────────────────────────────────────────
        {
          source: "/dashboard/product/units",
          destination: "/dashboard/items/product/units",
        },
        {
          source: "/dashboard/product/categories",
          destination: "/dashboard/items/product/categories",
        },
        { source: "/dashboard/product/products", destination: "/dashboard/items/products" },
        {
          source: "/dashboard/product/products/:path*",
          destination: "/dashboard/items/products/:path*",
        },
        // ── Product: inventory, purchasing, approval placeholders ───────────
        {
          source: "/dashboard/product/inventory/:path*",
          destination: "/dashboard/items/product/inventory/:path*",
        },
        {
          source: "/dashboard/product/purchasing/:path*",
          destination: "/dashboard/items/product/purchasing/:path*",
        },
        {
          source: "/dashboard/product/approval/:path*",
          destination: "/dashboard/items/product/approval/:path*",
        },
        // ── Product: production ─────────────────────────────────────────────
        {
          source: "/dashboard/product/production/:path*",
          destination: "/dashboard/items/product/production/:path*",
        },
      ],
    };
  },
  async redirects() {
    return [
      // Menu IAM aktif tanpa halaman fisik (cek 243 menu staging 30-10): jangan 404.
      // KPI dikelola di Konfigurasi KPI Department; Ecommerce membuka Pesanan.
      { source: "/dashboard/hris/kpi-templates", destination: "/dashboard/hris/kpi-config", permanent: false },
      { source: "/dashboard/shop", destination: "/dashboard/shop/orders", permanent: false },
      // EPIC-050 Fase 1: "Follow-up Hari Ini" → Tasks & Kalender (bookmark lama)
      {
        source: "/dashboard/sales-funnel/followups",
        destination: "/dashboard/sales-funnel/tasks",
        permanent: false,
      },
      {
        source: "/dashboard/finance/invoices",
        destination: "/dashboard/accounting/receivable/invoices-b2b",
        permanent: false,
      },
      {
        source: "/dashboard/finance/invoices/:path*",
        destination: "/dashboard/accounting/receivable/invoices-b2b",
        permanent: false,
      },
      {
        source: "/dashboard/product/purchasing/price-list/:path*",
        destination: "/dashboard/product/purchasing/vendor",
        permanent: false,
      },
      {
        source: "/dashboard/product/purchasing/price-list",
        destination: "/dashboard/product/purchasing/vendor",
        permanent: false,
      },
      {
        source: "/dashboard/items/product/purchasing/price-list/:path*",
        destination: "/dashboard/product/purchasing/vendor",
        permanent: false,
      },
      {
        source: "/dashboard/items/product/purchasing/price-list",
        destination: "/dashboard/product/purchasing/vendor",
        permanent: false,
      },
      // Raw Material legacy → canonical
      { source: "/dashboard/items/units", destination: "/dashboard/raw-material/units", permanent: false },
      {
        source: "/dashboard/items/raw-material/categories",
        destination: "/dashboard/raw-material/categories",
        permanent: false,
      },
      {
        source: "/dashboard/items/raw-materials",
        destination: "/dashboard/raw-material/materials",
        permanent: false,
      },
      {
        source: "/dashboard/items/raw-materials/:path*",
        destination: "/dashboard/raw-material/materials/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/inventory/stock",
        destination: "/dashboard/raw-material/inventory/stock",
        permanent: false,
      },
      {
        source: "/dashboard/inventory/opname",
        destination: "/dashboard/raw-material/inventory/opname",
        permanent: false,
      },
      {
        source: "/dashboard/inventory/opname/:path*",
        destination: "/dashboard/raw-material/inventory/opname/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/inventory/adjustment",
        destination: "/dashboard/raw-material/inventory/adjustment",
        permanent: false,
      },
      {
        source: "/dashboard/inventory/adjustment/:path*",
        destination: "/dashboard/raw-material/inventory/adjustment/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/inventory/transfers",
        destination: "/dashboard/raw-material/inventory/transfers",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/suppliers",
        destination: "/dashboard/raw-material/purchasing/suppliers",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/suppliers/:path*",
        destination: "/dashboard/raw-material/purchasing/suppliers/:path*",
        permanent: false,
      },
      { source: "/dashboard/purchasing/pr", destination: "/dashboard/raw-material/purchasing/pr", permanent: false },
      {
        source: "/dashboard/purchasing/pr/:path*",
        destination: "/dashboard/raw-material/purchasing/pr/:path*",
        permanent: false,
      },
      { source: "/dashboard/purchasing/po", destination: "/dashboard/raw-material/purchasing/po", permanent: false },
      {
        source: "/dashboard/purchasing/po/:path*",
        destination: "/dashboard/raw-material/purchasing/po/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/delivery",
        destination: "/dashboard/raw-material/purchasing/delivery",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/delivery/:path*",
        destination: "/dashboard/raw-material/purchasing/delivery/:path*",
        permanent: false,
      },
      { source: "/dashboard/purchasing/grn", destination: "/dashboard/raw-material/purchasing/grn", permanent: false },
      {
        source: "/dashboard/purchasing/grn/:path*",
        destination: "/dashboard/raw-material/purchasing/grn/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/returns",
        destination: "/dashboard/raw-material/purchasing/returns",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/returns/:path*",
        destination: "/dashboard/raw-material/purchasing/returns/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/approval/pr",
        destination: "/dashboard/raw-material/approval/pr",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/approval/po",
        destination: "/dashboard/raw-material/approval/po",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/production/recipes",
        destination: "/dashboard/raw-material/production/recipes",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/production",
        destination: "/dashboard/raw-material/production",
        permanent: false,
      },
      {
        source: "/dashboard/purchasing/production/:path*",
        destination: "/dashboard/raw-material/production/:path*",
        permanent: false,
      },
      // Product legacy → canonical
      {
        source: "/dashboard/items/product/units",
        destination: "/dashboard/product/units",
        permanent: false,
      },
      {
        source: "/dashboard/items/product/categories",
        destination: "/dashboard/product/categories",
        permanent: false,
      },
      { source: "/dashboard/items/products", destination: "/dashboard/product/products", permanent: false },
      {
        source: "/dashboard/items/products/:path*",
        destination: "/dashboard/product/products/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/items/product/inventory/:path*",
        destination: "/dashboard/product/inventory/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/items/product/purchasing/:path*",
        destination: "/dashboard/product/purchasing/:path*",
        permanent: false,
      },
      {
        source: "/dashboard/items/product/approval/:path*",
        destination: "/dashboard/product/approval/:path*",
        permanent: false,
      },
    ];
  },
  async headers() {
    // Security headers (audit 2026-09-17, temuan #7). Lima header di bawah
    // AMAN untuk langsung ditegakkan:
    //  - X-Frame-Options SAMEORIGIN: desktop Arkiv OS meng-iframe halaman
    //    /dashboard SE-ORIGIN, jadi SAMEORIGIN membiarkannya jalan (DENY akan
    //    membuat jendela desktop blank).
    //  - HSTS tanpa preload: komitmen preload dihindari, includeSubDomains
    //    aman karena semua subdomain (member.*) sudah HTTPS via Cloudflare.
    //  - Permissions-Policy: kamera & mikrofon = self (dipakai absensi,
    //    proctoring, interview); payment & usb dimatikan (tak dipakai).
    // CSP sengaja REPORT-ONLY dulu: app Next besar dengan skrip/inline style
    // bawaan — enforce langsung berisiko memutus halaman. Report-only memantau
    // pelanggaran (console) tanpa memblokir; ketatkan + tambah report endpoint
    // sebelum dipindah ke Content-Security-Policy yang menegakkan.
    // accounts.google.com: Google Identity Services (masuk dengan Google di
    // portal member) memuat skrip dan iframe tombolnya dari sana.
    const csp = [
      "default-src 'self'",
      "script-src 'self' 'unsafe-inline' 'unsafe-eval' https://accounts.google.com",
      "style-src 'self' 'unsafe-inline' https://accounts.google.com",
      "img-src 'self' data: blob: https:",
      "font-src 'self' data:",
      "connect-src 'self' https:",
      "frame-src 'self' https://accounts.google.com",
      "frame-ancestors 'self'",
      "base-uri 'self'",
      "form-action 'self'",
      "object-src 'none'",
    ].join("; ");
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Frame-Options", value: "SAMEORIGIN" },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" },
          {
            key: "Permissions-Policy",
            value: "camera=(self), microphone=(self), geolocation=(self), payment=(), usb=()",
          },
          { key: "Content-Security-Policy-Report-Only", value: csp },
        ],
      },
    ];
  },
};

export default nextConfig;

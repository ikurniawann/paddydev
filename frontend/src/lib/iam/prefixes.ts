/**
 * Prefix menu IAM untuk gate API/halaman.
 * Role baru yang di-grant menu di bawah prefix ini lolos tanpa ubah kode.
 */
export const IAM = {
  dashboard: ["dashboard"],
  reportSalesTarget: ["report.sales-vs-target"],
  ess: ["ess"],
  items: ["items"],
  itemsPurchasing: ["items.product.purchasing", "items.raw-material.purchasing"],
  itemsInventory: ["items.product.inventory", "items.raw-material.inventory"],
  itemsApproval: ["items.product.approval", "items.raw-material.approval"],
  itemsReports: ["items.reports"],
  /** Items → Pembelian → Pengaturan Pembelian (aturan PO wajib dari PR). */
  itemsPurchasingSettings: ["items.raw-material.purchasing.settings", "items.product.purchasing.settings"],
  pos: ["pos"],
  posOperations: ["pos.operations", "pos.cashier.central"],
  /** POS → Operasional → Tagihan (tagihan member, 2026-10-01). */
  posMemberBills: ["pos.operations.member-bills"],
  posCatalog: ["pos.catalog"],
  posKitchen: ["pos.kitchen"],
  posLoyalty: ["pos.loyalty"],
  posReports: ["pos.reports"],
  posSupervisors: ["pos.settings.supervisors"],
  hris: ["hris"],
  hrisKepegawaian: ["hris.kepegawaian"],
  /** HRIS → Kepegawaian → Karyawan: cukup untuk MEMBACA direktori karyawan (/api/users GET). */
  hrisKaryawan: ["hris.kepegawaian.users"],
  hrisWorkforce: ["hris.workforce"],
  /** HRIS → Workforce → Saldo Cuti (EPIC-068): daftar, ubah, impor saldo cuti tahunan. */
  hrisLeaveBalances: ["hris.workforce.leave-balances"],
  hrisCompensation: ["hris.compensation"],
  /** HRIS → Master Data → Penempatan Karyawan (karyawan multi-toko, 2026-10-15). */
  hrisPlacements: ["hris.master.placements"],
  hrisPerformance: ["hris.performance"],
  hrisRecruitment: ["hris.recruitment"],
  /** Pinjaman (Kepegawaian → Pinjaman); Penggajian tetap boleh seperti dulu. */
  hrisLoans: ["hris.kepegawaian.loans", "hris.compensation"],
  /** Analytics rekrutmen — membaca data kandidat. */
  hrisAnalytics: ["hris.insights.analytics"],
  /** HRIS → Hak Akses HRIS (EPIC-061): AdminHRIS mengatur akses role HR. */
  hrisAccess: ["hris.access"],
  accounting: ["accounting"],
  crm: ["crm"],
  crmSettings: ["crm.settings"],
  crmMembers: ["crm.members"],
  crmInbox: ["crm.members.inbox"],
  crmReviews: ["crm.members.reviews"],
  crmLoyalty: ["crm.loyalty"],
  crmReports: ["crm.reports"],
  crmPromo: ["crm.promo", "promo"],
  /** CRM → Engagement: pengumuman, event, challenge, check-in (port NüHabit). */
  crmEngagement: ["crm.engagement"],
  /** CRM → Customer Care → Ulasan Member (ulasan order dari portal). */
  crmMemberReviews: ["crm.members.member-reviews"],
  /** CRM → Loyalty → Partner Loyalty (event eksternal bertanda tangan). */
  crmPartners: ["crm.loyalty.partners"],
  settings: ["settings"],
  settingsUsers: ["settings.users"],
  settingsRoles: ["settings.roles"],
  settingsMenus: ["settings.menus"],
  settingsBusiness: ["settings.business"],
  settingsIntegrations: ["settings.integrations"],
  settingsAppearance: ["settings.appearance"],
  settingsPaymentGateways: ["settings.payment-gateways"],
  settingsWaGateway: ["settings.wa_gateway"],
  /** Settings → Audit Trail (jejak audit server-side). */
  settingsAudit: ["settings.audit"],
  shop: ["shop"],
  ticketing: ["ticketing"],
  ticketingAdmin: [
    "ticketing.settings",
    "ticketing.tickets",
    "ticketing.channel-manager",
    "ticketing.mapping",
  ],
  ticketingOperator: [
    "ticketing.loket",
    "ticketing.gate",
    "ticketing.booking",
    "ticketing.gate_pass",
    "ticketing.passes",
  ],
  ticketingReports: ["ticketing.reports"],
  salesFunnel: ["sales-funnel"],
  promo: ["promo", "crm.promo"],
  dataroom: ["dataroom"],
  resort: ["resort"],
  /** POS → Member → Dompet: paket top-up, koreksi saldo, pembayaran online, aturan saldo. */
  posWallet: ["pos.loyalty.wallet"],
  /** Gym → Stasiun & Latihan (pustaka HYROX + substitusi). */
  gymExercises: ["gym.exercises"],
  /** Gym → Race HYROX (kalender race + peserta). */
  gymRaces: ["gym.races"],
  /** Gym → Insentif Coach (skema, statement, payout). */
  gymIncentives: ["gym.incentives"],
  /** Gym → Paket Kredit (katalog paket kelas). */
  gymPackages: ["gym.packages"],
  /** Gym → Kredit Member (saldo, lot, koreksi, jual paket di front desk). */
  gymCredits: ["gym.credits"],
  /** Gym → Aturan Gym (global + override cabang). */
  gymRules: ["gym.rules"],
  /** Gym → jadwal, sesi & absensi, booking, jenis kelas, coach. */
  gymScheduling: ["gym.schedule", "gym.sessions", "gym.bookings", "gym.class-types", "gym.coaches"],
  /** Gym → Check-in (scan QR front desk; juga dibuka dari Sesi & Absensi). */
  gymCheckin: ["gym.checkin", "gym.sessions"],
  /** HRIS → Master Data (departemen, jabatan, status kepegawaian). */
  hrisMaster: ["hris.master"],
  /** HRIS → Organisasi (struktur organisasi, seksi). */
  hrisOrganization: ["hris.organization"],
  /** HRIS → Insights (analitik rekrutmen, laporan HR). */
  hrisInsights: ["hris.insights"],
  /**
   * Pengelola penilaian kinerja. Sengaja tanpa hris.performance.review dan
   * .dept-tasks yang juga di-grant ke role employee (ESS).
   */
  hrisPerformanceAdmin: ["hris.performance.kpi-scorecard", "hris.performance.kpi-config"],
  /** Tulis katalog produk POS (PATCH produk, SKU merchandise). */
  posCatalogProducts: ["pos.catalog.products"],
  /** Tulis pengaturan ARK & XP (kurs ARK, preset top-up). */
  posLoyaltySettings: ["pos.loyalty.settings"],
  /** Tulis profil Tax & Service (billing kasir). */
  settingsBilling: ["settings.billing"],
  /** Master produk/bahan & resep: halaman Items + POS katalog (picker produk, recipe-builder). */
  itemsCatalog: ["items", "pos.catalog"],
  /** Items → buat PR per modul (general, produk, bahan baku). */
  itemsPr: [
    "items.general.purchasing.pr",
    "items.product.purchasing.pr",
    "items.raw-material.purchasing.pr",
  ],
  /** Situs → Konten, Artikel, Event (site publik). */
  site: ["site"],
  siteContent: ["site.content"],
  siteArticles: ["site.articles"],
  siteEvents: ["site.events"],
  /** Spa (Paddy): booking, terapis, treatment, komisi, outlet. */
  spa: ["spa"],
  spaBookings: ["spa.bookings", "spa.book-order"],
  spaCustomer: ["spa.customer"],
  spaTreatments: ["spa.treatments"],
  spaCategories: ["spa.categories"],
  spaTherapists: ["spa.therapists"],
  spaCommissions: ["spa.commissions"],
  spaOutlets: ["spa.outlets"],
  spaRushHour: ["spa.rush-hour"],
  spaTrend: ["spa.trend"],
  spaRating: ["spa.rating"],
  /** Items → approval PR per modul. */
  itemsPrApproval: [
    "items.general.purchasing.approval-pr",
    "items.product.approval.pr",
    "items.raw-material.approval.pr",
  ],
} as const;

export type IamPrefixGroup = (typeof IAM)[keyof typeof IAM];

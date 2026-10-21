"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState, useSyncExternalStore } from "react";
import {
  Check,
  ChevronsUpDown,
  CircleUser,
  KeyRound,
  Loader2,
  LogOut,
  Monitor,
  MonitorSmartphone,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
  Store,
  Sun,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { ActivityLogBell } from "@/components/layout/ActivityLogBell";
import { NotificationBell } from "@/components/hris/NotificationBell";
import { useThemeOrNull } from "@/components/providers/theme-provider";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelForm,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  CanUseCentralCashierProvider,
  useConfirmAndSwitchStall,
} from "@/components/pos/confirm-stall-switch-dialog";
import { PosNfcShell } from "@/features/pos/nfc";
import { isPosImmersiveShell } from "@/features/pos/tablet-mode";
import { PosTabletManifestLink } from "@/features/pos/components/pos-tablet-manifest-link";
import { useMediaQuery } from "@/hooks/use-media-query";
import { cn } from "@/lib/utils";
import { brandOsName } from "@/lib/branding";
import type { NavItem } from "@/lib/iam/types";
import { isEssOnlyRole } from "@/lib/iam/access";
import { flattenNavLeaves } from "@/lib/iam/nav-leaves";
import AppSidebarNav from "./app-sidebar-nav";
import { DashboardBreadcrumbs } from "./dashboard-breadcrumbs";
import { GlobalSearch } from "./global-search";
import { PhoneNav } from "./phone-nav";
import Image from "next/image";
import { signOut } from "@/lib/auth/client";
import { stallLabel, type StallLabelSource } from "@/lib/auth/stall-label";

export interface SidebarUser {
  full_name: string;
  role: string;
  email?: string;
  company_name?: string | null;
  branch_id?: string | null;
  branch_name?: string | null;
  warehouse_name?: string | null;
  active_stall_id?: string | null;
  can_switch_stall?: boolean;
  can_central_checkout?: boolean;
  has_central_cashier_menu?: boolean;
}

export interface AppSidebarProps {
  user: SidebarUser;
  navItems: NavItem[];
  /** Kebijakan ESS-only hasil resolusi IAM di server; fallback ke role kode. */
  essOnly?: boolean;
  children: React.ReactNode;
}

const RAIL_PREF_KEY = "bcd.admin.rail";

/** The only decorative light on the canvas, mixed from the brand token. */
const CANVAS_GLOW = [
  "radial-gradient(38% 34% at 72% 22%, color-mix(in srgb, var(--brand-primary) 9%, transparent), transparent 70%)", // wit-allow: ambient canvas glow (layouts.md)
  "radial-gradient(28% 30% at 92% 48%, color-mix(in srgb, var(--brand-primary) 6%, transparent), transparent 70%)", // wit-allow: ambient canvas glow (layouts.md)
  "radial-gradient(34% 30% at 55% 8%, color-mix(in srgb, var(--brand-primary) 10%, white), transparent 72%)", // wit-allow: ambient canvas glow (layouts.md)
].join(", ");

const railListeners = new Set<() => void>();
// Fallback when storage is blocked: the choice then lasts for this visit.
let railMemory: boolean | null = null;

function readRailPref(): boolean {
  try {
    const saved = window.localStorage.getItem(RAIL_PREF_KEY);
    if (saved) return saved !== "collapsed";
  } catch {
    // Storage blocked: fall through to the in-memory choice.
  }
  return railMemory ?? true;
}

function subscribeRailPref(onChange: () => void) {
  railListeners.add(onChange);
  return () => {
    railListeners.delete(onChange);
  };
}

/** Expanded-rail preference (default expanded), persisted per browser. */
function useRailPreference() {
  const expanded = useSyncExternalStore(subscribeRailPref, readRailPref, () => true);
  const toggle = () => {
    railMemory = !expanded;
    try {
      window.localStorage.setItem(RAIL_PREF_KEY, expanded ? "collapsed" : "expanded");
    } catch {
      // Storage blocked: railMemory carries the choice.
    }
    railListeners.forEach((listener) => listener());
  };
  return [expanded, toggle] as const;
}

function AppSidebarContent({
  user,
  navItems,
  children,
  posImmersive,
  essOnly: essOnlyProp,
}: AppSidebarProps & { posImmersive: boolean }) {
  const pathname = usePathname();
  // ESS-only: sembunyikan seluruh jalan menuju desktop NüHabit OS.
  // Nilai dari server (IAM) diutamakan; fallback kebijakan role di kode.
  const essOnly = essOnlyProp ?? isEssOnlyRole(user.role);
  const isDesktop = useMediaQuery("(min-width: 1280px)");
  const [railPref, toggleRail] = useRailPreference();
  const railExpanded = railPref && isDesktop;
  const [accountOpen, setAccountOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const leaves = useMemo(() => flattenNavLeaves(navItems), [navItems]);
  const useActivityNotification = pathname.startsWith("/dashboard/purchasing");
  const canUseCentralCashier =
    user.has_central_cashier_menu === true && user.can_central_checkout === true;

  if (posImmersive) {
    return (
      <CanUseCentralCashierProvider value={canUseCentralCashier}>
        <PosNfcShell>
          <PosTabletManifestLink />
          <div className="arkiv-dashboard-theme min-h-screen bg-surface">
            <main className="min-h-[100dvh] overflow-auto p-2 sm:p-3 md:p-4">{children}</main>
          </div>
        </PosNfcShell>
      </CanUseCentralCashierProvider>
    );
  }

  return (
    <CanUseCentralCashierProvider value={canUseCentralCashier}>
    <PosNfcShell>
    {/* overflow-clip, not hidden: a hidden box can still be scrolled by focus. */}
    <div
      data-dashboard-shell
      className="arkiv-dashboard-theme relative flex h-dvh gap-4 overflow-clip bg-surface p-3 lg:p-4 print:block print:h-auto print:overflow-visible print:bg-white print:p-0"
    >
      <div aria-hidden className="pointer-events-none absolute inset-0 overflow-hidden print:hidden">
        <div
          className="absolute -top-[30%] -right-[8%] h-[120%] w-[70%] opacity-70 blur-xl"
          style={{ background: CANVAS_GLOW }}
        />
      </div>

      <aside
        className={`relative hidden shrink-0 flex-col rounded-hero bg-ink text-on-ink shadow-float transition-[width] duration-200 md:flex print:hidden ${
          railExpanded ? "w-60" : "w-[76px]"
        }`}
      >
        {/* Tekstur doodle Paddy di pucuk rail (dekoratif, memudar ke bawah). */}
        <div
          aria-hidden
          className="pointer-events-none absolute inset-x-0 top-0 h-28 rounded-t-hero bg-[url('/brand/pattern.png')] bg-[length:340px_auto] opacity-25 [mask-image:linear-gradient(to_bottom,black,transparent)]"
        />
        <RailHeader expanded={railExpanded} />
        <AppSidebarNav navItems={navItems} collapsed={!railExpanded} />
        <RailWorkspace
          expanded={railExpanded}
          companyName={user.company_name}
          branchName={user.branch_name}
          warehouseName={user.warehouse_name}
          canSwitchStall={user.can_switch_stall === true}
          canUseCentralCashier={canUseCentralCashier}
          activeStallId={user.active_stall_id ?? null}
        />
        {isDesktop && (
          <button
            type="button"
            onClick={toggleRail}
            aria-label={railExpanded ? "Ciutkan menu" : "Lebarkan menu"}
            className={`mx-3 mb-3 flex h-9 items-center gap-2 rounded-full px-3 text-xs font-semibold text-on-ink-muted transition-colors hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:outline-none ${
              railExpanded ? "" : "justify-center px-0"
            }`}
          >
            {railExpanded ? (
              <>
                <PanelLeftClose className="size-4" />
                Ciutkan
              </>
            ) : (
              <PanelLeftOpen className="size-4" />
            )}
          </button>
        )}
      </aside>

      <div className="relative flex min-w-0 flex-1 flex-col gap-4 print:block">
        <a
          href="#main"
          className="sr-only z-30 rounded-full bg-ink px-4 py-2 text-sm font-semibold text-on-ink shadow-float focus:not-sr-only focus:absolute focus:top-3 focus:left-4"
        >
          Lewati ke konten
        </a>

        <header className="flex h-14 shrink-0 items-center gap-3 print:hidden">
          <Link
            href="/dashboard"
            aria-label="Beranda"
            className="flex size-11 shrink-0 items-center justify-center rounded-2xl bg-ink shadow-card md:hidden"
          >
            <Image src="/brand/mark-lime.png" width={210} height={210} alt="" className="h-auto w-7 select-none" draggable={false} />
          </Link>
          <HeaderTitle branchName={user.branch_name ?? user.company_name} />
          <GlobalSearch
            leaves={leaves}
            className="hidden min-w-0 flex-1 md:ml-4 md:block md:max-w-md"
          />
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <button
              type="button"
              onClick={() => setSearchOpen((open) => !open)}
              aria-label="Cari halaman"
              aria-expanded={searchOpen}
              className={cn(ROUND_BUTTON, "md:hidden")}
            >
              <Search className="size-5" />
            </button>
            {!essOnly && (
              <Link
                href="/os"
                className={cn(ROUND_BUTTON, "hidden sm:inline-flex")}
                title="Buka Paddy OS desktop"
                aria-label="Buka desktop"
              >
                <MonitorSmartphone className="size-5" />
              </Link>
            )}
            <ThemeToggle />
            {useActivityNotification ? <ActivityLogBell /> : <NotificationBell />}
            <button
              type="button"
              onClick={() => setAccountOpen(true)}
              className="flex h-11 items-center gap-2.5 rounded-full bg-card p-1.5 shadow-card transition-colors hover:bg-surface-2 focus-visible:ring-2 focus-visible:ring-forest/40 focus-visible:outline-none xl:pr-4"
              aria-label={`Akun ${user.full_name}`}
            >
              <span className="grid size-8 shrink-0 place-items-center rounded-full bg-accent text-xs font-semibold text-accent-foreground">
                {user.full_name?.slice(0, 1).toUpperCase() || "A"}
              </span>
              <span className="hidden min-w-0 text-left xl:block">
                <span className="block max-w-[10rem] truncate text-sm leading-tight font-semibold text-foreground">
                  {user.full_name}
                </span>
                <span className="block max-w-[10rem] truncate text-[11px] text-muted-foreground capitalize">
                  {user.role.replace(/_/g, " ")}
                </span>
              </span>
            </button>
          </div>
        </header>
        {searchOpen && (
          <div className="-mt-2 md:hidden print:hidden">
            <GlobalSearch leaves={leaves} autoFocus onNavigate={() => setSearchOpen(false)} />
          </div>
        )}

        <main
          id="main"
          tabIndex={-1}
          className="relative min-h-0 flex-1 overflow-y-auto pr-0.5 pb-24 outline-none md:pb-2 print:block print:overflow-visible print:p-0"
        >
          <DashboardBreadcrumbs
            navItems={navItems}
            className="mb-4 hidden md:flex print:hidden"
          />
          {children}
        </main>
      </div>

      <PhoneNav
        leaves={leaves}
        userName={user.full_name}
        userRole={user.role}
        onAccount={() => setAccountOpen(true)}
      />

      {accountOpen && (
        <AccountPopup
          user={user}
          essOnly={essOnly}
          onClose={() => setAccountOpen(false)}
        />
      )}
    </div>
    </PosNfcShell>
    </CanUseCentralCashierProvider>
  );
}

function AppSidebarWithSearch(props: AppSidebarProps) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const posImmersive = isPosImmersiveShell(pathname, searchParams);

  return <AppSidebarContent {...props} posImmersive={posImmersive} />;
}

export default function AppSidebar(props: AppSidebarProps) {
  return (
    <Suspense
      fallback={<div data-dashboard-shell className="min-h-dvh w-full bg-surface" aria-hidden />}
    >
      <AppSidebarWithSearch {...props} />
    </Suspense>
  );
}

const ROUND_BUTTON =
  "inline-flex size-11 shrink-0 items-center justify-center rounded-full bg-card text-foreground shadow-card transition-colors hover:bg-surface active:scale-95 focus-visible:ring-2 focus-visible:ring-forest/40 focus-visible:outline-none";

function RailHeader({ expanded }: { expanded: boolean }) {
  return (
    <Link
      href="/dashboard"
      aria-label={`${brandOsName()} · Beranda`}
      className={`flex shrink-0 flex-col rounded-2xl pt-5 pb-3 focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:outline-none ${
        expanded ? "px-5" : "items-center px-0"
      }`}
    >
      {expanded ? (
        <>
          {/* Wordmark putih di permukaan gelap (Logo Colorways, brand guideline). */}
          <Image src="/brand/wordmark-white.png" width={640} height={165} alt="" className="relative h-auto w-36 select-none" draggable={false} />
          <span className="relative mt-2 block truncate text-[11px] font-bold text-sunny">Your Adorable Companion</span>
        </>
      ) : (
        <Image src="/brand/mark-lime.png" width={210} height={210} alt="" className="relative h-auto w-10 -rotate-6 select-none" draggable={false} />
      )}
    </Link>
  );
}

/**
 * Today's date and the active branch beside the search. The page title is
 * left to the breadcrumb and the page header so it is not shown twice.
 */
function HeaderTitle({ branchName }: { branchName?: string | null }) {
  const today = new Intl.DateTimeFormat("id-ID", {
    weekday: "long",
    day: "numeric",
    month: "short",
  }).format(new Date());

  return (
    <div className="hidden min-w-0 shrink-0 md:block md:max-w-[14rem] lg:max-w-[18rem]">
      <p className="truncate text-sm leading-tight font-semibold text-foreground" suppressHydrationWarning>
        {today}
      </p>
      {branchName?.trim() ? <p className="truncate text-xs text-muted-foreground">{branchName.trim()}</p> : null}
    </div>
  );
}

function RailWorkspace({
  expanded,
  companyName,
  branchName,
  warehouseName,
  canSwitchStall,
  canUseCentralCashier,
  activeStallId,
}: {
  expanded: boolean;
  companyName?: string | null;
  branchName?: string | null;
  warehouseName?: string | null;
  canSwitchStall: boolean;
  canUseCentralCashier: boolean;
  activeStallId: string | null;
}) {
  const scope = (
    <UserScopeLines
      companyName={companyName}
      branchName={branchName}
      warehouseName={warehouseName}
      variant="rail"
    />
  );
  const scopeLabel = [branchName?.trim() || companyName?.trim(), warehouseName?.trim()]
    .filter(Boolean)
    .join(" · ");

  if (!expanded) {
    const tile = (
      <span className="flex size-11 items-center justify-center rounded-2xl bg-white/5 text-on-ink-muted">
        <Store className="size-5" />
      </span>
    );
    return (
      <div className="flex shrink-0 justify-center px-2 pt-2 pb-2" title={scopeLabel}>
        {canSwitchStall ? (
          <StallSwitcher
            activeStallId={activeStallId}
            canUseCentralCashier={canUseCentralCashier}
            triggerClassName="rounded-2xl hover:bg-white/10"
          >
            {tile}
          </StallSwitcher>
        ) : (
          tile
        )}
      </div>
    );
  }

  return (
    <div className="mx-3 mt-2 mb-2 shrink-0 rounded-2xl bg-white/5 p-1.5">
      <p className="px-2 pt-1 text-[11px] font-semibold tracking-wider text-on-ink-muted uppercase">
        Lokasi aktif
      </p>
      {canSwitchStall ? (
        <StallSwitcher activeStallId={activeStallId} canUseCentralCashier={canUseCentralCashier}>
          {scope}
        </StallSwitcher>
      ) : (
        <div className="px-2 py-1.5">{scope}</div>
      )}
    </div>
  );
}

const THEME_MODES = [
  { value: "light" as const, icon: Sun, label: "Terang" },
  { value: "dark" as const, icon: Moon, label: "Gelap" },
  { value: "auto" as const, icon: Monitor, label: "Auto" },
];

function ThemeToggle() {
  const theme = useThemeOrNull();
  if (!theme) return null;
  const { state, setMode } = theme;
  const currentIdx = THEME_MODES.findIndex((m) => m.value === state.mode);
  const current = THEME_MODES[currentIdx] ?? THEME_MODES[0];
  const next = THEME_MODES[(currentIdx + 1) % THEME_MODES.length];
  const Icon = current.icon;

  return (
    <button
      type="button"
      className={cn("arkiv-theme-toggle", ROUND_BUTTON, "hidden sm:inline-flex")}
      title={`Tema: ${current.label}. Klik untuk ganti`}
      aria-label={`Tema: ${current.label}. Klik untuk ganti`}
      onClick={() => setMode(next.value)}
    >
      <Icon className="size-5" />
    </button>
  );
}

type StallOption = StallLabelSource & { id: string; code: string };

/**
 * Switcher stall di header sidebar — khusus super_admin & admin. Pilihan
 * dibatasi penempatan user (Main Storage = bebas semua stall), disimpan
 * sebagai cookie via /api/auth/active-stall lalu halaman di-reload agar
 * seluruh scope server mengikuti stall terpilih.
 */
export function StallSwitcher({
  activeStallId,
  canUseCentralCashier,
  triggerClassName,
  children,
}: {
  activeStallId: string | null;
  canUseCentralCashier: boolean;
  /** Replaces the default full-width row trigger (collapsed rail tile). */
  triggerClassName?: string;
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const hideAllStallsOption =
    (pathname.includes("/cashier") || pathname.includes("/restaurant")) &&
    !canUseCentralCashier;
  const [stalls, setStalls] = useState<StallOption[] | null>(null);
  const [allAccess, setAllAccess] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const { confirmAndSwitchStall, switching, dialogOpen, dialog } =
    useConfirmAndSwitchStall();
  const stallBusy = switching || dialogOpen;

  async function loadStalls() {
    if (stalls !== null) return;
    try {
      // Pilihan berbasis penempatan user (Main Storage = bebas semua stall).
      const res = await fetch("/api/auth/stall-options");
      const json = await res.json();
      if (!res.ok) throw new Error(json.error ?? "Failed to load outlets");
      setAllAccess(json.data?.all_access !== false);
      setStalls(
        ((json.data?.stalls ?? []) as StallOption[]).map(({ id, name, code, label }) => ({
          id,
          name,
          code,
          label,
        }))
      );
    } catch {
      setLoadFailed(true);
      setStalls([]);
    }
  }

  async function selectStall(warehouseId: string | null) {
    if (stallBusy || warehouseId === activeStallId) return;
    await confirmAndSwitchStall(warehouseId);
  }

  return (
    <>
    <DropdownMenu onOpenChange={(open) => open && loadStalls()}>
      <DropdownMenuTrigger
        className={
          triggerClassName ??
          "flex w-full min-w-0 items-center gap-1.5 rounded-xl px-2 py-1.5 text-left transition-colors hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:outline-none"
        }
        aria-label="Ganti outlet"
      >
        {triggerClassName ? (
          children
        ) : (
          <>
            <span className="min-w-0 flex-1">{children}</span>
            <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-on-ink-muted" />
          </>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="right"
        align="end"
        sideOffset={20}
        className="!w-64 overflow-hidden p-1.5"
      >
        {/* Scroll di wrapper dalam (bukan popup) agar radius sudut tidak terpotong scrollbar */}
        <div className="max-h-[min(24rem,calc(100vh-96px))] overflow-y-auto pr-0.5 [scrollbar-width:thin]">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="px-2 pb-1.5 pt-1 text-xs font-medium text-gray-400">
            Outlet
          </DropdownMenuLabel>
          {allAccess && !hideAllStallsOption && (
            <DropdownMenuItem
              disabled={stallBusy}
              onClick={() => selectStall(null)}
              className="gap-2.5 rounded-lg px-2 py-1.5"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-surface text-body">
                <Store className="h-4 w-4" />
              </span>
              <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                Semua Outlet
              </span>
              {activeStallId === null && <Check className="h-4 w-4 shrink-0 text-brand-text" />}
            </DropdownMenuItem>
          )}
          {hideAllStallsOption && (
            <p className="px-2 pb-1.5 text-[11px] leading-snug text-amber-700/90">
              Di kasir/restaurant wajib pilih satu outlet (bukan Semua Outlet).
            </p>
          )}
          {stalls === null ? (
            <div className="flex justify-center py-3">
              <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-border border-t-accent" />
            </div>
          ) : loadFailed ? (
            <p className="px-2 py-2 text-xs text-gray-400">Gagal memuat daftar outlet</p>
          ) : (
            stalls.map((stall) => (
              <DropdownMenuItem
                key={stall.id}
                disabled={stallBusy}
                onClick={() => selectStall(stall.id)}
                className="gap-2.5 rounded-lg px-2 py-1.5"
              >
                <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-surface text-body">
                  <Store className="h-4 w-4" />
                </span>
                <span
                  className="min-w-0 flex-1 truncate text-sm font-medium text-foreground"
                  title={`${stallLabel(stall)} (${stall.code})`}
                >
                  {stallLabel(stall)}
                </span>
                {activeStallId === stall.id && (
                  <Check className="h-4 w-4 shrink-0 text-brand-text" />
                )}
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuGroup>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
    {dialog}
    </>
  );
}

function UserScopeLines({
  companyName,
  branchName,
  warehouseName,
  variant = "default",
}: {
  companyName?: string | null;
  branchName?: string | null;
  warehouseName?: string | null;
  variant?: "default" | "rail";
}) {
  // Baris utama: branch; fallback ke company (mis. super_admin tanpa branch).
  const primary = branchName?.trim() || companyName?.trim() || "—";
  const warehouse = warehouseName?.trim() || null;

  if (variant === "rail") {
    return (
      <div className="min-w-0 space-y-0.5">
        <p className="truncate text-sm font-semibold text-white">{primary}</p>
        {warehouse && (
          <p className="truncate text-xs text-on-ink-muted" title={warehouse}>
            {warehouse}
          </p>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <p className="truncate text-sm font-semibold text-foreground">{primary}</p>
      {warehouse && (
        <p className="truncate text-xs text-muted-foreground" title={warehouse}>
          {warehouse}
        </p>
      )}
    </div>
  );
}

function AccountPopup({
  user,
  essOnly,
  onClose,
}: {
  user: SidebarUser;
  essOnly: boolean;
  onClose: () => void;
}) {
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);

  async function handleLogout() {
    if (loggingOut) return;
    setLoggingOut(true);
    try {
      await signOut();
      // Muat ulang penuh: buang cache React Query & state user sebelumnya.
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination
      window.location.href = "/login";
    } catch {
      setLoggingOut(false);
      toast.error("Failed to log out. Please try again.");
    }
  }

  const actionClass =
    "flex w-full cursor-pointer items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm font-medium text-foreground transition-colors hover:bg-surface [&_svg]:size-5 [&_svg]:text-muted-foreground";

  return (
    <>
      <div
        className="fixed inset-0 z-50 flex items-end justify-center bg-ink/50 p-3 backdrop-blur-[2px] md:items-start md:justify-end md:p-4 md:pt-20"
        onClick={onClose}
      >
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="account-popup-title"
          className="w-full max-w-sm overflow-hidden rounded-card bg-card shadow-float"
          onClick={(event) => event.stopPropagation()}
        >
          <div className="flex items-center justify-between px-5 pt-5 pb-2">
            <div>
              <h2 id="account-popup-title" className="text-lg font-semibold text-foreground">
                Akun
              </h2>
              <p className="text-xs text-muted-foreground">Sesi {brandOsName()} yang aktif</p>
            </div>
            <button
              onClick={onClose}
              className="inline-flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-surface hover:text-foreground focus-visible:ring-2 focus-visible:ring-forest/40 focus-visible:outline-none"
              aria-label="Tutup"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          <div className="p-4">
            <div className="relative isolate flex items-center gap-3 overflow-hidden rounded-2xl bg-ink p-4 text-on-ink">
              <div aria-hidden className="pointer-events-none absolute -top-16 -right-16 -z-10 size-40 rounded-full bg-accent/30 blur-3xl" />
              <div className="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-accent text-base font-bold text-accent-foreground">
                {user.full_name?.slice(0, 1).toUpperCase() || "A"}
              </div>
              <div className="min-w-0">
                <div className="truncate text-sm font-semibold text-white">
                  {user.full_name}
                </div>
                {user.email && (
                  <div className="truncate text-xs text-on-ink-muted">{user.email}</div>
                )}
                <div className="mt-1.5 inline-flex rounded-full bg-white/10 px-2.5 py-0.5 text-[11px] font-semibold text-white capitalize">
                  {user.role.replace(/_/g, " ")}
                </div>
              </div>
            </div>

            <div className="mt-3 rounded-2xl bg-surface-2 px-4 py-3">
              <UserScopeLines
                companyName={user.company_name}
                branchName={user.branch_name}
                warehouseName={user.warehouse_name}
              />
            </div>

            <div className="mt-3 grid gap-0.5">
              <Link href="/dashboard/me" onClick={onClose} className={actionClass}>
                <CircleUser />
                Profil
              </Link>
              <button
                type="button"
                onClick={() => setPasswordOpen(true)}
                className={actionClass}
              >
                <KeyRound />
                Ganti kata sandi
              </button>
              {!essOnly && (
                <Link href="/os" onClick={onClose} className={actionClass}>
                  <MonitorSmartphone />
                  Desktop
                </Link>
              )}
            </div>

            <div className="mt-2 border-t border-border pt-2">
              <button
                type="button"
                onClick={handleLogout}
                disabled={loggingOut}
                className="flex w-full cursor-pointer items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm font-medium text-danger transition-colors hover:bg-danger-soft disabled:opacity-60"
              >
                {loggingOut ? (
                  <Loader2 className="h-5 w-5 animate-spin" />
                ) : (
                  <LogOut className="h-5 w-5" />
                )}
                {loggingOut ? "Keluar…" : "Keluar"}
              </button>
            </div>
          </div>
        </div>
      </div>

      <ChangePasswordDialog open={passwordOpen} onOpenChange={setPasswordOpen} />
    </>
  );
}

function ChangePasswordDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [saving, setSaving] = useState(false);

  function resetForm() {
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
  }

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (saving) return;
    if (newPassword.length < 8) {
      toast.error("New password must be at least 8 characters");
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error("Password confirmation does not match");
      return;
    }
    setSaving(true);
    try {
      const res = await fetch("/api/auth/change-password", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
      });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(json.error ?? "Failed to change password");
      toast.success("Password changed successfully");
      resetForm();
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to change password");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) resetForm();
        onOpenChange(next);
      }}
    >
      <DialogPanel size="xs">
        <DialogPanelForm onSubmit={handleSubmit}>
          <DialogPanelHeader>
            <DialogPanelTitle>Change Password</DialogPanelTitle>
            <DialogPanelDescription>
              Enter your current password, then choose a new one.
            </DialogPanelDescription>
          </DialogPanelHeader>
          <DialogPanelBody className="space-y-4">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-foreground">
                Current Password
              </label>
              <Input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
                required
              />
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-foreground">
                New Password
              </label>
              <Input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
                placeholder="Minimum 8 characters"
                required
              />
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-foreground">
                Confirm New Password
              </label>
              <Input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
                required
              />
            </div>
          </DialogPanelBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={saving}
              className="h-10"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={saving}
              className="h-10 gap-2 px-4"
            >
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              {saving ? "Saving..." : "Save Password"}
            </Button>
          </DialogFooter>
        </DialogPanelForm>
      </DialogPanel>
    </Dialog>
  );
}

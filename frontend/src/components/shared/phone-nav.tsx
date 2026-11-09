"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useMemo, useState } from "react";
import { ChevronDown, Home, LayoutGrid, Search } from "lucide-react";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { isNavLinkActive } from "@/lib/iam/nav-active";
import { matchesNavQuery, type NavLeaf } from "@/lib/iam/nav-leaves";
import { cn } from "@/lib/utils";
import { AppSidebarNavIcon } from "./app-sidebar-nav-icons";
import { NavLink } from "./nav-link";
import { UserAvatar } from "./user-avatar";

const HOME_HREF = "/dashboard";
const RECENT_KEY = "bcd.admin.recent-nav";
const RECENT_LIMIT = 4;

function readRecent(): string[] {
  try {
    const raw = JSON.parse(window.localStorage.getItem(RECENT_KEY) ?? "[]");
    return Array.isArray(raw) ? raw.filter((v): v is string => typeof v === "string") : [];
  } catch {
    return [];
  }
}

function pushRecent(href: string) {
  try {
    const next = [href, ...readRecent().filter((h) => h !== href)].slice(0, RECENT_LIMIT);
    window.localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // Recent pages are a convenience; storage may be blocked.
  }
}

/**
 * Phone navigation (< md): a floating ink bottom bar (Home, the first three
 * menu sections the user can open, More) and the dark More sheet with every
 * destination.
 */
export function PhoneNav({
  leaves,
  userName,
  userRole,
  userAvatar = null,
  onAccount,
}: {
  leaves: NavLeaf[];
  userName: string;
  userRole: string;
  /** Foto profil akun (EPIC-087); null = inisial. */
  userAvatar?: string | null;
  onAccount: () => void;
}) {
  const pathname = usePathname();
  const [moreOpen, setMoreOpen] = useState(false);
  const allHrefs = useMemo(() => leaves.map((leaf) => leaf.href), [leaves]);

  // One shortcut per section: the IAM menu already follows the user's role.
  const shortcuts = useMemo(() => {
    const seen = new Set<string>();
    const picked: NavLeaf[] = [];
    for (const leaf of leaves) {
      if (leaf.href === HOME_HREF || seen.has(leaf.section)) continue;
      seen.add(leaf.section);
      picked.push(leaf);
      if (picked.length === 3) break;
    }
    return picked;
  }, [leaves]);

  const isActive = (href: string) =>
    href === HOME_HREF ? pathname === HOME_HREF : isNavLinkActive(pathname, href, allHrefs);

  const slot =
    "group flex min-w-0 flex-1 flex-col items-center justify-center gap-1 text-[11px] font-semibold focus-visible:outline-none";
  const tile = (active: boolean) =>
    cn(
      "flex h-9 w-12 items-center justify-center rounded-full transition-colors group-focus-visible:ring-2 group-focus-visible:ring-white/60 [&_svg]:size-5",
      active ? "bg-accent text-accent-foreground shadow-glow" : "text-on-ink-muted group-hover:text-white"
    );

  return (
    <>
      <nav
        aria-label="Navigasi utama"
        className="fixed inset-x-3 bottom-3 z-40 flex h-[68px] items-stretch rounded-[22px] bg-ink px-1 text-on-ink shadow-float md:hidden print:hidden"
      >
        <Link
          href={HOME_HREF}
          aria-current={isActive(HOME_HREF) ? "page" : undefined}
          className={slot}
        >
          <span className={tile(isActive(HOME_HREF))}>
            <Home />
          </span>
          <span className={isActive(HOME_HREF) ? "text-white" : "text-on-ink-muted"}>Beranda</span>
        </Link>
        {shortcuts.map((leaf) => {
          const active = isActive(leaf.href);
          return (
            <NavLink
              key={leaf.href}
              href={leaf.href}
              aria-current={active ? "page" : undefined}
              className={slot}
            >
              <span className={tile(active)}>
                <AppSidebarNavIcon name={leaf.icon} isActive={active} className="size-5" />
              </span>
              <span className={cn("max-w-full truncate px-1", active ? "text-white" : "text-on-ink-muted")}>
                {leaf.section}
              </span>
            </NavLink>
          );
        })}
        <button
          type="button"
          onClick={() => setMoreOpen(true)}
          aria-expanded={moreOpen}
          className={slot}
        >
          <span className={tile(moreOpen)}>
            <LayoutGrid />
          </span>
          <span className={moreOpen ? "text-white" : "text-on-ink-muted"}>Lainnya</span>
        </button>
      </nav>

      <Sheet open={moreOpen} onOpenChange={setMoreOpen}>
        <SheetContent
          side="bottom"
          showCloseButton={false}
          className="gap-0 overflow-y-auto bg-ink pb-[max(env(safe-area-inset-bottom),1rem)] text-on-ink"
        >
          {moreOpen && (
            <MoreSheetBody
              leaves={leaves}
              pathname={pathname}
              allHrefs={allHrefs}
              userName={userName}
              userRole={userRole}
              userAvatar={userAvatar}
              onNavigate={(href) => {
                pushRecent(href);
                setMoreOpen(false);
              }}
              onAccount={() => {
                setMoreOpen(false);
                onAccount();
              }}
            />
          )}
        </SheetContent>
      </Sheet>
    </>
  );
}

function MoreSheetBody({
  leaves,
  pathname,
  allHrefs,
  userName,
  userRole,
  userAvatar,
  onNavigate,
  onAccount,
}: {
  leaves: NavLeaf[];
  pathname: string;
  allHrefs: string[];
  userName: string;
  userRole: string;
  userAvatar: string | null;
  onNavigate: (href: string) => void;
  onAccount: () => void;
}) {
  const [query, setQuery] = useState("");
  const [recent] = useState(readRecent);
  const currentSection = leaves.find((leaf) =>
    isNavLinkActive(pathname, leaf.href, allHrefs)
  )?.section;
  const [openSection, setOpenSection] = useState<string | undefined>(currentSection);

  const sections = useMemo(() => {
    const trimmed = query.trim();
    const map = new Map<string, NavLeaf[]>();
    for (const leaf of leaves) {
      if (trimmed && !matchesNavQuery(leaf, trimmed)) continue;
      map.set(leaf.section, [...(map.get(leaf.section) ?? []), leaf]);
    }
    const entries = [...map.entries()];
    // Current section first, then menu order.
    entries.sort(([a], [b]) => Number(b === currentSection) - Number(a === currentSection));
    return entries;
  }, [leaves, query, currentSection]);

  const recentLeaves = recent
    .map((href) => leaves.find((leaf) => leaf.href === href))
    .filter((leaf): leaf is NavLeaf => Boolean(leaf));

  return (
    <>
      <SheetTitle className="sr-only">Semua halaman</SheetTitle>
      <div className="sticky top-0 z-10 bg-ink px-4 pt-4 pb-3">
        <div className="mx-auto mb-3 h-1 w-10 rounded-full bg-white/20" aria-hidden />
        <label className="flex h-11 items-center gap-2 rounded-2xl bg-white/10 px-3">
          <Search className="size-4 text-on-ink-muted" aria-hidden />
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Cari halaman"
            aria-label="Cari halaman"
            className="min-w-0 flex-1 !border-0 !bg-transparent text-base text-white outline-none !shadow-none placeholder:text-on-ink-muted"
          />
        </label>
      </div>

      {!query && recentLeaves.length > 0 && (
        <div className="px-5 pb-2">
          <p className="text-[11px] font-semibold tracking-wider text-on-ink-muted uppercase">
            Terakhir dibuka
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            {recentLeaves.map((leaf) => (
              <NavLink
                key={leaf.href}
                href={leaf.href}
                onClick={() => onNavigate(leaf.href)}
                className="rounded-full bg-white/10 px-3 py-2 text-xs font-medium hover:bg-white/20"
              >
                {leaf.label}
              </NavLink>
            ))}
          </div>
        </div>
      )}

      {sections.length === 0 && (
        <p className="px-5 py-6 text-center text-sm text-on-ink-muted">
          Tidak ada halaman untuk &ldquo;{query.trim()}&rdquo;
        </p>
      )}

      {sections.map(([section, items]) => {
        const open = Boolean(query.trim()) || openSection === section;
        return (
          <div key={section}>
            <button
              type="button"
              aria-expanded={open}
              onClick={() => setOpenSection(open ? undefined : section)}
              className="flex w-full items-center justify-between px-5 py-3 text-left text-[11px] font-semibold tracking-wider text-on-ink-muted uppercase hover:text-white"
            >
              {section}
              <ChevronDown className={cn("size-4 transition-transform", open && "rotate-180")} />
            </button>
            {open && (
              <div className="grid grid-cols-3 gap-2 px-3 pb-2">
                {items.map((leaf) => {
                  const active = isNavLinkActive(pathname, leaf.href, allHrefs);
                  return (
                    <NavLink
                      key={`${leaf.href}::${leaf.label}`}
                      href={leaf.href}
                      onClick={() => onNavigate(leaf.href)}
                      aria-current={active ? "page" : undefined}
                      className="flex flex-col items-center gap-2 rounded-2xl p-3 text-center transition-colors hover:bg-white/10"
                    >
                      <span
                        className={cn(
                          "relative flex size-11 items-center justify-center rounded-2xl [&_svg]:size-5",
                          active ? "bg-accent text-accent-foreground shadow-glow" : "bg-white/10"
                        )}
                      >
                        <AppSidebarNavIcon name={leaf.icon} isActive={active} className="size-5" />
                      </span>
                      <span className="line-clamp-2 text-xs leading-tight font-medium">
                        {leaf.label}
                      </span>
                    </NavLink>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}

      <div className="mt-3 flex items-center gap-3 border-t border-white/10 px-5 pt-4">
        <UserAvatar name={userName} src={userAvatar} size={36} className="bg-accent text-xs font-semibold text-accent-foreground" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{userName}</p>
          <p className="truncate text-xs text-on-ink-muted capitalize">{userRole.replace(/_/g, " ")}</p>
        </div>
        <button
          type="button"
          onClick={onAccount}
          className="inline-flex h-8 items-center rounded-full bg-white/10 px-3 text-xs font-semibold text-white hover:bg-white/20"
        >
          Akun
        </button>
      </div>
    </>
  );
}

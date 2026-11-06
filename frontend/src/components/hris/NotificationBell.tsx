"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Bell, BellRing, Check, CheckCircle } from "lucide-react";
import { sortNotifications, timeAgo } from "@/lib/hris/notification-time";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  useHrisNotifications,
  useMarkNotificationsRead,
  type HrisNotification,
} from "./use-hris-notifications";

const TYPE_STYLES: Record<string, { dot: string; bg: string }> = {
  approval: { dot: "bg-blue-500", bg: "bg-blue-50" },
  status_change: { dot: "bg-green-500", bg: "bg-green-50" },
  reminder: { dot: "bg-yellow-500", bg: "bg-yellow-50" },
  alert: { dot: "bg-red-500", bg: "bg-red-50" },
};

export function NotificationBell() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const notificationsQuery = useHrisNotifications();
  const notifications = notificationsQuery.data ?? [];
  const unreadCount = notifications.filter((n) => !n.is_read).length;
  const markMutation = useMarkNotificationsRead();
  const marking = markMutation.isPending ? markMutation.variables : null;

  const markRead = (id: string) => markMutation.mutate(id);
  const markAllRead = () => markMutation.mutate(null);

  function handleNotificationClick(n: HrisNotification) {
    if (!n.is_read) markRead(n.id);
    if (n.link) {
      setOpen(false);
      router.push(n.link);
    }
  }

  const sorted = sortNotifications(notifications);

  return (
    // Popover bersama (Base UI) menjaga dropdown di dalam layar; sebelumnya
    // dropdown absolute right-0 terpotong di tepi kiri layar HP (QA EPIC-088).
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        className="relative inline-flex size-11 items-center justify-center rounded-full bg-card text-foreground shadow-card transition-colors hover:bg-surface active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-forest/40"
        aria-label={unreadCount > 0 ? `Notifikasi, ${unreadCount} belum dibaca` : "Notifikasi"}
      >
        {unreadCount > 0 ? (
          <BellRing className="w-5 h-5" />
        ) : (
          <Bell className="w-5 h-5" />
        )}
        {unreadCount > 0 && (
          <span className="absolute -top-1 -right-1 flex h-[19px] min-w-[19px] items-center justify-center rounded-full border-2 border-surface bg-accent px-1 text-[10.5px] font-bold text-accent-foreground">
            {unreadCount > 99 ? "99+" : unreadCount}
          </span>
        )}
      </PopoverTrigger>

      <PopoverContent
        align="end"
        sideOffset={8}
        className="w-[min(400px,calc(100vw-1.5rem))] gap-0 overflow-hidden rounded-2xl border border-border bg-card p-0 shadow-float"
      >
        {/* Header */}
        <div className="flex items-center justify-between gap-3 px-4 pt-4 pb-2">
          <div>
            <h3 className="text-base font-semibold text-foreground">Notifikasi</h3>
            <p className="text-xs text-gray-500">
              {unreadCount > 0 ? `${unreadCount} belum dibaca` : "Semua sudah dibaca"}
            </p>
          </div>
          {unreadCount > 0 && (
            <button
              onClick={markAllRead}
              className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-foreground hover:bg-black/5"
            >
              <CheckCircle className="w-3.5 h-3.5" />
              Tandai semua
            </button>
          )}
        </div>

        {/* List */}
        <div className="max-h-[360px] overflow-y-auto divide-y divide-gray-50">
          {sorted.length === 0 ? (
            <div className="py-12 text-center">
              <Bell className="w-8 h-8 text-gray-200 mx-auto mb-2" />
              <p className="text-sm text-gray-400">Tidak ada notifikasi</p>
            </div>
          ) : (
            sorted.map((n) => {
              const style = TYPE_STYLES[n.type] || TYPE_STYLES.alert;
              return (
                <div
                  key={n.id}
                  className={`px-4 py-3 cursor-pointer transition-colors hover:bg-gray-50 ${!n.is_read ? style.bg : ""}`}
                  onClick={() => handleNotificationClick(n)}
                >
                  <div className="flex items-start gap-3">
                    {/* Unread dot */}
                    <div className="mt-1.5 shrink-0">
                      {n.is_read ? (
                        <div className="w-2 h-2 rounded-full bg-gray-200" />
                      ) : (
                        <div className={`w-2 h-2 rounded-full ${style.dot}`} />
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className={`text-sm leading-tight ${n.is_read ? "text-gray-600" : "text-gray-900 font-medium"}`}>
                        {n.title}
                      </p>
                      <p className="text-xs text-gray-500 mt-0.5 line-clamp-2">{n.message}</p>
                      <p className="text-xs text-gray-400 mt-1">{timeAgo(n.created_at)}</p>
                    </div>
                    {/* Mark read button (single) */}
                    {!n.is_read && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          markRead(n.id);
                        }}
                        disabled={marking === n.id}
                        className="shrink-0 mt-0.5 p-1 rounded hover:bg-white/80 text-gray-400 hover:text-green-600"
                        title="Tandai sudah dibaca"
                      >
                        <Check className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Footer */}
        {notifications.length > 0 && (
          <div className="px-4 py-2.5 border-t border-gray-100 bg-gray-50 text-center">
            <button
              onClick={() => { setOpen(false); void notificationsQuery.refetch(); }}
              className="text-xs text-gray-500 hover:text-gray-700"
            >
              Refresh
            </button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}

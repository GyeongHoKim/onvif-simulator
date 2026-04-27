import { useEffect, useState } from "react"
import {
  RiDashboardLine,
  RiNotificationLine,
  RiVideoLine,
  RiShieldUserLine,
  RiCameraLine,
  RiListUnordered,
  RiSunLine,
  RiMoonLine,
} from "@remixicon/react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { useTheme } from "@/components/theme-provider"
import { useSim } from "@/store/simulator"
import * as App from "@/lib/wails/wailsjs/go/main/App"
import { cn } from "@/lib/utils"

import { DashboardScreen } from "@/screens/Dashboard"
import { EventsScreen } from "@/screens/Events"
import { MediaScreen } from "@/screens/Media"
import { AuthScreen } from "@/screens/Auth"
import { DeviceScreen } from "@/screens/Device"
import { LogScreen } from "@/screens/Log"

type TabId = "dashboard" | "events" | "media" | "auth" | "device" | "log"

const NAV: { id: TabId; label: string; Icon: typeof RiDashboardLine }[] = [
  { id: "dashboard", label: "Dashboard", Icon: RiDashboardLine },
  { id: "events", label: "Events", Icon: RiNotificationLine },
  { id: "media", label: "Media", Icon: RiVideoLine },
  { id: "auth", label: "Auth", Icon: RiShieldUserLine },
  { id: "device", label: "Device", Icon: RiCameraLine },
  { id: "log", label: "Log", Icon: RiListUnordered },
]

function formatUptime(ns: number): string {
  const totalSec = Math.floor(ns / 1e9)
  if (totalSec <= 0) return "—"
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  return `${h.toString().padStart(2, "0")}:${m.toString().padStart(2, "0")}:${s.toString().padStart(2, "0")}`
}

export function Layout() {
  const [tab, setTab] = useState<TabId>("dashboard")
  const [transitioning, setTransitioning] = useState(false)
  const status = useSim((s) => s.status)
  const { theme, setTheme } = useTheme()

  useEffect(() => {
    void useSim.getState().bootstrap()
  }, [])

  const running = status?.running ?? false
  const listen = status?.listenAddr || ""
  const uptime = formatUptime(Number(status?.uptime ?? 0))
  const discovery = status?.discoveryMode || "—"

  async function toggleRun() {
    setTransitioning(true)
    try {
      if (running) {
        await App.Stop()
        toast.success("Simulator stopped")
      } else {
        await App.Start()
        toast.success("Simulator started")
      }
      await useSim.getState().refreshStatus()
    } catch (e) {
      toast.error(String(e))
    } finally {
      setTransitioning(false)
    }
  }

  const Body =
    tab === "dashboard" ? <DashboardScreen />
    : tab === "events" ? <EventsScreen />
    : tab === "media" ? <MediaScreen />
    : tab === "auth" ? <AuthScreen />
    : tab === "device" ? <DeviceScreen />
    : <LogScreen />

  return (
    <div className="flex h-svh flex-col bg-background text-foreground">
      <div className="flex min-h-0 flex-1">
        <aside className="flex w-60 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
          <button
            type="button"
            onClick={toggleRun}
            disabled={transitioning}
            className={cn(
              "m-3 flex items-center gap-2 rounded-md border px-3 py-2 text-left text-sm font-medium transition-colors",
              running
                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/20"
                : "border-border bg-muted text-muted-foreground hover:bg-muted/80",
              transitioning && "cursor-wait opacity-70"
            )}
          >
            {transitioning ? (
              <Spinner className="size-3" />
            ) : (
              <span
                className={cn(
                  "size-2 rounded-full",
                  running ? "bg-emerald-500" : "bg-muted-foreground/60"
                )}
              />
            )}
            <span className="truncate">
              {running ? `Running · ${listen}` : "Stopped"}
            </span>
          </button>

          <nav className="flex flex-col gap-0.5 px-2">
            {NAV.map(({ id, label, Icon }) => (
              <button
                key={id}
                type="button"
                onClick={() => setTab(id)}
                className={cn(
                  "flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors",
                  tab === id
                    ? "bg-sidebar-accent text-sidebar-accent-foreground"
                    : "text-sidebar-foreground/80 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
                )}
              >
                <Icon className="size-4" />
                {label}
              </button>
            ))}
          </nav>

          <div className="mt-auto flex items-center justify-between border-t px-3 py-2 text-xs text-muted-foreground">
            <span>v0.1.0</span>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              aria-label="Toggle theme"
            >
              {theme === "dark" ? <RiSunLine /> : <RiMoonLine />}
            </Button>
          </div>
        </aside>

        <main className="flex min-w-0 flex-1 flex-col overflow-y-auto">
          <div className="flex-1 p-6">{Body}</div>
        </main>
      </div>

      <footer className="flex items-center gap-4 border-t px-4 py-2 text-xs text-muted-foreground">
        <span>Uptime: {uptime}</span>
        <span
          className={cn(
            "rounded-full border px-2 py-0.5",
            discovery === "Discoverable"
              ? "border-emerald-500/30 text-emerald-700 dark:text-emerald-300"
              : "border-amber-500/30 text-amber-700 dark:text-amber-300"
          )}
        >
          {discovery}
        </span>
        <span className="ml-auto">onvif-simulator</span>
      </footer>
    </div>
  )
}

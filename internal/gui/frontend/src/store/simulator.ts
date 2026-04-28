// Global state for the GUI. See store/README.md for the rationale behind
// Zustand over alternatives.

import { create } from "zustand"
import * as App from "@/lib/wails/wailsjs/go/gui/App"
import { config as cfgNs, gui as guiNs } from "@/lib/wails/wailsjs/go/models"

type Status = guiNs.Status
type Config = cfgNs.Config
type UserView = guiNs.UserView
type WailsLogEntry = guiNs.LogEntry

export type LogEntry =
  | { kind: "event"; time: string; topic: string; source: string; payload: string }
  | { kind: "mutation"; time: string; op: string; target: string; detail: string }

const LOG_CAP = 500

function toLogEntry(e: WailsLogEntry): LogEntry {
  // Wails encodes time as `any` (it sees Go's time.Time). The runtime value is
  // an RFC3339Nano string per the backend's zap encoder; coerce defensively.
  const time = String(e.time)
  if (e.kind === "mutation") {
    return {
      kind: "mutation",
      time,
      op: e.op ?? "",
      target: e.target ?? "",
      detail: e.detail ?? "",
    }
  }
  return {
    kind: "event",
    time,
    topic: e.topic ?? "",
    source: e.source ?? "",
    payload: e.payload ?? "",
  }
}

type SimState = {
  status: Status | null
  config: Config | null
  users: UserView[]
  log: LogEntry[]

  bootstrap: () => Promise<void>
  refreshStatus: () => Promise<void>
  refreshConfig: () => Promise<void>
  refreshUsers: () => Promise<void>
  refreshLog: () => Promise<void>
  clearLog: () => Promise<void>
}

export const useSim = create<SimState>((set, get) => ({
  status: null,
  config: null,
  users: [],
  log: [],

  bootstrap: async () => {
    await Promise.all([
      get().refreshStatus(),
      get().refreshConfig(),
      get().refreshUsers(),
      get().refreshLog(),
    ])
  },

  refreshStatus: async () => {
    try {
      const s = await App.Status()
      set({ status: s })
    } catch {
      // ignore transient errors — polled every tick.
    }
  },

  refreshConfig: async () => {
    const c = await App.ConfigSnapshot()
    set({ config: c })
  },

  refreshUsers: async () => {
    const u = await App.Users()
    set({ users: u })
  },

  refreshLog: async () => {
    try {
      const page = await App.GetLogs(0, LOG_CAP)
      const entries = (page.entries ?? []).map(toLogEntry)
      set({ log: entries })
    } catch {
      // ignore transient errors — polled every tick.
    }
  },

  clearLog: async () => {
    await App.ClearLogs()
    set({ log: [] })
  },
}))

/** Poll Status() on an interval. Call once from a mounted screen. */
export function startStatusPolling(intervalMs = 1000): () => void {
  const tick = () => {
    void useSim.getState().refreshStatus()
  }
  const id = window.setInterval(tick, intervalMs)
  return () => window.clearInterval(id)
}

/** Poll the on-disk log file via App.GetLogs. Call from the Log screen. */
export function startLogPolling(intervalMs = 2000): () => void {
  const tick = () => {
    void useSim.getState().refreshLog()
  }
  const id = window.setInterval(tick, intervalMs)
  return () => window.clearInterval(id)
}

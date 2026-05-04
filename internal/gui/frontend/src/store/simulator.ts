// Global state for the GUI. See store/README.md for the rationale behind
// Zustand over alternatives.

import { create } from "zustand"
import * as App from "@/lib/wails/wailsjs/go/gui/App"
import * as wruntime from "@/lib/wails/wailsjs/runtime/runtime"
import { config as cfgNs, gui as guiNs } from "@/lib/wails/wailsjs/go/models"

type Status = guiNs.Status
type Config = cfgNs.Config
type UserView = guiNs.UserView
type EventRecord = guiNs.EventRecord

export type LogEntry =
  | { kind: "event"; time: string; topic: string; source: string; payload: string }
  | { kind: "mutation"; time: string; op: string; target: string; detail: string }
  | {
      kind: "log"
      time: string
      level: string
      component: string
      message: string
      attrs: Record<string, unknown>
    }

const LOG_CAP = 500

type SimState = {
  status: Status | null
  config: Config | null
  users: UserView[]
  log: LogEntry[]

  bootstrap: () => Promise<void>
  refreshStatus: () => Promise<void>
  refreshConfig: () => Promise<void>
  refreshUsers: () => Promise<void>
  appendEvent: (r: EventRecord) => void
  appendMutation: (r: {
    time: string
    kind: string
    target: string
    detail: string
  }) => void
  appendLog: (r: guiNs.LogRecord) => void
  clearLog: () => void
}

export const useSim = create<SimState>((set, get) => ({
  status: null,
  config: null,
  users: [],
  log: [],

  bootstrap: async () => {
    await Promise.all([get().refreshStatus(), get().refreshConfig(), get().refreshUsers()])
    const recent = await App.RecentLogs()
    for (const r of recent) {
      get().appendLog(r)
    }
    wruntime.EventsOn("event:new", (rec: EventRecord) => get().appendEvent(rec))
    wruntime.EventsOn("mutation:new", (rec) => get().appendMutation(rec))
    wruntime.EventsOn("log:new", (rec: guiNs.LogRecord) => get().appendLog(rec))
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

  appendEvent: (r) => {
    const entry: LogEntry = {
      kind: "event",
      time: String(r.time),
      topic: r.topic,
      source: r.source,
      payload: r.payload,
    }
    set((s) => ({ log: [entry, ...s.log].slice(0, LOG_CAP) }))
  },

  appendMutation: (r) => {
    const entry: LogEntry = {
      kind: "mutation",
      time: String(r.time),
      op: r.kind,
      target: r.target,
      detail: r.detail,
    }
    set((s) => ({ log: [entry, ...s.log].slice(0, LOG_CAP) }))
  },

  appendLog: (r) => {
    const attrs = r.attrs ?? {}
    const entry: LogEntry = {
      kind: "log",
      time: String(r.time),
      level: r.level,
      component: r.component,
      message: r.message,
      attrs: attrs as Record<string, unknown>,
    }
    set((s) => ({ log: [entry, ...s.log].slice(0, LOG_CAP) }))
  },

  clearLog: () => set({ log: [] }),
}))

/** Poll Status() on an interval. Call once from App.tsx. */
export function startStatusPolling(intervalMs = 1000): () => void {
  const tick = () => {
    void useSim.getState().refreshStatus()
  }
  const id = window.setInterval(tick, intervalMs)
  return () => window.clearInterval(id)
}

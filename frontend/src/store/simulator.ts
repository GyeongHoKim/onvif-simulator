// Global state for the GUI. See store/README.md for the rationale behind
// Zustand over alternatives.

import { create } from "zustand"
import * as App from "@/lib/wails/wailsjs/go/main/App"
import * as wruntime from "@/lib/wails/wailsjs/runtime/runtime"
import { config as cfgNs, main as mainNs } from "@/lib/wails/wailsjs/go/models"

type Status = mainNs.Status
type Config = cfgNs.Config
type UserView = mainNs.UserView
type EventRecord = mainNs.EventRecord

export type LogEntry =
  | { kind: "event"; time: string; topic: string; source: string; payload: string }
  | { kind: "mutation"; time: string; op: string; target: string; detail: string }

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
  clearLog: () => void
}

export const useSim = create<SimState>((set, get) => ({
  status: null,
  config: null,
  users: [],
  log: [],

  bootstrap: async () => {
    await Promise.all([get().refreshStatus(), get().refreshConfig(), get().refreshUsers()])
    wruntime.EventsOn("event:new", (rec: EventRecord) => get().appendEvent(rec))
    wruntime.EventsOn("mutation:new", (rec) => get().appendMutation(rec))
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

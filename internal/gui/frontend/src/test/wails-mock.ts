// Reusable mocks for the Wails-generated bindings. Tests vi.mock the modules
// to point at these helpers so screens render without a Go runtime present.

import { vi } from "vitest"

export type EventListener = (...payload: unknown[]) => void

const eventListeners = new Map<string, Set<EventListener>>()

export function emitWailsEvent(event: string, payload: unknown): void {
  const set = eventListeners.get(event)
  if (!set) return
  for (const cb of set) cb(payload)
}

export function resetWailsMocks(): void {
  eventListeners.clear()
  for (const [name, fn] of Object.entries(appMocks)) {
    if (typeof fn === "function" && "mockReset" in fn) {
      const m = fn as ReturnType<typeof vi.fn>
      m.mockReset()
      // Restore default resolved values so deferred promises from a prior
      // test don't poison subsequent state with undefined snapshots.
      switch (name) {
        case "Status":
          m.mockResolvedValue(defaultStatus())
          break
        case "ConfigSnapshot":
          m.mockResolvedValue(defaultConfig())
          break
        case "Users":
          m.mockResolvedValue(defaultUsers())
          break
        case "Running":
          m.mockResolvedValue(false)
          break
        default:
          m.mockResolvedValue(undefined)
      }
    }
  }
}

// Loose typing: the real Wails-generated functions take varied argument
// shapes — we erase that here so tests can inspect mock.calls without TS
// complaining about each individual signature.
type LooseFn = (...args: unknown[]) => Promise<unknown>

function lf(impl?: (...args: unknown[]) => unknown): ReturnType<typeof vi.fn<LooseFn>> {
  return vi.fn<LooseFn>(async (...args) => (impl ? impl(...args) : undefined))
}

export const appMocks = {
  Start: lf(),
  Stop: lf(),
  Running: lf(() => false),
  Status: lf(() => defaultStatus()),
  ConfigSnapshot: lf(() => defaultConfig()),
  Users: lf(() => defaultUsers()),
  Motion: lf(),
  ImageTooBlurry: lf(),
  ImageTooDark: lf(),
  ImageTooBright: lf(),
  DigitalInput: lf(),
  SyncProperty: lf(),
  PublishRaw: lf(),
  SetDiscoveryMode: lf(),
  SetHostname: lf(),
  AddProfile: lf(),
  RemoveProfile: lf(),
  SetProfileRTSP: lf(),
  SetProfileSnapshotURI: lf(),
  SetProfileEncoder: lf(),
  SetTopicEnabled: lf(),
  SetEventsTopics: lf(),
  AddUser: lf(),
  UpsertUser: lf(),
  RemoveUser: lf(),
  SetAuthEnabled: lf(),
}

export const runtimeMocks = {
  EventsOn: vi.fn((event: string, cb: EventListener) => {
    let set = eventListeners.get(event)
    if (!set) {
      set = new Set()
      eventListeners.set(event, set)
    }
    set.add(cb)
    return () => set?.delete(cb)
  }),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
}

// Reasonable defaults so screens render with non-empty data. The shapes
// match the Wails-generated classes well enough for the screens — but they're
// returned as `any` because the generated classes carry a private
// `convertValues` method that plain object literals don't have.

/* eslint-disable @typescript-eslint/no-explicit-any */
export function defaultStatus(): any {
  return {
    running: false,
    listenAddr: "",
    startedAt: "0001-01-01T00:00:00Z",
    uptime: 0,
    discoveryMode: "Discoverable",
    profileCount: 1,
    topicCount: 5,
    userCount: 1,
    activePullSubs: 0,
    recentEvents: [],
  }
}

export function defaultConfig(): any {
  return {
    version: 1,
    device: {
      uuid: "urn:uuid:00000000-0000-4000-8000-000000000001",
      manufacturer: "ONVIF Simulator",
      model: "SimCam-100",
      serial: "SN-0001",
      firmware: "0.1.0",
      scopes: ["onvif://www.onvif.org/Profile/Streaming"],
    },
    network: { http_port: 8080, xaddrs: [] },
    media: {
      profiles: [
        {
          name: "main",
          token: "profile_main",
          rtsp: "rtsp://127.0.0.1:8554/main",
          encoding: "H264",
          width: 1920,
          height: 1080,
          fps: 30,
          bitrate: 4096,
          gop_length: 60,
          snapshot_uri: "http://127.0.0.1:8080/snapshot/main.jpg",
          video_source_token: "VS_MAIN",
        },
      ],
    },
    auth: {
      enabled: true,
      users: [{ username: "admin", password: "admin", role: "Administrator" }],
      digest: { realm: "onvif-simulator", algorithms: ["MD5"], nonce_ttl: "5m" },
      jwt: { algorithms: ["RS256"] },
    },
    runtime: {
      discovery_mode: "Discoverable",
      hostname: "onvif-simulator",
      dns: { search_domain: ["local"], dns_manual: ["8.8.8.8"] },
      default_gateway: { ipv4_address: ["192.168.1.1"] },
      network_protocols: [{ name: "HTTP", enabled: true, port: [8080] }],
      system_date_and_time: { date_time_type: "NTP", tz: "UTC" },
    },
    events: {
      max_pull_points: 10,
      subscription_timeout: "1h",
      topics: [
        { name: "tns1:VideoSource/MotionAlarm", enabled: true },
        { name: "tns1:VideoSource/ImageTooBlurry", enabled: true },
        { name: "tns1:Device/Trigger/DigitalInput", enabled: true },
        { name: "tns1:Custom/Other", enabled: false },
      ],
    },
  }
}

export function defaultUsers(): any {
  return [{ username: "admin", roles: ["Administrator"] }]
}
/* eslint-enable @typescript-eslint/no-explicit-any */

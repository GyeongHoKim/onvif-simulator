import { describe, it, expect, vi, beforeEach } from "vitest"

import { useSim, startStatusPolling } from "./simulator"
import {
  appMocks,
  runtimeMocks,
  emitWailsEvent,
  defaultStatus,
  defaultConfig,
  defaultUsers,
} from "@/test/wails-mock"

function reset() {
  useSim.setState({ status: null, config: null, users: [], log: [] })
}

beforeEach(() => {
  reset()
  appMocks.Status.mockResolvedValue(defaultStatus())
  appMocks.ConfigSnapshot.mockResolvedValue(defaultConfig())
  appMocks.Users.mockResolvedValue(defaultUsers())
})

describe("simulator store", () => {
  it("bootstrap pulls status, config, users and subscribes to events", async () => {
    await useSim.getState().bootstrap()

    expect(appMocks.Status).toHaveBeenCalled()
    expect(appMocks.ConfigSnapshot).toHaveBeenCalled()
    expect(appMocks.Users).toHaveBeenCalled()
    expect(useSim.getState().status?.discoveryMode).toBe("Discoverable")
    expect(useSim.getState().config?.media.profiles[0].token).toBe("profile_main")
    expect(useSim.getState().users[0].username).toBe("admin")

    const onSubs = runtimeMocks.EventsOn.mock.calls.map((c) => c[0])
    expect(onSubs).toContain("event:new")
    expect(onSubs).toContain("mutation:new")
  })

  it("appendEvent caps the log at 500 entries and prepends new ones", () => {
    const { appendEvent } = useSim.getState()
    for (let i = 0; i < 510; i++) {
      appendEvent({
        time: new Date().toISOString(),
        topic: "tns1:T",
        source: `vs${i}`,
        payload: "state=true",
      } as Parameters<typeof appendEvent>[0])
    }
    const log = useSim.getState().log
    expect(log).toHaveLength(500)
    expect(log[0].kind).toBe("event")
    if (log[0].kind === "event") expect(log[0].source).toBe("vs509")
  })

  it("appendMutation records mutation entries", () => {
    useSim.getState().appendMutation({
      time: new Date().toISOString(),
      kind: "AddProfile",
      target: "profile_x",
      detail: "x",
    })
    const log = useSim.getState().log
    expect(log[0].kind).toBe("mutation")
    if (log[0].kind === "mutation") {
      expect(log[0].op).toBe("AddProfile")
      expect(log[0].target).toBe("profile_x")
    }
  })

  it("clearLog empties the log", () => {
    useSim.setState({
      log: [
        { kind: "event", time: "x", topic: "t", source: "", payload: "" },
      ],
    })
    useSim.getState().clearLog()
    expect(useSim.getState().log).toHaveLength(0)
  })

  it("refreshStatus swallows transient errors so polling stays alive", async () => {
    appMocks.Status.mockRejectedValueOnce(new Error("boom"))
    await expect(useSim.getState().refreshStatus()).resolves.toBeUndefined()
    expect(useSim.getState().status).toBeNull()
  })

  it("event:new wails events feed appendEvent through the bootstrap subscription", async () => {
    await useSim.getState().bootstrap()
    emitWailsEvent("event:new", {
      time: new Date().toISOString(),
      topic: "tns1:VideoSource/MotionAlarm",
      source: "VS_MAIN",
      payload: "state=true",
    })
    const log = useSim.getState().log
    expect(log).toHaveLength(1)
    if (log[0].kind === "event") {
      expect(log[0].topic).toBe("tns1:VideoSource/MotionAlarm")
    }
  })

  it("startStatusPolling polls Status() on the given interval and returns a stop fn", () => {
    vi.useFakeTimers()
    try {
      const stop = startStatusPolling(50)
      vi.advanceTimersByTime(120)
      // Bootstrap calls Status once; polling adds two ticks.
      expect(appMocks.Status.mock.calls.length).toBeGreaterThanOrEqual(2)
      const before = appMocks.Status.mock.calls.length
      stop()
      vi.advanceTimersByTime(200)
      expect(appMocks.Status.mock.calls.length).toBe(before)
    } finally {
      vi.useRealTimers()
    }
  })
})

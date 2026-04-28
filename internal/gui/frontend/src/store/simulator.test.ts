import { describe, it, expect, vi, beforeEach } from "vitest"

import { useSim, startStatusPolling, startLogPolling } from "./simulator"
import {
  appMocks,
  defaultStatus,
  defaultConfig,
  defaultUsers,
  defaultLogPage,
} from "@/test/wails-mock"

function reset() {
  useSim.setState({ status: null, config: null, users: [], log: [] })
}

beforeEach(() => {
  reset()
  appMocks.Status.mockResolvedValue(defaultStatus())
  appMocks.ConfigSnapshot.mockResolvedValue(defaultConfig())
  appMocks.Users.mockResolvedValue(defaultUsers())
  appMocks.GetLogs.mockResolvedValue(defaultLogPage())
})

describe("simulator store", () => {
  it("bootstrap pulls status, config, users, and seeds log via GetLogs", async () => {
    appMocks.GetLogs.mockResolvedValueOnce({
      entries: [
        {
          time: "2026-04-28T10:00:00Z",
          kind: "event",
          topic: "tns1:VideoSource/MotionAlarm",
          source: "VS_MAIN",
          payload: "state=true",
        },
        {
          time: "2026-04-28T10:00:01Z",
          kind: "mutation",
          op: "AddProfile",
          target: "profile_x",
          detail: "x",
        },
      ],
      total: 2,
    })

    await useSim.getState().bootstrap()

    expect(appMocks.Status).toHaveBeenCalled()
    expect(appMocks.ConfigSnapshot).toHaveBeenCalled()
    expect(appMocks.Users).toHaveBeenCalled()
    expect(appMocks.GetLogs).toHaveBeenCalledWith(0, 500)

    const log = useSim.getState().log
    expect(log).toHaveLength(2)
    if (log[0].kind === "event") {
      expect(log[0].topic).toBe("tns1:VideoSource/MotionAlarm")
    } else {
      throw new Error("expected first entry to be an event")
    }
    if (log[1].kind === "mutation") {
      expect(log[1].op).toBe("AddProfile")
      expect(log[1].target).toBe("profile_x")
    } else {
      throw new Error("expected second entry to be a mutation")
    }
  })

  it("refreshLog requests at most LOG_CAP entries", async () => {
    await useSim.getState().refreshLog()
    expect(appMocks.GetLogs).toHaveBeenCalledWith(0, 500)
  })

  it("refreshLog swallows transient errors so polling stays alive", async () => {
    appMocks.GetLogs.mockRejectedValueOnce(new Error("boom"))
    await expect(useSim.getState().refreshLog()).resolves.toBeUndefined()
    expect(useSim.getState().log).toEqual([])
  })

  it("clearLog calls App.ClearLogs and empties the log", async () => {
    useSim.setState({
      log: [
        { kind: "event", time: "x", topic: "t", source: "", payload: "" },
      ],
    })
    await useSim.getState().clearLog()
    expect(appMocks.ClearLogs).toHaveBeenCalled()
    expect(useSim.getState().log).toHaveLength(0)
  })

  it("refreshStatus swallows transient errors so polling stays alive", async () => {
    appMocks.Status.mockRejectedValueOnce(new Error("boom"))
    await expect(useSim.getState().refreshStatus()).resolves.toBeUndefined()
    expect(useSim.getState().status).toBeNull()
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

  it("startLogPolling polls GetLogs on the given interval and returns a stop fn", () => {
    vi.useFakeTimers()
    try {
      const stop = startLogPolling(50)
      vi.advanceTimersByTime(120)
      expect(appMocks.GetLogs.mock.calls.length).toBeGreaterThanOrEqual(2)
      const before = appMocks.GetLogs.mock.calls.length
      stop()
      vi.advanceTimersByTime(200)
      expect(appMocks.GetLogs.mock.calls.length).toBe(before)
    } finally {
      vi.useRealTimers()
    }
  })
})

import { describe, it, expect, beforeEach } from "vitest"
import { render, screen } from "@testing-library/react"

import { DashboardScreen } from "./Dashboard"
import { useSim } from "@/store/simulator"
import { defaultStatus } from "@/test/wails-mock"

beforeEach(() => {
  useSim.setState({ status: null, config: null, users: [], log: [] })
})

describe("Dashboard", () => {
  it("renders empty state for recent events when status is missing", () => {
    render(<DashboardScreen />)
    expect(screen.getByText(/no recent events/i)).toBeInTheDocument()
  })

  it("renders status numbers and recent events table when populated", () => {
    useSim.setState({
      status: {
        ...defaultStatus(),
        running: true,
        listenAddr: "0.0.0.0:8080",
        profileCount: 2,
        topicCount: 3,
        userCount: 1,
        activePullSubs: 4,
        recentEvents: [
          {
            time: new Date("2026-04-25T12:34:56Z").toISOString(),
            topic: "tns1:VideoSource/MotionAlarm",
            source: "VS_MAIN",
            payload: "state=true",
          },
        ],
      },
    })
    render(<DashboardScreen />)
    expect(screen.getByText("0.0.0.0:8080")).toBeInTheDocument()
    expect(screen.getByText("2")).toBeInTheDocument()
    expect(screen.getByText("VideoSource/MotionAlarm")).toBeInTheDocument()
    expect(screen.getByText("VS_MAIN")).toBeInTheDocument()
  })

  it("formats discovery as a stat card", () => {
    useSim.setState({
      status: { ...defaultStatus(), discoveryMode: "NonDiscoverable" },
    })
    render(<DashboardScreen />)
    expect(screen.getByText("NonDiscoverable")).toBeInTheDocument()
  })

  it("formats uptime hours/minutes/seconds when running", () => {
    useSim.setState({
      status: {
        ...defaultStatus(),
        running: true,
        // 1h 2m 3s in nanoseconds.
        uptime: (3600 + 120 + 3) * 1e9,
      },
    })
    render(<DashboardScreen />)
    expect(screen.getByText(/1h 2m 3s/)).toBeInTheDocument()
  })
})

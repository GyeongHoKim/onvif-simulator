import { describe, it, expect, beforeEach } from "vitest"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { LogScreen } from "./Log"
import { useSim } from "@/store/simulator"

beforeEach(() => {
  useSim.setState({ status: null, config: null, users: [], log: [] })
})

describe("Log screen", () => {
  it("shows empty state when log is empty", () => {
    render(<LogScreen />)
    expect(screen.getByText(/no log entries/i)).toBeInTheDocument()
  })

  it("renders event and mutation rows", () => {
    useSim.setState({
      log: [
        {
          kind: "event",
          time: new Date("2026-04-25T01:02:03Z").toISOString(),
          topic: "tns1:VideoSource/MotionAlarm",
          source: "VS_MAIN",
          payload: "state=true",
        },
        {
          kind: "mutation",
          time: new Date("2026-04-25T01:02:04Z").toISOString(),
          op: "AddProfile",
          target: "profile_x",
          detail: "x",
        },
      ],
    })
    render(<LogScreen />)
    expect(screen.getByText("VideoSource/MotionAlarm")).toBeInTheDocument()
    expect(screen.getByText("AddProfile")).toBeInTheDocument()
    expect(screen.getByText("VS_MAIN")).toBeInTheDocument()
    expect(screen.getByText("profile_x")).toBeInTheDocument()
  })

  it("filters by kind via the checkbox", async () => {
    const user = userEvent.setup()
    useSim.setState({
      log: [
        {
          kind: "event",
          time: new Date().toISOString(),
          topic: "tns1:T",
          source: "",
          payload: "x",
        },
        {
          kind: "mutation",
          time: new Date().toISOString(),
          op: "Op",
          target: "t",
          detail: "d",
        },
      ],
    })
    render(<LogScreen />)
    expect(screen.getByText("Op")).toBeInTheDocument()
    // Toggle the Mutations filter via its checkbox role (case-insensitive).
    const checkboxes = screen.getAllByRole("checkbox")
    // Order: Events, Mutations.
    await user.click(checkboxes[1])
    expect(screen.queryByText("Op")).not.toBeInTheDocument()
  })

  it("clears the log when Clear is clicked", async () => {
    const user = userEvent.setup()
    useSim.setState({
      log: [
        {
          kind: "event",
          time: new Date().toISOString(),
          topic: "tns1:T",
          source: "",
          payload: "",
        },
      ],
    })
    render(<LogScreen />)
    await user.click(screen.getByRole("button", { name: /^clear$/i }))
    expect(useSim.getState().log).toHaveLength(0)
  })

  it("filters by search query", async () => {
    const user = userEvent.setup()
    useSim.setState({
      log: [
        {
          kind: "event",
          time: new Date().toISOString(),
          topic: "tns1:Motion",
          source: "VS_A",
          payload: "x",
        },
        {
          kind: "event",
          time: new Date().toISOString(),
          topic: "tns1:Other",
          source: "VS_B",
          payload: "x",
        },
      ],
    })
    render(<LogScreen />)
    await user.type(screen.getByPlaceholderText(/search/i), "Motion")
    expect(screen.getByText("Motion")).toBeInTheDocument()
    expect(screen.queryByText("Other")).not.toBeInTheDocument()
  })
})

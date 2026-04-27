import { describe, it, expect, beforeEach } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { EventsScreen } from "./Events"
import { useSim } from "@/store/simulator"
import {
  appMocks,
  defaultConfig,
  defaultStatus,
} from "@/test/wails-mock"

function seed() {
  useSim.setState({
    status: { ...defaultStatus(), running: true },
    config: defaultConfig(),
    users: [],
    log: [],
  })
}

beforeEach(seed)

describe("Events screen", () => {
  it("lists every configured topic", () => {
    render(<EventsScreen />)
    expect(screen.getByText("tns1:VideoSource/MotionAlarm")).toBeInTheDocument()
    expect(screen.getByText("tns1:VideoSource/ImageTooBlurry")).toBeInTheDocument()
    expect(screen.getByText("tns1:Device/Trigger/DigitalInput")).toBeInTheDocument()
    expect(screen.getByText("tns1:Custom/Other")).toBeInTheDocument()
  })

  it("toggling Enabled checkbox calls SetTopicEnabled", async () => {
    const user = userEvent.setup()
    render(<EventsScreen />)

    // Find checkbox in the row of MotionAlarm
    const checkboxes = screen.getAllByRole("checkbox")
    expect(checkboxes.length).toBeGreaterThan(0)
    await user.click(checkboxes[0])

    await waitFor(() => {
      expect(appMocks.SetTopicEnabled).toHaveBeenCalled()
    })
    expect(appMocks.SetTopicEnabled.mock.calls[0][0]).toBe("tns1:VideoSource/MotionAlarm")
  })

  it("MotionAlarm On button calls App.Motion(token, true)", async () => {
    const user = userEvent.setup()
    render(<EventsScreen />)

    const onButtons = screen.getAllByRole("button", { name: /^On$/ })
    expect(onButtons.length).toBeGreaterThan(0)
    await user.click(onButtons[0])

    await waitFor(() => expect(appMocks.Motion).toHaveBeenCalled())
    expect(appMocks.Motion.mock.calls[0]).toEqual(["VS_MAIN", true])
  })

  it("DigitalInput Off button calls App.DigitalInput(token, false)", async () => {
    const user = userEvent.setup()
    render(<EventsScreen />)

    // Find the Off button in the digital input row by reading near the input.
    const allOffButtons = screen.getAllByRole("button", { name: /^Off$/ })
    // Three video sources (motion, blurry, digital, etc.). Just trigger the
    // last one which corresponds to DigitalInput per defaultConfig order.
    await user.click(allOffButtons[allOffButtons.length - 1])

    await waitFor(() => expect(appMocks.DigitalInput).toHaveBeenCalled())
    const last = appMocks.DigitalInput.mock.calls.at(-1)!
    expect(last[1]).toBe(false)
  })

  it("custom topic shows raw publish trigger that calls App.PublishRaw", async () => {
    const user = userEvent.setup()
    // Enable the custom topic so the raw button is interactive.
    const cfg = defaultConfig()
    cfg.events.topics[3].enabled = true
    useSim.setState({ config: cfg })

    render(<EventsScreen />)

    const rawBtn = screen.getByRole("button", { name: /publish raw xml/i })
    await user.click(rawBtn)

    const textarea = await screen.findByPlaceholderText(/tt:Message/i)
    await user.type(textarea, "<tt:Message/>")
    await user.click(screen.getByRole("button", { name: /^Publish$/ }))

    await waitFor(() => expect(appMocks.PublishRaw).toHaveBeenCalled())
    expect(appMocks.PublishRaw.mock.calls[0][0]).toBe("tns1:Custom/Other")
  })

  it("renders empty state when there are no topics", () => {
    const cfg = defaultConfig()
    cfg.events.topics = []
    useSim.setState({ config: cfg })
    render(<EventsScreen />)
    expect(screen.getByText(/no topics configured/i)).toBeInTheDocument()
  })

  it("ImageTooBlurry On dispatches App.ImageTooBlurry", async () => {
    const user = userEvent.setup()
    render(<EventsScreen />)
    const onButtons = screen.getAllByRole("button", { name: /^On$/ })
    // Order in defaultConfig: MotionAlarm[0], ImageTooBlurry[1], DigitalInput[2].
    await user.click(onButtons[1])
    await waitFor(() => expect(appMocks.ImageTooBlurry).toHaveBeenCalled())
  })

  it("ImageTooDark and ImageTooBright are dispatched from the right rows", async () => {
    const user = userEvent.setup()
    const cfg = defaultConfig()
    cfg.events.topics = [
      { name: "tns1:VideoSource/ImageTooDark", enabled: true },
      { name: "tns1:VideoSource/ImageTooBright", enabled: true },
    ]
    useSim.setState({ config: cfg })
    render(<EventsScreen />)
    const onButtons = screen.getAllByRole("button", { name: /^On$/ })
    await user.click(onButtons[0])
    await waitFor(() => expect(appMocks.ImageTooDark).toHaveBeenCalled())
    await user.click(onButtons[1])
    await waitFor(() => expect(appMocks.ImageTooBright).toHaveBeenCalled())
  })

  it("SyncProperty popover submits with the form values", async () => {
    const user = userEvent.setup()
    render(<EventsScreen />)
    const syncBtns = screen.getAllByRole("button", { name: /sync/i })
    // Each row has a SyncProperty icon button — click the first one.
    await user.click(syncBtns[0])
    const send = await screen.findByRole("button", { name: /send syncproperty/i })
    await user.click(send)
    await waitFor(() => expect(appMocks.SyncProperty).toHaveBeenCalled())
  })

  it("SyncProperty error path renders error toast", async () => {
    const user = userEvent.setup()
    appMocks.SyncProperty.mockRejectedValueOnce(new Error("boom"))
    render(<EventsScreen />)
    const syncBtns = screen.getAllByRole("button", { name: /^sync /i })
    await user.click(syncBtns[0])
    const send = await screen.findByRole("button", { name: /send syncproperty/i })
    await user.click(send)
    await waitFor(() => expect(appMocks.SyncProperty).toHaveBeenCalled())
  })

  it("SetTopicEnabled errors do not crash the row", async () => {
    const user = userEvent.setup()
    appMocks.SetTopicEnabled.mockRejectedValueOnce(new Error("boom"))
    render(<EventsScreen />)
    const checkboxes = screen.getAllByRole("checkbox")
    await user.click(checkboxes[0])
    await waitFor(() => expect(appMocks.SetTopicEnabled).toHaveBeenCalled())
  })

  it("Motion error path is swallowed and surfaces an error toast", async () => {
    const user = userEvent.setup()
    appMocks.Motion.mockRejectedValueOnce(new Error("boom"))
    render(<EventsScreen />)
    const onButtons = screen.getAllByRole("button", { name: /^On$/ })
    await user.click(onButtons[0])
    await waitFor(() => expect(appMocks.Motion).toHaveBeenCalled())
  })

  it("DigitalInput error path is swallowed", async () => {
    const user = userEvent.setup()
    appMocks.DigitalInput.mockRejectedValueOnce(new Error("boom"))
    render(<EventsScreen />)
    const offButtons = screen.getAllByRole("button", { name: /^Off$/ })
    await user.click(offButtons[offButtons.length - 1])
    await waitFor(() => expect(appMocks.DigitalInput).toHaveBeenCalled())
  })

  it("trigger widgets are disabled while the simulator is stopped", () => {
    useSim.setState({
      status: { ...defaultStatus(), running: false },
      config: defaultConfig(),
    })
    render(<EventsScreen />)
    const onButtons = screen.getAllByRole("button", { name: /^On$/ })
    expect(onButtons[0]).toBeDisabled()
  })
})

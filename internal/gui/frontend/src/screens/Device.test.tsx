import { describe, it, expect, beforeEach } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { DeviceScreen } from "./Device"
import { useSim } from "@/store/simulator"
import { appMocks, defaultConfig, defaultStatus } from "@/test/wails-mock"

beforeEach(() => {
  useSim.setState({
    status: defaultStatus(),
    config: defaultConfig(),
    users: [],
    log: [],
  })
})

describe("Device screen", () => {
  it("renders identity card values", () => {
    render(<DeviceScreen />)
    expect(screen.getByText("ONVIF Simulator")).toBeInTheDocument()
    expect(screen.getByText("SimCam-100")).toBeInTheDocument()
    expect(screen.getByText("SN-0001")).toBeInTheDocument()
  })

  it("discovery segmented control toggles via SetDiscoveryMode", async () => {
    const user = userEvent.setup()
    render(<DeviceScreen />)
    await user.click(screen.getByRole("button", { name: /^NonDiscoverable$/ }))
    await waitFor(() =>
      expect(appMocks.SetDiscoveryMode).toHaveBeenCalledWith("NonDiscoverable")
    )
  })

  it("Save hostname calls SetHostname with the input value", async () => {
    const user = userEvent.setup()
    render(<DeviceScreen />)
    const hostInput = screen.getByDisplayValue("onvif-simulator")
    await user.clear(hostInput)
    await user.type(hostInput, "newhost")
    await user.click(screen.getByRole("button", { name: /^save$/i }))
    await waitFor(() => expect(appMocks.SetHostname).toHaveBeenCalledWith("newhost"))
  })

  it("renders runtime read-only DNS values", () => {
    render(<DeviceScreen />)
    expect(screen.getByText("8.8.8.8")).toBeInTheDocument()
    expect(screen.getByText("192.168.1.1")).toBeInTheDocument()
  })

  it("hostname save surfaces backend error inline", async () => {
    const user = userEvent.setup()
    appMocks.SetHostname.mockRejectedValueOnce(new Error("config: hostname empty"))
    render(<DeviceScreen />)
    const hostInput = screen.getByDisplayValue("onvif-simulator")
    await user.clear(hostInput)
    await user.type(hostInput, "x")
    await user.click(screen.getByRole("button", { name: /^save$/i }))
    expect(await screen.findByText(/hostname empty/i)).toBeInTheDocument()
  })

  it("returns null while config is still loading", () => {
    useSim.setState({ config: null })
    const { container } = render(<DeviceScreen />)
    expect(container).toBeEmptyDOMElement()
  })
})

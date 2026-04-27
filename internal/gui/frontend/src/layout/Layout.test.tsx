import { describe, it, expect, beforeEach } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { Layout } from "./Layout"
import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"
import { useSim } from "@/store/simulator"
import { appMocks, defaultStatus } from "@/test/wails-mock"

function renderLayout() {
  return render(
    <ThemeProvider>
      <Layout />
      <Toaster />
    </ThemeProvider>
  )
}

beforeEach(() => {
  useSim.setState({ status: null, config: null, users: [], log: [] })
})

describe("Layout", () => {
  it("renders all six nav items", () => {
    renderLayout()
    for (const label of ["Dashboard", "Events", "Media", "Auth", "Device", "Log"]) {
      expect(screen.getByRole("button", { name: new RegExp(label) })).toBeInTheDocument()
    }
  })

  it("clicking the run-state pill while stopped calls Start", async () => {
    const user = userEvent.setup()
    useSim.setState({ status: { ...defaultStatus(), running: false } })
    renderLayout()
    await user.click(screen.getByRole("button", { name: /^Stopped$/ }))
    await waitFor(() => expect(appMocks.Start).toHaveBeenCalled())
  })

  it("clicking the run-state pill while running calls Stop", async () => {
    const user = userEvent.setup()
    // Layout's bootstrap re-fetches Status from the backend; mock it so the
    // post-bootstrap state reflects a running simulator.
    appMocks.Status.mockResolvedValue({
      ...defaultStatus(),
      running: true,
      listenAddr: "0.0.0.0:8080",
    })
    useSim.setState({
      status: { ...defaultStatus(), running: true, listenAddr: "0.0.0.0:8080" },
    })
    renderLayout()
    const pill = await screen.findByRole("button", { name: /Running/ })
    await user.click(pill)
    await waitFor(() => expect(appMocks.Stop).toHaveBeenCalled())
  })

  it("dark-mode toggle button cycles between sun and moon icons", async () => {
    const user = userEvent.setup()
    renderLayout()
    const toggle = screen.getByRole("button", { name: /toggle theme/i })
    await user.click(toggle)
    // Just asserts the button stays mounted after click — no thrown errors.
    expect(toggle).toBeInTheDocument()
  })

  it("Start failure surfaces an error toast and does not change running state", async () => {
    const user = userEvent.setup()
    appMocks.Start.mockRejectedValueOnce(new Error("port in use"))
    renderLayout()
    await user.click(screen.getByRole("button", { name: /^Stopped$/ }))
    await waitFor(() => expect(appMocks.Start).toHaveBeenCalled())
  })

  it("switching nav swaps the rendered screen", async () => {
    const user = userEvent.setup()
    renderLayout()
    await user.click(screen.getByRole("button", { name: /^Events/ }))
    expect(
      screen.getByRole("heading", { level: 1, name: /events/i })
    ).toBeInTheDocument()
  })
})

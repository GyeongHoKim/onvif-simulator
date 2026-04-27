import { describe, it, expect, beforeEach } from "vitest"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { AuthScreen } from "./Auth"
import { useSim } from "@/store/simulator"
import { appMocks, defaultConfig, defaultStatus, defaultUsers } from "@/test/wails-mock"

beforeEach(() => {
  useSim.setState({
    status: defaultStatus(),
    config: defaultConfig(),
    users: defaultUsers(),
    log: [],
  })
})

describe("Auth screen", () => {
  it("lists users from the auth store", () => {
    render(<AuthScreen />)
    expect(screen.getByText("admin")).toBeInTheDocument()
    expect(screen.getByText("Administrator")).toBeInTheDocument()
  })

  it("auth toggle calls SetAuthEnabled with the new value", async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)
    const toggle = screen.getByRole("switch")
    await user.click(toggle)
    await waitFor(() => expect(appMocks.SetAuthEnabled).toHaveBeenCalled())
  })

  it("Add user dialog calls AddUser with the form values", async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)
    await user.click(screen.getByRole("button", { name: /add user/i }))
    const dialog = await screen.findByRole("dialog")
    const textboxes = within(dialog).getAllByRole("textbox")
    // Form order: Username (textbox role), Password (type=password — not in
    // textbox role and queryable only by selector).
    await user.type(textboxes[0], "alice")
    const pwd = dialog.querySelector('input[type="password"]') as HTMLInputElement
    expect(pwd).not.toBeNull()
    await user.type(pwd, "wonderland")
    await user.click(within(dialog).getByRole("button", { name: /^add user$/i }))

    await waitFor(() => expect(appMocks.AddUser).toHaveBeenCalled())
    expect(appMocks.AddUser.mock.calls[0][0]).toMatchObject({
      username: "alice",
      password: "wonderland",
      role: "Administrator",
    })
  })

  it("delete confirm dialog removes the user", async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)
    await user.click(screen.getByRole("button", { name: /^delete$/i }))
    const confirm = await screen.findByRole("dialog")
    await user.click(within(confirm).getByRole("button", { name: /^delete$/i }))
    await waitFor(() => expect(appMocks.RemoveUser).toHaveBeenCalledWith("admin"))
  })

  it("shows the auth-off banner when authentication is disabled", () => {
    const cfg = defaultConfig()
    cfg.auth.enabled = false
    useSim.setState({ config: cfg })
    render(<AuthScreen />)
    expect(screen.getByText(/auth off/i)).toBeInTheDocument()
  })

  it("renders empty state when there are no users", () => {
    useSim.setState({ users: [] })
    render(<AuthScreen />)
    expect(screen.getByText(/no users/i)).toBeInTheDocument()
  })

  it("password reveal toggle switches the input type", async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)
    await user.click(screen.getByRole("button", { name: /add user/i }))
    const dialog = await screen.findByRole("dialog")
    const pwd = dialog.querySelector('input[type="password"]') as HTMLInputElement
    expect(pwd).not.toBeNull()
    await user.click(within(dialog).getByRole("button", { name: /show password/i }))
    const visible = dialog.querySelector('input[type="text"]:not([role])') as HTMLInputElement
    expect(visible).not.toBeNull()
  })

  it("backend AddUser errors surface inline in the dialog", async () => {
    const user = userEvent.setup()
    appMocks.AddUser.mockRejectedValueOnce(new Error("config: username taken"))
    render(<AuthScreen />)
    await user.click(screen.getByRole("button", { name: /add user/i }))
    const dialog = await screen.findByRole("dialog")
    const textboxes = within(dialog).getAllByRole("textbox")
    await user.type(textboxes[0], "alice")
    const pwd = dialog.querySelector('input[type="password"]') as HTMLInputElement
    await user.type(pwd, "pw")
    await user.click(within(dialog).getByRole("button", { name: /^add user$/i }))
    expect(await within(dialog).findByText(/username taken/i)).toBeInTheDocument()
  })

  it("renders DigestJWTPanels with safe fallbacks when auth is undefined", () => {
    // Force the panels' optional-chain fallback branches.
    const cfg = defaultConfig()
    cfg.auth.digest = undefined
    cfg.auth.jwt = undefined
    useSim.setState({ config: cfg })
    render(<AuthScreen />)
    expect(screen.getByText(/digest \/ jwt/i)).toBeInTheDocument()
  })

  it("Cancel on add user dialog closes without dispatching", async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)
    await user.click(screen.getByRole("button", { name: /add user/i }))
    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByRole("button", { name: /^cancel$/i }))
    expect(appMocks.AddUser).not.toHaveBeenCalled()
  })

  it("Cancel on delete user dialog leaves user list intact", async () => {
    const user = userEvent.setup()
    render(<AuthScreen />)
    await user.click(screen.getByRole("button", { name: /^delete$/i }))
    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByRole("button", { name: /^cancel$/i }))
    expect(appMocks.RemoveUser).not.toHaveBeenCalled()
  })
})

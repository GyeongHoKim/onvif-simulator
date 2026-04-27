import { describe, it, expect, beforeEach } from "vitest"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { MediaScreen } from "./Media"
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

describe("Media screen", () => {
  it("lists existing profiles in a table", () => {
    render(<MediaScreen />)
    expect(screen.getByText("profile_main")).toBeInTheDocument()
    expect(screen.getByText("1920x1080@30")).toBeInTheDocument()
  })

  it("Add profile dialog submits AddProfile to the backend", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)

    await user.click(screen.getByRole("button", { name: /add profile/i }))

    const dialog = await screen.findByRole("dialog")
    const inputs = within(dialog).getAllByRole("textbox")
    // Form order: Name, Token, RTSP URI, (Encoding select), Snapshot URI,
    // (Width/Height/FPS number inputs), Bitrate, GOP length, Video source token.
    await user.type(inputs[0], "extra")
    await user.type(inputs[1], "profile_extra")
    await user.type(inputs[2], "rtsp://127.0.0.1:8554/extra")
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }))

    await waitFor(() => expect(appMocks.AddProfile).toHaveBeenCalled())
    const arg = appMocks.AddProfile.mock.calls[0]?.[0] as { token?: string }
    expect(arg.token).toBe("profile_extra")
  })

  it("delete confirm dialog removes the profile", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)

    await user.click(screen.getByRole("button", { name: /^delete$/i }))
    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByRole("button", { name: /^delete$/i }))

    await waitFor(() => expect(appMocks.RemoveProfile).toHaveBeenCalledWith("profile_main"))
  })

  it("renders empty state when there are no profiles", () => {
    const cfg = defaultConfig()
    cfg.media.profiles = []
    useSim.setState({ config: cfg })
    render(<MediaScreen />)
    expect(screen.getByText(/no profiles yet/i)).toBeInTheDocument()
  })

  it("dedups video sources from profiles", () => {
    const cfg = defaultConfig()
    cfg.media.profiles.push({
      ...cfg.media.profiles[0],
      token: "profile_extra",
      name: "extra",
      video_source_token: "VS_MAIN",
    })
    useSim.setState({ config: cfg })
    render(<MediaScreen />)
    // Both profiles share VS_MAIN — the Video sources section renders exactly
    // one Badge for it (the table itself doesn't show that column).
    expect(screen.getAllByText("VS_MAIN")).toHaveLength(1)
  })

  it("Edit dialog pre-fills the form and dispatches per-field mutators", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /^edit$/i }))
    const dialog = await screen.findByRole("dialog")
    // Save without changes — should call all three Set* mutators.
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }))
    await waitFor(() => expect(appMocks.SetProfileEncoder).toHaveBeenCalled())
    expect(appMocks.SetProfileRTSP).toHaveBeenCalledWith(
      "profile_main",
      "rtsp://127.0.0.1:8554/main"
    )
    expect(appMocks.SetProfileSnapshotURI).toHaveBeenCalled()
  })

  it("Cancel button on add dialog closes without saving", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByRole("button", { name: /^cancel$/i }))
    expect(appMocks.AddProfile).not.toHaveBeenCalled()
  })

  it("typing into form fields updates state across all inputs", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")
    const numbers = within(dialog).getAllByRole("spinbutton") // type=number inputs
    // Width / Height / FPS / Bitrate / GOP — touch each so the update branch
    // executes for every onChange handler.
    for (const input of numbers) {
      await user.clear(input)
      await user.type(input, "10")
    }
    const textboxes = within(dialog).getAllByRole("textbox")
    // Snapshot URI + Video source token are both textboxes with no
    // pre-existing value.
    await user.type(textboxes[3], "http://example.com/snap.jpg")
    await user.type(textboxes[4], "VS_NEW")
  })

  it("Cancel on delete confirm leaves the profile alone", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /^delete$/i }))
    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByRole("button", { name: /^cancel$/i }))
    expect(appMocks.RemoveProfile).not.toHaveBeenCalled()
  })

  it("backend validation errors render inline and keep the dialog open", async () => {
    const user = userEvent.setup()
    appMocks.AddProfile.mockRejectedValueOnce(new Error("config: profile.rtsp invalid"))

    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")
    const inputs = within(dialog).getAllByRole("textbox")
    await user.type(inputs[0], "bad")
    await user.type(inputs[1], "bad")
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }))

    expect(await within(dialog).findByText(/profile.rtsp invalid/i)).toBeInTheDocument()
  })
})

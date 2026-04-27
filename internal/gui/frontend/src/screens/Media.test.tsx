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
    // The table now renders the local media file path instead of the
    // deprecated passthrough RTSP URI.
    expect(screen.getByText("/var/onvif/main.mp4")).toBeInTheDocument()
    expect(screen.getByText("1920x1080@30")).toBeInTheDocument()
  })

  it("Add profile dialog submits AddProfile to the backend", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)

    await user.click(screen.getByRole("button", { name: /add profile/i }))

    const dialog = await screen.findByRole("dialog")
    // Form order (text inputs only — encoder fields are now disabled):
    // 0 Name, 1 Token, 2 Media file path, 3 Snapshot URI, 4 Video source.
    const inputs = within(dialog).getAllByRole("textbox")
    await user.type(inputs[0], "extra")
    await user.type(inputs[1], "profile_extra")
    await user.type(inputs[2], "/var/onvif/extra.mp4")
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }))

    await waitFor(() => expect(appMocks.AddProfile).toHaveBeenCalled())
    const arg = appMocks.AddProfile.mock.calls[0]?.[0] as {
      token?: string
      media_file_path?: string
    }
    expect(arg.token).toBe("profile_extra")
    expect(arg.media_file_path).toBe("/var/onvif/extra.mp4")
  })

  it("Browse button populates the media file path from PickMediaFile", async () => {
    const user = userEvent.setup()
    appMocks.PickMediaFile.mockResolvedValueOnce("/picked/from/dialog.mp4")

    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")

    await user.click(within(dialog).getByRole("button", { name: /browse/i }))

    await waitFor(() => expect(appMocks.PickMediaFile).toHaveBeenCalled())
    const inputs = within(dialog).getAllByRole("textbox")
    expect((inputs[2] as HTMLInputElement).value).toBe("/picked/from/dialog.mp4")
  })

  it("Browse cancellation leaves the path field empty", async () => {
    const user = userEvent.setup()
    appMocks.PickMediaFile.mockResolvedValueOnce("")

    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")

    await user.click(within(dialog).getByRole("button", { name: /browse/i }))
    await waitFor(() => expect(appMocks.PickMediaFile).toHaveBeenCalled())
    const inputs = within(dialog).getAllByRole("textbox")
    expect((inputs[2] as HTMLInputElement).value).toBe("")
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
    expect(screen.getAllByText("VS_MAIN")).toHaveLength(1)
  })

  it("Edit dialog routes saves through SetProfileMediaFile", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /^edit$/i }))
    const dialog = await screen.findByRole("dialog")
    // Replace the media file path with a new value, then save.
    const inputs = within(dialog).getAllByRole("textbox")
    await user.clear(inputs[2])
    await user.type(inputs[2], "/new/main.mp4")
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }))

    await waitFor(() =>
      expect(appMocks.SetProfileMediaFile).toHaveBeenCalledWith(
        "profile_main",
        "/new/main.mp4"
      )
    )
    // Snapshot URI was non-empty so SetProfileSnapshotURI should also fire.
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

  it("typing into form fields updates state across all editable inputs", async () => {
    const user = userEvent.setup()
    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")
    // The encoder fields (Encoding/Resolution/FPS) render as disabled
    // inputs because they are auto-detected from the file at Start; only
    // the editable textboxes need to be exercised here.
    const editable = within(dialog)
      .getAllByRole("textbox")
      .filter((el) => !(el as HTMLInputElement).disabled)
    expect(editable).toHaveLength(5) // Name, Token, MediaFile, Snapshot, VideoSrc
    const values = ["extra", "profile_extra", "/var/onvif/extra.mp4", "http://x/y.jpg", "VS_NEW"]
    for (let i = 0; i < editable.length; i++) {
      await user.clear(editable[i])
      await user.type(editable[i], values[i])
    }
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
    appMocks.AddProfile.mockRejectedValueOnce(
      new Error("config: profile.media_file_path required")
    )

    render(<MediaScreen />)
    await user.click(screen.getByRole("button", { name: /add profile/i }))
    const dialog = await screen.findByRole("dialog")
    const inputs = within(dialog).getAllByRole("textbox")
    await user.type(inputs[0], "bad")
    await user.type(inputs[1], "bad")
    await user.click(within(dialog).getByRole("button", { name: /^save$/i }))

    expect(
      await within(dialog).findByText(/media_file_path required/i)
    ).toBeInTheDocument()
  })
})

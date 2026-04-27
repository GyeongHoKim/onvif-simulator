# Frontend (Wails GUI)

## Overview

Users pick **Device**, **Media**, or **Event** in the **sidebar**, then edit settings in the **main panel** (forms for Device/Media, actions such as buttons for Event). It mirrors the root `README.md` TUI: virtual device info, stream URIs, and event triggers—presented as a graphical shell.

## Component hierarchy

```
ThemeProvider
└── TooltipProvider
    ├── Toaster
    └── App
        └── AppShell
            ├── SidebarProvider · Sidebar … navigate Device | Media | Event
            └── SettingsPanel
                └── one active section only:
                    ├── DeviceSettingsForm
                    ├── MediaSettingsForm
                    └── EventActionsPanel
```

Shared controls live under `src/components/ui/` (shadcn). Helpers use `src/lib/utils.ts`. Wails generates the Go bridge under **`src/lib/wails/`** (see `wailsjsdir` in `wails.json`).

## Data sent through Wails

Go **public methods** on the struct you pass to `Bind` are generated as TypeScript functions. The frontend calls them to send **configuration and commands**. Every call returns a **Promise**—use `async`/`await` as usual.

| Screen | What you pass to the backend (conceptually) |
|--------|---------------------------------------------|
| **Device** | Virtual device metadata exposed to ONVIF (manufacturer, model, firmware, etc.—match the actual Go method signatures). |
| **Media** | **Stream URIs** for main/sub (and profile or token fields if your API needs them). |
| **Event** | Test event type (e.g. motion) and any extra arguments your API defines. |

Typical imports look like `@/lib/wails/go/main/App`; the `go/...` path segment depends on your Go package and struct names. Regenerate bindings from the repo root with **`wails dev`** (during development) or **`wails generate module`**. See [How does it work](https://wails.io/docs/howdoesitwork) and the [CLI reference](https://wails.io/docs/reference/cli).

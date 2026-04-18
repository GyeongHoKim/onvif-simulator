import * as React from "react"
import { Toaster as Sonner, type ToasterProps } from "sonner"
import {
  RiCheckboxCircleLine,
  RiCloseCircleLine,
  RiErrorWarningLine,
  RiInformationLine,
  RiLoaderLine,
} from "@remixicon/react"

import { useTheme } from "@/components/theme-provider.tsx"

function useResolvedSonnerTheme(): ToasterProps["theme"] {
  const { theme } = useTheme()

  const getSnapshot = React.useCallback((): ToasterProps["theme"] => {
    if (theme === "dark" || theme === "light") {
      return theme
    }

    if (typeof window === "undefined") {
      return "light"
    }

    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light"
  }, [theme])

  return React.useSyncExternalStore(
    (onStoreChange) => {
      if (theme !== "system") {
        return () => {}
      }

      const media = window.matchMedia("(prefers-color-scheme: dark)")
      media.addEventListener("change", onStoreChange)

      return () => {
        media.removeEventListener("change", onStoreChange)
      }
    },
    getSnapshot,
    getSnapshot
  )
}

const Toaster = ({ ...props }: ToasterProps) => {
  const resolvedTheme = useResolvedSonnerTheme()

  return (
    <Sonner
      theme={resolvedTheme}
      className="toaster group"
      icons={{
        success: <RiCheckboxCircleLine className="size-4" />,
        info: <RiInformationLine className="size-4" />,
        warning: <RiErrorWarningLine className="size-4" />,
        error: <RiCloseCircleLine className="size-4" />,
        loading: <RiLoaderLine className="size-4 animate-spin" />,
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: "cn-toast",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }

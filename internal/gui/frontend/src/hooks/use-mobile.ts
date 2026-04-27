import * as React from "react"

const MOBILE_BREAKPOINT = 768

const MOBILE_QUERY = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

function getIsMobile() {
  return window.matchMedia(MOBILE_QUERY).matches
}

export function useIsMobile() {
  return React.useSyncExternalStore(
    (onStoreChange) => {
      const mql = window.matchMedia(MOBILE_QUERY)
      mql.addEventListener("change", onStoreChange)

      return () => {
        mql.removeEventListener("change", onStoreChange)
      }
    },
    getIsMobile,
    () => false
  )
}

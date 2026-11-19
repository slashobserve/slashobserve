import { useEffect } from "react"
import { useRouter } from "@tanstack/react-router"

type EntryState = { __TSR_key?: string } | null

const documentEntry = (window.history.state as EntryState)?.__TSR_key
let checked = false

export function withoutEditor(href: string): string {
  const url = new URL(href, "http://x")
  url.searchParams.delete("panel")
  url.searchParams.delete("edit")
  return `${url.pathname}${url.search}${url.hash}`
}

export function useEditorEntryInDocument() {
  const router = useRouter()
  useEffect(() => {
    if (checked) return
    checked = true
    const { href, state } = router.history.location
    if (documentEntry === undefined || state.__TSR_key !== documentEntry) {
      return
    }
    router.history.replace(withoutEditor(href), state, { ignoreBlocker: true })
    // The browser history batches writes made in one task into one entry, so the push waits for the replace to land.
    window.setTimeout(
      () => router.history.push(href, undefined, { ignoreBlocker: true }),
      0
    )
  }, [router])
}

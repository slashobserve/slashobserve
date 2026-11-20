import { useNavigate } from "@tanstack/react-router"

import { askLink } from "@/lib/links"

import { useSendExample } from "./chat-composer"
import type { Example } from "./examples"

export function useChooseExample() {
  const navigate = useNavigate()
  const { send, pending } = useSendExample()

  function choose(example: Example) {
    if (example.dashboardId === undefined) {
      void send(example.prompt)
      return
    }
    void navigate(askLink(example.prompt, example.dashboardId))
  }

  return { choose, pending }
}

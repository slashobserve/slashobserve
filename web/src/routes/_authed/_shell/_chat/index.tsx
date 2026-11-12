import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/_shell/_chat/")({
  beforeLoad: ({ location }) => {
    throw redirect({
      href: location.searchStr === "" ? "/inbox" : `/ask${location.searchStr}`,
      replace: true,
    })
  },
})

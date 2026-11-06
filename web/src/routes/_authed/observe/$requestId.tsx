import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/observe/$requestId")({
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/c/$requestId",
      params,
      replace: true,
      statusCode: 301,
    })
  },
})

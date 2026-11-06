import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/builds/$requestId")({
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/b/$requestId",
      params,
      replace: true,
      statusCode: 301,
    })
  },
})

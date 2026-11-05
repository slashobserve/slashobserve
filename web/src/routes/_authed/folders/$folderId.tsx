import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/folders/$folderId")({
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/f/$folderId",
      params,
      replace: true,
      statusCode: 301,
    })
  },
})

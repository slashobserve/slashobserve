import type { ReactNode } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { parseSearch, stringifySearch } from "@/lib/search"

type Options = {
  queryClient?: QueryClient
  route?: { path: string; url: string }
}

export function testQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  })
}

function memoryRouter(ui: ReactNode, route: { path: string; url: string }) {
  const root = createRootRoute()
  const page = createRoute({
    getParentRoute: () => root,
    path: route.path,
    component: () => ui,
  })
  return createRouter({
    routeTree: root.addChildren([page]),
    history: createMemoryHistory({ initialEntries: [route.url] }),
    parseSearch,
    stringifySearch,
  })
}

export function renderWithProviders(ui: ReactNode, options: Options = {}) {
  const queryClient = options.queryClient ?? testQueryClient()
  const router = options.route ? memoryRouter(ui, options.route) : null
  const user = userEvent.setup()
  const result = render(
    <QueryClientProvider client={queryClient}>
      {router ? <RouterProvider router={router} /> : ui}
    </QueryClientProvider>
  )
  return { ...result, user, queryClient, router }
}

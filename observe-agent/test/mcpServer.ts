import { Server } from "@modelcontextprotocol/sdk/server/index.js"
import { WebStandardStreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/webStandardStreamableHttp.js"
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js"

export type SeenCall = {
  name: string
  args: unknown
  meta: Record<string, unknown> | undefined
  authorization: string | null
}

export type ToolAnswer = {
  text: string
  structured?: Record<string, unknown>
  isError?: boolean
  meta?: Record<string, unknown>
}

export type FakeServer = {
  url: URL
  port: number
  calls: SeenCall[]
  stop: () => void
}

export function startMcpServer(
  token: string,
  answer: (call: SeenCall) => ToolAnswer | Promise<ToolAnswer>,
  port = 0
): FakeServer {
  const calls: SeenCall[] = []
  const server = Bun.serve({
    port,
    async fetch(request) {
      const authorization = request.headers.get("authorization")
      if (authorization !== `Bearer ${token}`) {
        return new Response("unauthorized", { status: 401 })
      }
      const mcp = new Server(
        { name: "fake", version: "0" },
        { capabilities: { tools: {} } }
      )
      mcp.setRequestHandler(ListToolsRequestSchema, () => ({
        tools: [
          {
            name: "lint_query",
            description: "Checks a query.",
            inputSchema: { type: "object", properties: {} },
          },
        ],
      }))
      mcp.setRequestHandler(CallToolRequestSchema, async (call) => {
        const seen: SeenCall = {
          name: call.params.name,
          args: call.params.arguments,
          meta: call.params._meta as Record<string, unknown> | undefined,
          authorization,
        }
        calls.push(seen)
        const reply = await answer(seen)
        return {
          content: [{ type: "text" as const, text: reply.text }],
          ...(reply.structured === undefined
            ? {}
            : { structuredContent: reply.structured }),
          ...(reply.isError === undefined ? {} : { isError: reply.isError }),
          ...(reply.meta === undefined ? {} : { _meta: reply.meta }),
        }
      })
      const transport = new WebStandardStreamableHTTPServerTransport({
        enableJsonResponse: true,
      })
      await mcp.connect(transport)
      return transport.handleRequest(request)
    },
  })
  return {
    url: new URL(`http://127.0.0.1:${server.port}/mcp`),
    port: server.port ?? 0,
    calls,
    stop: () => void server.stop(true),
  }
}

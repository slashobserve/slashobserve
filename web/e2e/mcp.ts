import type { APIRequestContext } from "@playwright/test"

import { createServiceAccount, createServiceAccountKey } from "./api"

type ToolResult = {
  isError?: boolean
  structuredContent?: unknown
  content?: Array<{ type: string; text?: string }>
}

export async function mcpKey(
  request: APIRequestContext,
  accountName: string
): Promise<string> {
  const account = await createServiceAccount(request, accountName)
  const key = await createServiceAccountKey(request, account.id, "journey")
  return key.token
}

function parseBody(contentType: string, text: string): unknown {
  if (!contentType.includes("text/event-stream")) return JSON.parse(text)
  const data = text
    .split("\n")
    .filter((line) => line.startsWith("data: "))
    .at(-1)
  if (data === undefined) throw new Error(`no data frame in: ${text}`)
  return JSON.parse(data.slice("data: ".length))
}

export async function mcpTool(
  request: APIRequestContext,
  token: string,
  name: string,
  args: Record<string, unknown>
): Promise<unknown> {
  const response = await request.post("/mcp", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      Accept: "application/json, text/event-stream",
      "MCP-Protocol-Version": "2025-11-25",
    },
    data: {
      jsonrpc: "2.0",
      id: 1,
      method: "tools/call",
      params: {
        name,
        arguments: args,
        _meta: {
          "io.modelcontextprotocol/clientInfo": {
            name: "claude-code",
            version: "9",
          },
        },
      },
    },
  })
  const text = await response.text()
  if (response.status() !== 200) {
    throw new Error(
      `mcp ${name}: expected 200, got ${response.status()}: ${text}`
    )
  }
  const body = parseBody(response.headers()["content-type"] ?? "", text) as {
    result?: ToolResult
    error?: unknown
  }
  const result = body.result
  if (result === undefined || result.isError === true) {
    throw new Error(`mcp ${name} failed: ${text}`)
  }
  if (result.structuredContent !== undefined) return result.structuredContent
  return JSON.parse(result.content?.[0]?.text ?? "null")
}

import { afterEach, describe, expect, test } from "bun:test"
import { ClaimClient, McpClient, ProtocolError } from "../src/mcp"
import { ToolError } from "../src/protocol"
import { type FakeServer, startMcpServer } from "./mcpServer"

const token = "sot_fixture"
const running: FakeServer[] = []
const clients: McpClient[] = []

function serve(answer: Parameters<typeof startMcpServer>[1]): FakeServer {
  const server = startMcpServer(token, answer)
  running.push(server)
  return server
}

function connect(server: FakeServer, bearer = token): McpClient {
  const client = new McpClient(server.url, bearer)
  clients.push(client)
  return client
}

afterEach(async () => {
  await Promise.all(clients.splice(0).map((c) => c.close()))
  for (const server of running.splice(0)) {
    server.stop()
  }
})

describe("McpClient", () => {
  test("lists the tools of a session", async () => {
    const client = connect(serve(() => ({ text: "{}" })))
    const tools = await client.listTools(5000)
    expect(tools).toEqual([
      {
        name: "lint_query",
        description: "Checks a query.",
        inputSchema: { type: "object", properties: {} },
      },
    ])
  })

  test("sends the bearer token, the arguments and the turn key, and reads the run key back", async () => {
    const server = serve(() => ({
      text: '{"ok":true}\n\n<note>careful</note>',
      structured: { ok: true },
      meta: {
        "slashobserve.com/run": {
          settled: "saved",
          build: { saves: 1, panels: 2, planned_panels: 4 },
        },
      },
    }))
    const reply = await connect(server).callTool(
      "lint_query",
      { query: "up" },
      { id: "turn-1", calls: 2, saves: 1 },
      5000
    )
    expect(server.calls).toHaveLength(1)
    expect(server.calls[0]).toMatchObject({
      name: "lint_query",
      args: { query: "up" },
      authorization: `Bearer ${token}`,
    })
    expect(server.calls[0]?.meta?.["slashobserve.com/turn"]).toEqual({
      id: "turn-1",
      calls: 2,
      saves: 1,
    })
    expect(reply.content).toBe('{"ok":true}\n\n<note>careful</note>')
    expect(reply.structured).toEqual({ ok: true })
    expect(reply.isError).toBe(false)
    expect(reply.meta).toEqual({
      settled: "saved",
      build: { saves: 1, panels: 2, planned_panels: 4 },
    })
  })

  test("a call without a turn sends no turn key and a result without the run key reads as unsettled", async () => {
    const server = serve(() => ({ text: "{}" }))
    const reply = await connect(server).callTool(
      "lint_query",
      {},
      undefined,
      5000
    )
    expect(server.calls[0]?.meta?.["slashobserve.com/turn"]).toBeUndefined()
    expect(reply.meta).toEqual({ settled: null, build: null })
  })

  test("a tool error is a reply, not a throw", async () => {
    const server = serve(() => ({
      text: '{"code":"query_invalid","message":"bad"}',
      isError: true,
    }))
    const reply = await connect(server).callTool(
      "lint_query",
      {},
      undefined,
      5000
    )
    expect(reply.isError).toBe(true)
    expect(reply.content).toContain("query_invalid")
  })

  test("a call that runs past its timeout throws", async () => {
    const server = serve(() => new Promise(() => undefined))
    await expect(
      connect(server).callTool("lint_query", {}, undefined, 100)
    ).rejects.toThrow()
  })

  test("a call aborted by its signal throws", async () => {
    const server = serve(() => new Promise(() => undefined))
    const controller = new AbortController()
    const pending = connect(server).callTool(
      "lint_query",
      {},
      undefined,
      5000,
      controller.signal
    )
    setTimeout(() => controller.abort(), 50)
    await expect(pending).rejects.toThrow()
  })

  test("a transport failure drops the connection so the next call reconnects", async () => {
    const first = serve(() => ({ text: "{}" }))
    const client = connect(first)
    await client.callTool("lint_query", {}, undefined, 5000)
    first.stop()
    await expect(
      client.callTool("lint_query", {}, undefined, 1000)
    ).rejects.toThrow()
    const second = startMcpServer(
      token,
      () => ({ text: '{"back":true}' }),
      first.port
    )
    running.push(second)
    const reply = await client.callTool("lint_query", {}, undefined, 5000)
    expect(reply.content).toBe('{"back":true}')
    expect(second.calls).toHaveLength(1)
  })

  test("a wrong token fails to connect", async () => {
    const server = serve(() => ({ text: "{}" }))
    await expect(
      connect(server, "sot_wrong").callTool("lint_query", {}, undefined, 2000)
    ).rejects.toThrow()
  })
})

describe("ClaimClient", () => {
  const claimed = {
    run: { id: "run-1", attempt: 1, kind: "observe", dashboard_id: null },
    session: { url: "http://127.0.0.1:1/mcp", token: "sot_run" },
  }

  test("claim_run parses the structured answer", async () => {
    const server = serve(() => ({
      text: JSON.stringify(claimed),
      structured: claimed,
    }))
    const claim = new ClaimClient(connect(server))
    const answer = await claim.claimRun(30)
    expect(server.calls[0]?.name).toBe("claim_run")
    expect(server.calls[0]?.args).toEqual({ wait_seconds: 30 })
    expect(answer.run?.id).toBe("run-1")
    expect(answer.session?.token).toBe("sot_run")
  })

  test("an answer without structured content is read from the text", async () => {
    const server = serve(() => ({ text: JSON.stringify({ run: null }) }))
    const answer = await new ClaimClient(connect(server)).claimRun(0)
    expect(answer.run).toBeNull()
  })

  test("a claim tool error throws ToolError with the server's code", async () => {
    const server = serve(() => ({
      text: '{"code":"not_your_run","message":"that run is not yours"}',
      isError: true,
    }))
    const claim = new ClaimClient(connect(server))
    const failure = claim.reportRun({ run_id: "r", attempt: 1 })
    await expect(failure).rejects.toBeInstanceOf(ToolError)
    await expect(failure).rejects.toMatchObject({ code: "not_your_run" })
  })

  test("an answer in an unreadable shape throws ProtocolError", async () => {
    const server = serve(() => ({
      text: '{"unexpected":1}',
      structured: { unexpected: 1 },
    }))
    await expect(
      new ClaimClient(connect(server)).claimRun(0)
    ).rejects.toBeInstanceOf(ProtocolError)
  })

  test("arguments the server would refuse are refused before sending", () => {
    const server = serve(() => ({ text: "{}" }))
    const claim = new ClaimClient(connect(server))
    expect(() => claim.claimRun(31)).toThrow()
    expect(server.calls).toHaveLength(0)
  })

  test("openRun opens a client on the session url with the session token", async () => {
    const run = serve(() => ({ text: "{}" }))
    const tools = new ClaimClient(
      connect(serve(() => ({ text: "{}" })))
    ).openRun({
      url: run.url.href,
      token,
    })
    try {
      await tools.callTool("lint_query", {}, undefined, 5000)
      expect(run.calls[0]?.authorization).toBe(`Bearer ${token}`)
    } finally {
      await tools.close()
    }
  })
})

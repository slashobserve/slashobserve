import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js"
import { ErrorCode, McpError } from "@modelcontextprotocol/sdk/types.js"
import type { z } from "zod"
import {
  type BriefAnswer,
  type BriefInput,
  type ClaimAnswer,
  type FinishAnswer,
  type FinishInput,
  type ReportAnswer,
  type ReportInput,
  type RunMeta,
  type RunSessionOffer,
  type TurnMeta,
  briefAnswer,
  briefInput,
  claimAnswer,
  claimInput,
  finishAnswer,
  finishInput,
  noRunMeta,
  parseToolError,
  reportAnswer,
  reportInput,
  runMeta,
  runMetaKey,
  turnMetaKey,
} from "./protocol"
import type { ToolSpec } from "./session/types"
import { version } from "./version"

export const claimTimeoutMs = 60_000
export const reportTimeoutMs = 10_000

export type ToolReply = {
  content: string
  isError: boolean
  structured: unknown
  meta: RunMeta
}

export interface RunTools {
  listTools(timeoutMs: number, signal?: AbortSignal): Promise<ToolSpec[]>
  callTool(
    name: string,
    args: Record<string, unknown>,
    turn: TurnMeta | undefined,
    timeoutMs: number,
    signal?: AbortSignal
  ): Promise<ToolReply>
  close(): Promise<void>
}

export interface ClaimApi {
  claimRun(waitSeconds: number, signal?: AbortSignal): Promise<ClaimAnswer>
  getRunBrief(input: BriefInput, signal?: AbortSignal): Promise<BriefAnswer>
  reportRun(input: ReportInput, signal?: AbortSignal): Promise<ReportAnswer>
  finishRun(input: FinishInput, signal?: AbortSignal): Promise<FinishAnswer>
  openRun(session: RunSessionOffer): RunTools
}

export class ProtocolError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "ProtocolError"
  }
}

type Connected = { client: Client }

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function isTransportFailure(error: unknown, signal?: AbortSignal): boolean {
  if (signal?.aborted) {
    return false
  }
  if (error instanceof McpError) {
    return (
      error.code === ErrorCode.RequestTimeout ||
      error.code === ErrorCode.ConnectionClosed
    )
  }
  return true
}

export class McpClient implements RunTools {
  private readonly url: URL
  private readonly token: string
  private connection: Promise<Connected> | undefined

  constructor(url: URL, token: string) {
    this.url = url
    this.token = token
  }

  private connect(): Promise<Connected> {
    if (this.connection === undefined) {
      const attempt = this.open()
      this.connection = attempt
      attempt.catch(() => {
        if (this.connection === attempt) {
          this.connection = undefined
        }
      })
    }
    return this.connection
  }

  private async open(): Promise<Connected> {
    const transport = new StreamableHTTPClientTransport(this.url, {
      requestInit: { headers: { Authorization: `Bearer ${this.token}` } },
    })
    const client = new Client(
      { name: "observe-agent", version },
      { capabilities: {} }
    )
    try {
      await client.connect(transport)
    } catch (error) {
      await transport.close().catch(() => undefined)
      throw error
    }
    return { client }
  }

  private drop(attempt: Promise<Connected>): void {
    if (this.connection === attempt) {
      this.connection = undefined
      void attempt.then((c) => c.client.close()).catch(() => undefined)
    }
  }

  private async use<T>(
    run: (client: Client) => Promise<T>,
    signal?: AbortSignal
  ): Promise<T> {
    const attempt = this.connect()
    try {
      const { client } = await attempt
      return await run(client)
    } catch (error) {
      if (isTransportFailure(error, signal)) {
        this.drop(attempt)
      }
      throw error
    }
  }

  async listTools(
    timeoutMs: number,
    signal?: AbortSignal
  ): Promise<ToolSpec[]> {
    const specs: ToolSpec[] = []
    let cursor: string | undefined
    do {
      const page = await this.use(
        (client) =>
          client.listTools(cursor === undefined ? {} : { cursor }, {
            timeout: timeoutMs,
            ...(signal === undefined ? {} : { signal }),
          }),
        signal
      )
      for (const tool of page.tools) {
        specs.push({
          name: tool.name,
          description: tool.description ?? "",
          inputSchema: tool.inputSchema,
        })
      }
      cursor = page.nextCursor
    } while (cursor !== undefined)
    return specs
  }

  async callTool(
    name: string,
    args: Record<string, unknown>,
    turn: TurnMeta | undefined,
    timeoutMs: number,
    signal?: AbortSignal
  ): Promise<ToolReply> {
    const result = await this.use(
      (client) =>
        client.callTool(
          {
            name,
            arguments: args,
            ...(turn === undefined ? {} : { _meta: { [turnMetaKey]: turn } }),
          },
          undefined,
          { timeout: timeoutMs, ...(signal === undefined ? {} : { signal }) }
        ),
      signal
    )
    const blocks = Array.isArray(result.content) ? result.content : []
    const content = blocks
      .map((block) => (block.type === "text" ? block.text : ""))
      .join("")
    const meta = runMeta.safeParse(result._meta?.[runMetaKey])
    return {
      content,
      isError: result.isError === true,
      structured: result.structuredContent,
      meta: meta.success ? meta.data : noRunMeta,
    }
  }

  async claimTool<S extends z.ZodType>(
    name: string,
    args: Record<string, unknown>,
    answer: S,
    timeoutMs: number,
    signal?: AbortSignal
  ): Promise<z.output<S>> {
    const reply = await this.callTool(name, args, undefined, timeoutMs, signal)
    if (reply.isError) {
      throw parseToolError(reply.content)
    }
    const parsed = answer.safeParse(
      reply.structured ?? parseJson(reply.content)
    )
    if (!parsed.success) {
      throw new ProtocolError(
        `${name} answered a shape observe-agent cannot read`
      )
    }
    return parsed.data
  }

  async close(): Promise<void> {
    const attempt = this.connection
    this.connection = undefined
    if (attempt !== undefined) {
      await attempt.then((c) => c.client.close()).catch(() => undefined)
    }
  }
}

export class ClaimClient implements ClaimApi {
  private readonly base: McpClient

  constructor(base: McpClient) {
    this.base = base
  }

  claimRun(waitSeconds: number, signal?: AbortSignal): Promise<ClaimAnswer> {
    return this.base.claimTool(
      "claim_run",
      claimInput.parse({ wait_seconds: waitSeconds }),
      claimAnswer,
      claimTimeoutMs,
      signal
    )
  }

  getRunBrief(input: BriefInput, signal?: AbortSignal): Promise<BriefAnswer> {
    return this.base.claimTool(
      "get_run_brief",
      briefInput.parse(input),
      briefAnswer,
      reportTimeoutMs,
      signal
    )
  }

  reportRun(input: ReportInput, signal?: AbortSignal): Promise<ReportAnswer> {
    return this.base.claimTool(
      "report_run",
      reportInput.parse(input),
      reportAnswer,
      reportTimeoutMs,
      signal
    )
  }

  finishRun(input: FinishInput, signal?: AbortSignal): Promise<FinishAnswer> {
    return this.base.claimTool(
      "finish_run",
      finishInput.parse(input),
      finishAnswer,
      reportTimeoutMs,
      signal
    )
  }

  openRun(session: RunSessionOffer): RunTools {
    return new McpClient(new URL(session.url), session.token)
  }
}

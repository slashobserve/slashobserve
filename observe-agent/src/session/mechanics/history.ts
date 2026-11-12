import type { AssistantModelMessage, ModelMessage, ToolModelMessage } from "ai"
import { SessionError, type ToolCall, type ToolResult } from "../types"
import {
  keptBytes,
  cutResult,
  prunedResult,
  utf8Bytes,
  type PruneMessage,
  type Pruner,
  type Weighed,
} from "./compact"

export const errorResultLabel = "ERROR: "

export type ErrorStyle = "flag" | "label"

type ToolResultPart = ToolModelMessage["content"][number] & {
  type: "tool-result"
}

type AssistantParts = Exclude<AssistantModelMessage["content"], string>

export function checkResults(
  provider: string,
  ended: boolean,
  pending: string[],
  results: ToolResult[]
): void {
  if (ended) {
    throw new SessionError(
      `${provider} session: the previous turn made no tool calls, so the session has ended`
    )
  }
  if (results.length !== pending.length) {
    throw new SessionError(
      `${provider} session: got ${results.length} tool results for ${pending.length} tool calls`
    )
  }
  for (const [i, result] of results.entries()) {
    if (result.callId !== pending[i]) {
      throw new SessionError(
        `${provider} session: tool result ${i} answers ${JSON.stringify(result.callId)}, want ${JSON.stringify(pending[i])}`
      )
    }
  }
}

export function userMessage(text: string): ModelMessage {
  return { role: "user", content: [{ type: "text", text }] }
}

function toolResultPart(
  style: ErrorStyle,
  toolName: string,
  result: ToolResult
): ToolResultPart {
  const output =
    style === "flag"
      ? {
          type: result.isError ? ("error-text" as const) : ("text" as const),
          value: result.content,
        }
      : {
          type: "text" as const,
          value: result.isError
            ? errorResultLabel + result.content
            : result.content,
        }
  return {
    type: "tool-result",
    toolCallId: result.callId,
    toolName,
    output,
  }
}

export function toolMessage(
  style: ErrorStyle,
  calls: ToolCall[],
  results: ToolResult[]
): ToolModelMessage {
  const names = new Map(calls.map((call) => [call.id, call.name]))
  return {
    role: "tool",
    content: results.map((result) =>
      toolResultPart(style, names.get(result.callId) ?? "", result)
    ),
  }
}

export function assistantMessage(parts: AssistantParts): ModelMessage {
  return { role: "assistant", content: parts }
}

function resultParts(message: ModelMessage): ToolResultPart[] {
  if (message.role !== "tool") {
    return []
  }
  return message.content.filter(
    (part): part is ToolResultPart => part.type === "tool-result"
  )
}

function resultText(part: ToolResultPart): string {
  const output = part.output
  switch (output.type) {
    case "text":
    case "error-text":
      return output.value
    case "execution-denied":
      return output.reason ?? ""
    default:
      return JSON.stringify(output.value)
  }
}

function withResultText(part: ToolResultPart, text: string): ToolResultPart {
  const type = part.output.type === "error-text" ? "error-text" : "text"
  return { ...part, output: { type, value: text } }
}

function assistantParts(message: ModelMessage): AssistantParts {
  return message.role === "assistant" && typeof message.content !== "string"
    ? message.content
    : []
}

export function messageBytes(
  message: ModelMessage,
  resultBytes: number
): number {
  switch (message.role) {
    case "tool":
      return resultParts(message).reduce(
        (total, part) =>
          total + keptBytes(utf8Bytes(resultText(part)), resultBytes),
        0
      )
    case "assistant":
      return assistantParts(message).reduce((total, part) => {
        if (part.type === "text") {
          return total + utf8Bytes(part.text)
        }
        if (part.type === "tool-call") {
          return total + utf8Bytes(JSON.stringify(part.input))
        }
        return total
      }, 0)
    case "user":
      return typeof message.content === "string"
        ? utf8Bytes(message.content)
        : message.content.reduce(
            (total, part) =>
              part.type === "text" ? total + utf8Bytes(part.text) : total,
            0
          )
    default:
      return 0
  }
}

export function weigh(
  messages: ModelMessage[],
  resultBytes: number
): Weighed[] {
  return messages.map((message) => ({
    assistant: message.role === "assistant",
    bytes: messageBytes(message, resultBytes),
  }))
}

export function pruneView(messages: ModelMessage[]): PruneMessage[] {
  return messages.map((message) => ({
    assistant: message.role === "assistant",
    results: resultParts(message).map((part) => ({
      id: part.toolCallId,
      bytes: utf8Bytes(resultText(part)),
    })),
  }))
}

function stubbed(style: ErrorStyle, part: ToolResultPart): ToolResultPart {
  const labelled =
    style === "label" && resultText(part).startsWith(errorResultLabel)
  return withResultText(
    part,
    labelled ? errorResultLabel + prunedResult : prunedResult
  )
}

export function withStubs(
  messages: ModelMessage[],
  boundary: number,
  pruner: Pruner,
  style: ErrorStyle
): ModelMessage[] {
  if (boundary === 0) {
    return messages
  }
  return messages.map((message, i) => {
    if (
      i >= boundary ||
      message.role !== "tool" ||
      !resultParts(message).some((part) => pruner.prunable(part.toolCallId))
    ) {
      return message
    }
    return {
      ...message,
      content: message.content.map((part) =>
        part.type === "tool-result" && pruner.prunable(part.toolCallId)
          ? stubbed(style, part)
          : part
      ),
    }
  })
}

export function cutToolResults(
  message: ModelMessage,
  limit: number
): ModelMessage {
  if (message.role !== "tool") {
    return message
  }
  return {
    ...message,
    content: message.content.map((part) =>
      part.type === "tool-result"
        ? withResultText(part, cutResult(resultText(part), limit))
        : part
    ),
  }
}

export function withoutReasoning(message: ModelMessage): ModelMessage {
  if (message.role !== "assistant" || typeof message.content === "string") {
    return message
  }
  return {
    ...message,
    content: message.content.filter((part) => part.type !== "reasoning"),
  }
}

export function endsInPlainAnswer(messages: ModelMessage[]): boolean {
  const last = messages.at(-1)
  return (
    last !== undefined &&
    last.role === "assistant" &&
    !assistantParts(last).some((part) => part.type === "tool-call")
  )
}

export function isAssistant(message: ModelMessage): boolean {
  return message.role === "assistant"
}

import { SessionError, type ToolSpec } from "../types"

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function describe(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value)
}

export function parseToolSchema(spec: ToolSpec): Record<string, unknown> {
  const schema: unknown = spec.inputSchema
  if (!isRecord(schema)) {
    throw new SessionError(
      `tool "${spec.name}": input schema is not a JSON object`
    )
  }
  if ("type" in schema && schema["type"] !== "object") {
    throw new SessionError(
      `tool "${spec.name}": input schema type must be "object", got ${describe(schema["type"])}`
    )
  }
  const normalized: Record<string, unknown> = { ...schema, type: "object" }
  if ("properties" in normalized && !isRecord(normalized["properties"])) {
    throw new SessionError(
      `tool "${spec.name}": input schema properties must be an object`
    )
  }
  const required = normalized["required"]
  if (
    "required" in normalized &&
    !(
      Array.isArray(required) &&
      required.every((item) => typeof item === "string")
    )
  ) {
    throw new SessionError(
      `tool "${spec.name}": input schema required must be an array of strings`
    )
  }
  return normalized
}

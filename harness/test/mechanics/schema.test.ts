import { describe, expect, test } from "bun:test"
import { parseToolSchema } from "../../src/session/mechanics/schema"
import { SessionError } from "../../src/session/types"

const spec = (inputSchema: unknown) => ({
  name: "save",
  description: "",
  inputSchema: inputSchema as Record<string, unknown>,
})

describe("parseToolSchema", () => {
  test("an empty schema becomes an empty object schema", () => {
    expect(parseToolSchema(spec({}))).toEqual({ type: "object" })
  })

  test("a schema without a type gets type object and keeps the rest", () => {
    const schema = {
      properties: { title: { type: "string" } },
      required: ["title"],
    }
    expect(parseToolSchema(spec(schema))).toEqual({
      type: "object",
      ...schema,
    })
  })

  test("the input is not changed", () => {
    const schema = { properties: {} }
    parseToolSchema(spec(schema))
    expect(schema).toEqual({ properties: {} })
  })

  const rejected: Array<[string, unknown, string]> = [
    ["not an object", [], 'tool "save": input schema is not a JSON object'],
    ["null", null, 'tool "save": input schema is not a JSON object'],
    ["a string", "x", 'tool "save": input schema is not a JSON object'],
    [
      "a non-object type",
      { type: "string" },
      'tool "save": input schema type must be "object", got string',
    ],
    [
      "a non-string type",
      { type: 3 },
      'tool "save": input schema type must be "object", got 3',
    ],
    [
      "properties as an array",
      { properties: [] },
      'tool "save": input schema properties must be an object',
    ],
    [
      "properties as null",
      { properties: null },
      'tool "save": input schema properties must be an object',
    ],
    [
      "required as a string",
      { required: "a" },
      'tool "save": input schema required must be an array of strings',
    ],
    [
      "required with a number",
      { required: ["a", 1] },
      'tool "save": input schema required must be an array of strings',
    ],
  ]
  test.each(rejected)("rejects %s", (_name, schema, message) => {
    expect(() => parseToolSchema(spec(schema))).toThrow(SessionError)
    expect(() => parseToolSchema(spec(schema))).toThrow(message)
  })
})

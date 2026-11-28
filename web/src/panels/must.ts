import { expect } from "vitest"

export function must<T>(value: T | undefined): T {
  expect(value).toBeDefined()
  return value as T
}

export function defined<T>(value: T | undefined): asserts value is T {
  expect(value).toBeDefined()
}

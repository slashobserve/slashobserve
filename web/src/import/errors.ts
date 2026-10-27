import { ApiError, toApiError } from "@/api/errors"
import type { components } from "@/api/schema.gen"

import { TOO_LARGE_MESSAGE } from "./read-source"

export type ImportSourceDatasource =
  components["schemas"]["ImportSourceDatasource"]

export type Failure = {
  message: string
  retry: boolean
  datasources?: ImportSourceDatasource[]
  resetMapping?: boolean
}

export class DatasourcesUnresolvedError extends ApiError {
  readonly datasources: ImportSourceDatasource[]

  constructor(source: ApiError, datasources: ImportSourceDatasource[]) {
    super(source.status, source.code, source.message, source.issues)
    this.name = "DatasourcesUnresolvedError"
    this.datasources = datasources
  }
}

const serverTextCodes = new Set([
  "invalid_json",
  "not_a_dashboard",
  "unsupported_format",
  "too_many_panels",
])

export function toImportError(response: Response, body: unknown): ApiError {
  const error = toApiError(response, body)
  if (
    error.status === 422 &&
    error.code === "datasources_unresolved" &&
    typeof body === "object" &&
    body !== null &&
    "datasources" in body &&
    Array.isArray(body.datasources)
  ) {
    return new DatasourcesUnresolvedError(
      error,
      body.datasources as ImportSourceDatasource[]
    )
  }
  return error
}

export function previewFailure(error: unknown): Failure {
  if (error instanceof ApiError) {
    if (error.status === 413) {
      return { message: TOO_LARGE_MESSAGE, retry: false }
    }
    if (error.status === 422 && serverTextCodes.has(error.code)) {
      return { message: error.message, retry: false }
    }
    if (error.status === 422 && error.code === "invalid_mapping") {
      return { message: error.message, retry: false, resetMapping: true }
    }
  }
  return { message: "Could not read this dashboard. Try again.", retry: true }
}

export function importFailure(error: unknown): Failure {
  if (error instanceof DatasourcesUnresolvedError) {
    return {
      message: "Choose a datasource for each row first.",
      retry: false,
      datasources: error.datasources,
    }
  }
  if (error instanceof ApiError) {
    if (error.status === 429) {
      return {
        message: "Another import is still running. Try again in a moment.",
        retry: false,
      }
    }
    if (error.status === 413) {
      return { message: TOO_LARGE_MESSAGE, retry: false }
    }
    if (error.status === 422 && error.code === "invalid_spec") {
      const issue = error.issues[0]?.message.replace(/\.+$/, "")
      return {
        message: issue
          ? `The converted dashboard could not be saved: ${issue}.`
          : "The converted dashboard could not be saved.",
        retry: false,
      }
    }
  }
  return { message: "Could not import. Try again.", retry: false }
}

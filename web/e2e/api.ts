import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { APIRequestContext, APIResponse } from "@playwright/test"

export type Spec = Record<string, unknown> & {
  title: string
  panels: Array<Record<string, unknown>>
}

const baseURL =
  process.env.SLASHOBSERVE_E2E_BASE_URL ?? "http://localhost:18080"

export const mutationHeaders = {
  Origin: baseURL,
  "Content-Type": "application/json",
}

export function loadSpec(name: string): Spec {
  const text = readFileSync(join(import.meta.dirname, "data", `${name}.json`), {
    encoding: "utf8",
  })
  return JSON.parse(text) as Spec
}

export function uniqueTitle(base: string): string {
  const random = Math.random().toString(36).slice(2, 6).padEnd(4, "0")
  return `${base} ${Date.now().toString(36)}${random}`
}

async function expectStatus(
  response: APIResponse,
  expected: number,
  what: string
): Promise<unknown> {
  const text = await response.text()
  if (response.status() !== expected) {
    throw new Error(
      `${what}: expected ${expected}, got ${response.status()}: ${text}`
    )
  }
  return JSON.parse(text)
}

export async function createDashboard(
  request: APIRequestContext,
  spec: Spec,
  summary?: string
): Promise<{ id: string; version: number }> {
  const response = await request.post("/api/v1/dashboards", {
    headers: mutationHeaders,
    data: { spec, summary },
  })
  const body = (await expectStatus(response, 201, "create dashboard")) as {
    dashboard: { id: string }
    version: { version: number }
  }
  return { id: body.dashboard.id, version: body.version.version }
}

export async function addVersion(
  request: APIRequestContext,
  id: string,
  spec: Spec,
  parentVersion: number,
  summary = "E2E change"
): Promise<number> {
  const response = await request.post(`/api/v1/dashboards/${id}/versions`, {
    headers: mutationHeaders,
    data: { spec, summary, parent_version: parentVersion },
  })
  const body = (await expectStatus(response, 201, "add version")) as {
    version: { version: number }
  }
  return body.version.version
}

export async function getDashboard(
  request: APIRequestContext,
  id: string
): Promise<{ title: string; latestVersion: number; spec: Spec }> {
  const response = await request.get(`/api/v1/dashboards/${id}`)
  const body = (await expectStatus(response, 200, "get dashboard")) as {
    dashboard: { title: string; latest_version: number }
    version: { spec: Spec }
  }
  return {
    title: body.dashboard.title,
    latestVersion: body.dashboard.latest_version,
    spec: body.version.spec,
  }
}

export async function restoreVersion(
  request: APIRequestContext,
  id: string,
  version: number,
  parentVersion: number
): Promise<number> {
  const response = await request.post(
    `/api/v1/dashboards/${id}/versions/${version}/restore`,
    {
      headers: mutationHeaders,
      data: { parent_version: parentVersion },
    }
  )
  const body = (await expectStatus(response, 201, "restore version")) as {
    version: { version: number }
  }
  return body.version.version
}

export async function createComment(
  request: APIRequestContext,
  id: string,
  version: number,
  panelId: string,
  body: string
): Promise<string> {
  const response = await request.post(`/api/v1/dashboards/${id}/comments`, {
    headers: mutationHeaders,
    data: { version, panel_id: panelId, body },
  })
  const created = (await expectStatus(response, 201, "create comment")) as {
    comment: { id: string }
  }
  return created.comment.id
}

export async function requestRevision(
  request: APIRequestContext,
  id: string,
  prompt: string,
  baseVersion: number
): Promise<{ status: number; id?: string }> {
  const response = await request.post(`/api/v1/dashboards/${id}/revisions`, {
    headers: mutationHeaders,
    data: { prompt, base_version: baseVersion },
  })
  const status = response.status()
  if (status !== 202) return { status }
  const body = (await response.json()) as { revision: { id: string } }
  return { status, id: body.revision.id }
}

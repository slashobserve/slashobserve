import { describe, expect, test } from "bun:test"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { agentSettings } from "../src/protocol"
import {
  classify,
  full,
  parseTask,
  type Question,
  taskName,
  tasks,
  type Task,
} from "../src/route/route"
import { callsForTask, toolsForTask } from "../src/route/tools"

const fixtures = resolve(import.meta.dir, "fixtures", "route")
const taskTools = resolve(
  import.meta.dir,
  "..",
  "..",
  "spec",
  "examples",
  "agent",
  "route",
  "task-tools.json"
)

function fixture<T>(name: string): T {
  return JSON.parse(readFileSync(resolve(fixtures, name), "utf8")) as T
}

type RouteCases = {
  types: string[]
  classify: { text: string; want: Task }[]
  shape: { name: string; question: Question; want: Task }[]
}

type TaskToolsCases = {
  all: string[]
  cases: {
    task: Task
    subject: { dashboard: boolean; observation: boolean }
    want: string[]
  }[]
}

describe("classify", () => {
  const cases = fixture<RouteCases>("cases.json")

  test("picks the task type Go picks for each question", () => {
    expect(cases.classify.length).toBeGreaterThan(0)
    for (const c of cases.classify) {
      expect({
        text: c.text,
        task: classify({ types: cases.types, text: c.text }),
      }).toEqual({ text: c.text, task: c.want })
    }
  })

  test("reads the run's shape as Go does", () => {
    expect(cases.shape.length).toBeGreaterThan(0)
    for (const c of cases.shape) {
      expect({ name: c.name, task: classify(c.question) }).toEqual({
        name: c.name,
        task: c.want,
      })
    }
  })
})

describe("parseTask", () => {
  test("knows every task and refuses others", () => {
    const known: Task[] = [full, ...tasks]
    for (const task of known) {
      expect(parseTask(task)).toBe(task)
    }
    expect(parseTask("dashboard")).toBeUndefined()
    expect(taskName(full)).toBe("full")
    expect(taskName("measure")).toBe("measure")
  })
})

describe("toolsForTask", () => {
  const { all, cases } = JSON.parse(
    readFileSync(taskTools, "utf8")
  ) as TaskToolsCases

  test("offers each task type the tools the Go registry offers", () => {
    expect(cases.length).toBe(2 * (tasks.length + 1))
    for (const c of cases) {
      expect({
        task: c.task,
        subject: c.subject,
        tools: toolsForTask(c.task, all, c.subject),
      }).toEqual({ task: c.task, subject: c.subject, tools: c.want })
    }
  })

  test("never offers a tool the mode does not", () => {
    const some = ["measure_window", "save_finding"]
    expect(
      toolsForTask("measure", some, { dashboard: true, observation: true })
    ).toEqual(some)
  })
})

describe("callsForTask", () => {
  const agent = agentSettings.parse({
    default_calls: 30,
    task_calls: { measure: 16, incident: 30 },
  })

  test("reads the claim's per-task limits", () => {
    expect(callsForTask("measure", agent)).toBe(16)
    expect(callsForTask("logs", agent)).toBe(30)
    expect(callsForTask(full, agent)).toBe(30)
  })

  test("lets the configured limit win", () => {
    expect(callsForTask("measure", { ...agent, investigation_calls: 50 })).toBe(
      50
    )
  })
})

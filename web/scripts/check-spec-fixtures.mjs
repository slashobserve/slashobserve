import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { validateSpec } from "../src/spec/index.ts"

const examplesDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../spec/examples"
)
const readJSON = (file) => JSON.parse(fs.readFileSync(file, "utf8"))

const failures = []
let ok = 0
const record = (file, reason) => {
  if (reason) failures.push(`FAIL ${file}: ${reason}`)
  else ok++
}

const valid = fs
  .readdirSync(examplesDir)
  .filter((name) => name.endsWith(".json"))
  .sort()
for (const name of valid) {
  const result = validateSpec(readJSON(path.join(examplesDir, name)))
  record(
    name,
    result.valid ? "" : `expected valid, got ${JSON.stringify(result.issues)}`
  )
}

const invalidDir = path.join(examplesDir, "invalid")
const manifest = readJSON(path.join(invalidDir, "expected.json"))
for (const c of manifest.cases) {
  const result = validateSpec(readJSON(path.join(invalidDir, c.file)))
  if (c.layer === "schema") {
    if (result.valid) {
      record(c.file, "expected schema issues, got valid")
    } else if (!result.issues.some((issue) => issue.path === c.path)) {
      record(
        c.file,
        `no issue at ${JSON.stringify(c.path)}, got ${JSON.stringify(result.issues)}`
      )
    } else {
      record(c.file, "")
    }
  } else {
    record(
      c.file,
      result.valid
        ? ""
        : `semantic case must pass the schema, got ${JSON.stringify(result.issues)}`
    )
  }
}

if (failures.length > 0) {
  console.error(failures.join("\n"))
  process.exit(1)
}
console.log(`spec fixtures: ${ok} ok`)

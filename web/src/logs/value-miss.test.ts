import { describe, expect, it } from "vitest"

import { firstValueClause, valueMissText } from "./value-miss"

describe("firstValueClause", () => {
  it("takes the first field and value of a KQL query", () => {
    expect(
      firstValueClause(
        "elasticsearch",
        "service.name:checkout and log.level:error"
      )
    ).toEqual({ field: "service.name", value: "checkout" })
  })

  it("has nothing to say for a query the builder cannot show or another language", () => {
    expect(firstValueClause("elasticsearch", "a:1 or b:2")).toBeNull()
    expect(firstValueClause("loki", '{service_name="a"}')).toBeNull()
    expect(firstValueClause("elasticsearch", undefined)).toBeNull()
  })
})

describe("valueMissText", () => {
  const clause = { field: "log.level", value: "error" }

  it("lists the values seen when none matches", () => {
    expect(valueMissText({ ...clause, value: "fatal" }, ["INFO", "WARN"])).toBe(
      "No document has log.level = fatal in this range; values seen: INFO, WARN."
    )
  })

  it("says the values are case sensitive when only the letters differ", () => {
    expect(valueMissText(clause, ["INFO", "ERROR"])).toBe(
      "No document has log.level = error in this range, but it has ERROR; values are case sensitive, so seen: INFO, ERROR."
    )
  })

  it("stays silent when the value exists or nothing was seen", () => {
    expect(valueMissText(clause, ["error", "INFO"])).toBeNull()
    expect(valueMissText(clause, [])).toBeNull()
  })
})

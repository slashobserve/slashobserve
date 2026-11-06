import { useEffect, useState } from "react"

import { loadSpecValidator, type SpecValidator } from "@/spec"

let loaded: SpecValidator | undefined

export function useSpecValidator(): SpecValidator | undefined {
  const [validate, setValidate] = useState(() => loaded)
  useEffect(() => {
    if (validate) return
    let live = true
    void loadSpecValidator().then((next) => {
      loaded = next
      if (live) setValidate(() => next)
    })
    return () => {
      live = false
    }
  }, [validate])
  return validate
}

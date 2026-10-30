import { useEffect, useMemo, useState } from "react"

import type { Transform } from "@/spec"

import {
  buildGazetteer,
  type Gazetteer,
  type Gazetteers,
} from "../geomap/gazetteer/lookup"
import { atlasFeatures, type AtlasName } from "../geomap/world"
import { gazetteersNeeded } from "./location"

const built: Partial<Record<AtlasName, Promise<Gazetteer>>> = {}

function gazetteerFor(name: AtlasName): Promise<Gazetteer> {
  built[name] ??= atlasFeatures(name).then((features) =>
    buildGazetteer(name, features)
  )
  return built[name]
}

export function useGazetteers(
  transforms: readonly Transform[] | undefined
): Gazetteers {
  const names = useMemo(() => gazetteersNeeded(transforms), [transforms])
  const [loaded, setLoaded] = useState<Gazetteers>({})
  useEffect(() => {
    let live = true
    for (const name of names) {
      gazetteerFor(name)
        .then((gazetteer) => {
          if (live) setLoaded((current) => ({ ...current, [name]: gazetteer }))
        })
        .catch(() => undefined)
    }
    return () => {
      live = false
    }
  }, [names])
  return loaded
}

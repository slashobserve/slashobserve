import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel, Transform } from "@/spec"

import { calculateField } from "./calculate"
import { concatenate } from "./concatenate"
import { configFromData } from "./config-from-data"
import { convertFieldType } from "./convert-type"
import { extractFields } from "./extract-fields"
import { filterFieldsByName } from "./filter"
import { filterByRefId } from "./filter-ref"
import { filterByValue } from "./filter-value"
import { applyQueryFormats } from "./format"
import { formatString } from "./format-string"
import { formatTime } from "./format-time"
import { groupBy } from "./group-by"
import { groupToNestedTable } from "./group-to-nested-table"
import { groupingToMatrix } from "./grouping-to-matrix"
import { heatmap } from "./heatmap"
import { histogram } from "./histogram"
import { joinByField } from "./join"
import { joinByLabels } from "./join-by-labels"
import { labelsToFields } from "./labels-to-fields"
import { limit } from "./limit"
import { merge } from "./merge"
import { organize } from "./organize"
import { partitionByValues } from "./partition-by-values"
import { prepareTimeSeries } from "./prepare-time-series"
import { reduceFrames } from "./reduce"
import { regression } from "./regression"
import { renameByRegex } from "./rename-regex"
import { rowsToFields } from "./rows-to-fields"
import { stampSeriesNames, type DatasourceTypeOf } from "./series-names"
import { seriesToRows } from "./series-to-rows"
import { sortBy } from "./sort"
import { timeSeriesTable } from "./time-series-table"
import { transpose } from "./transpose"
import type { PreparedFrame } from "./names"
export type { DatasourceTypeOf } from "./series-names"

export {
  matchesName,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"

function applyTransform(
  frames: PreparedFrame[],
  transform: Transform
): PreparedFrame[] {
  switch (transform.type) {
    case "merge":
      return merge(frames)
    case "seriesToRows":
      return seriesToRows(frames)
    case "organize":
      return organize(frames, transform)
    case "reduce":
      return reduceFrames(frames, transform)
    case "joinByField":
      return joinByField(frames, transform)
    case "filterFieldsByName":
      return filterFieldsByName(frames, transform)
    case "calculateField":
      return calculateField(frames, transform)
    case "sortBy":
      return sortBy(frames, transform)
    case "limit":
      return limit(frames, transform)
    case "convertFieldType":
      return convertFieldType(frames, transform)
    case "formatString":
      return formatString(frames, transform)
    case "formatTime":
      return formatTime(frames, transform)
    case "renameByRegex":
      return renameByRegex(frames, transform)
    case "filterByValue":
      return filterByValue(frames, transform)
    case "filterByRefId":
      return filterByRefId(frames, transform)
    case "extractFields":
      return extractFields(frames, transform)
    case "labelsToFields":
      return labelsToFields(frames, transform)
    case "joinByLabels":
      return joinByLabels(frames, transform)
    case "partitionByValues":
      return partitionByValues(frames, transform)
    case "prepareTimeSeries":
      return prepareTimeSeries(frames, transform)
    case "timeSeriesTable":
      return timeSeriesTable(frames, transform)
    case "rowsToFields":
      return rowsToFields(frames, transform)
    case "configFromData":
      return configFromData(frames, transform)
    case "concatenate":
      return concatenate(frames, transform)
    case "groupBy":
      return groupBy(frames, transform)
    case "groupingToMatrix":
      return groupingToMatrix(frames, transform)
    case "groupToNestedTable":
      return groupToNestedTable(frames, transform)
    case "heatmap":
      return heatmap(frames, transform)
    case "histogram":
      return histogram(frames, transform)
    case "regression":
      return regression(frames, transform)
    case "transpose":
      return transpose(frames, transform)
  }
}

export function applyTransforms(
  frames: PreparedFrame[],
  transforms: readonly Transform[] | undefined
): PreparedFrame[] {
  return (transforms ?? []).reduce(applyTransform, frames)
}

export function prepareFrames(
  panel: DataPanel,
  frames: WireFrame[],
  datasourceTypeOf?: DatasourceTypeOf
): PreparedFrame[] {
  return applyTransforms(
    applyQueryFormats(panel, stampSeriesNames(panel, frames, datasourceTypeOf)),
    panel.transform
  )
}

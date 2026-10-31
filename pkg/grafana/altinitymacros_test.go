package grafana

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	altinityRef   = `{"type":"vertamedia-clickhouse-datasource","uid":"ch"}`
	altinityFrame = `"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":` + altinityRef

	epoch      = "toInt64(toUnixTimestamp(t))"
	sinceTick  = epoch + " - lagInFrame(" + epoch + ", 1, " + epoch + " - $__interval_s) OVER w"
	sinceStart = epoch + " - lagInFrame(" + epoch + ", 1, " + epoch + ") OVER w"
	tickWindow = " WINDOW w AS (ORDER BY t) ORDER BY t"
	dateFilter = "WHERE $__dateTimeFilter(d, ts)"
)

func altinityDoc(query, targetMembers, panelMembers string, variables ...string) string {
	templatingPart := ""
	if len(variables) > 0 {
		templatingPart = templating(variables...) + ","
	}
	if panelMembers != "" {
		panelMembers += ","
	}
	target := fmt.Sprintf(`{"refId":"A","query":%q,"datasource":%s,"database":"db","table":"t","dateTimeColDataType":"ts","dateColDataType":"d"%s}`, query, altinityRef, targetMembers)
	return fmt.Sprintf(`{"title":"T","schemaVersion":39,%s"panels":[{%s,"type":"timeseries",%s"targets":[%s]}]}`, templatingPart, altinityFrame, panelMembers, target)
}

func altinityPanel(t *testing.T, query, targetMembers string, variables ...string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, altinityDoc(query, targetMembers, "", variables...))
}

func altinityQuery(t *testing.T, query, targetMembers string, variables ...string) string {
	t.Helper()
	panel, line := altinityPanel(t, query, targetMembers, variables...)
	if len(panel.Queries) != 1 {
		t.Fatalf("%s: queries = %+v, line = %+v", query, panel.Queries, line.Queries)
	}
	return panel.Queries[0].Query
}

func counterWindow(partition string) string {
	return " WINDOW w AS (" + partition + "ORDER BY t)"
}

func TestAltinityCounterMacrosExpandToWindowSQL(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{
			"rate",
			"$rate(count() c) FROM db.t",
			"SELECT t, c / (" + sinceTick + ") AS cRate FROM (SELECT $__timeInterval(ts) AS t, count() c\nFROM db.t\n" + dateFilter + "\nGROUP BY t ORDER BY t)" + tickWindow,
		},
		{
			"perSecond",
			"$perSecond(x, y) FROM db.t",
			"SELECT t, if((max_0 - lagInFrame(max_0, 1, max_0) OVER w) < 0, nan, (max_0 - lagInFrame(max_0, 1, max_0) OVER w) / (" + sinceStart + ")) AS max_0_PerSecond, " +
				"if((max_1 - lagInFrame(max_1, 1, max_1) OVER w) < 0, nan, (max_1 - lagInFrame(max_1, 1, max_1) OVER w) / (" + sinceStart + ")) AS max_1_PerSecond " +
				"FROM (SELECT $__timeInterval(ts) AS t, toFloat64(max(x)) AS max_0, toFloat64(max(y)) AS max_1\nFROM db.t\n" + dateFilter + "\nGROUP BY t ORDER BY t)" + tickWindow,
		},
		{
			"delta",
			"$delta(x) FROM db.t",
			"SELECT t, max_0 - lagInFrame(max_0, 1, max_0) OVER w AS max_0_Delta FROM (SELECT $__timeInterval(ts) AS t, toFloat64(max(x)) AS max_0\nFROM db.t\n" + dateFilter + "\nGROUP BY t ORDER BY t)" + tickWindow,
		},
		{
			"increase",
			"$increase(x) FROM db.t",
			"SELECT t, if((max_0 - lagInFrame(max_0, 1, max_0) OVER w) < 0, 0, max_0 - lagInFrame(max_0, 1, max_0) OVER w) AS max_0_Increase FROM (SELECT $__timeInterval(ts) AS t, toFloat64(max(x)) AS max_0\nFROM db.t\n" + dateFilter + "\nGROUP BY t ORDER BY t)" + tickWindow,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := altinityQuery(t, tc.query, ""); got != tc.want {
				t.Fatalf("query = %q\nwant    %q", got, tc.want)
			}
		})
	}
}

func TestAltinityColumnsMacrosExpandToLongRows(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{
			"columns",
			"$columns(k, sum(a) c) FROM $table",
			"SELECT t, toString(k) AS k, c AS Value FROM (SELECT $__timeInterval(ts) AS t, k, sum(a) c\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY t, k) ORDER BY t, k",
		},
		{
			"columnsMs",
			"$columnsMs(k, count() c) FROM $table",
			"SELECT t, toString(k) AS k, c AS Value FROM (SELECT $__timeInterval_ms(ts) AS t, k, count() c\nFROM db.t\nWHERE $__timeFilter_ms(ts)\nGROUP BY t, k\nORDER BY t, k) ORDER BY t, k",
		},
		{
			"rateColumns",
			"$rateColumns(k, count() c) FROM db.t",
			"SELECT t, toString(k) AS k, c / (" + sinceTick + ") AS Value FROM (SELECT $__timeInterval(ts) AS t, k, count() c\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY t, k)" + counterWindow("PARTITION BY k ") + " ORDER BY t, k",
		},
		{
			"perSecondColumns",
			"$perSecondColumns(k, x) FROM db.t",
			"SELECT t, toString(k) AS k, if((max_0 - lagInFrame(max_0, 1, max_0) OVER w) < 0, nan, (max_0 - lagInFrame(max_0, 1, max_0) OVER w) / (" + sinceStart + ")) AS Value " +
				"FROM (SELECT $__timeInterval(ts) AS t, k, toFloat64(max(x)) AS max_0\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY k, t)" + counterWindow("PARTITION BY k ") + " ORDER BY t, k",
		},
		{
			"increaseColumns",
			"$increaseColumns(k, x) FROM db.t",
			"SELECT t, toString(k) AS k, if((max_0 - lagInFrame(max_0, 1, max_0) OVER w) < 0, 0, max_0 - lagInFrame(max_0, 1, max_0) OVER w) AS Value " +
				"FROM (SELECT $__timeInterval(ts) AS t, k, toFloat64(max(x)) AS max_0\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY k, t)" + counterWindow("PARTITION BY k ") + " ORDER BY t, k",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := altinityQuery(t, tc.query, ""); got != tc.want {
				t.Fatalf("query = %q\nwant    %q", got, tc.want)
			}
		})
	}
}

func TestAltinityAggregatedMacrosUseThreeLayers(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{
			"rateColumnsAggregated",
			"$rateColumnsAggregated(k, 'sub' AS sk, sum, x, max, y) FROM db.t",
			"SELECT t, toString(k) AS k, sum(xRate) AS xRateAgg, max(yRate) AS yRateAgg FROM (SELECT t, k, sk, x / (" + sinceTick + ") AS xRate, y / (" + sinceTick + ") AS yRate " +
				"FROM (SELECT $__timeInterval(ts) AS t, k, 'sub' AS sk, max(x) AS x, max(y) AS y\nFROM db.t\n" + dateFilter + "\nGROUP BY k, sk, t\nORDER BY k, sk, t)" + counterWindow("PARTITION BY k, sk ") + ") GROUP BY k, t ORDER BY t, k",
		},
		{
			"perSecondColumnsAggregated",
			"$perSecondColumnsAggregated(k, 'sub' AS sk, sum, x) FROM db.t",
			"SELECT t, toString(k) AS k, sum(xPerSecond) AS xPerSecondAgg FROM (SELECT t, k, sk, if((x - lagInFrame(x, 1, x) OVER w) < 0, nan, (x - lagInFrame(x, 1, x) OVER w) / (" + sinceStart + ")) AS xPerSecond " +
				"FROM (SELECT $__timeInterval(ts) AS t, k, 'sub' AS sk, toFloat64(max(x)) AS x\nFROM db.t\n" + dateFilter + "\nGROUP BY k, sk, t\nORDER BY k, sk, t)" + counterWindow("PARTITION BY k, sk ") + ") GROUP BY k, t ORDER BY t, k",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := altinityQuery(t, tc.query, ""); got != tc.want {
				t.Fatalf("query = %q\nwant    %q", got, tc.want)
			}
		})
	}
}

func TestAltinityLTTBExpandsToLargestTriangleThreeBuckets(t *testing.T) {
	const source = "FROM (SELECT $timeSeries AS t, count() AS c FROM $table GROUP BY t)"
	fixed := altinityQuery(t, "$lttb(100, t, c) "+source, "")
	wantFixed := "SELECT point.1 AS t, point.2 AS c FROM (SELECT arrayJoin(largestTriangleThreeBuckets(100)(t, c)) AS point\nFROM (SELECT $__timeInterval(ts) AS t, count() AS c FROM db.t GROUP BY t)\n" + dateFilter + "\n) ORDER BY t"
	if fixed != wantFixed {
		t.Fatalf("query = %q\nwant    %q", fixed, wantFixed)
	}

	auto := altinityQuery(t, "$lttb(auto, k, t, c) FROM db.t GROUP BY k", "")
	wantAuto := "SELECT point.1 AS t, k, point.2 AS c FROM (SELECT k, arrayJoin(largestTriangleThreeBuckets(toUInt64((${__to:date:seconds} - ${__from:date:seconds}) / $__interval_s))(t, c)) AS point\nFROM db.t\n" + dateFilter + "\nGROUP BY k) ORDER BY t"
	if auto != wantAuto {
		t.Fatalf("query = %q\nwant    %q", auto, wantAuto)
	}
}

func TestAltinityMacroArgumentsKeepNestedParenthesesAndQuotedCommas(t *testing.T) {
	got := altinityQuery(t, "$columns(concat(a, ', b') AS k, sum(if(x IN (1, 2), 1, 0)) c) FROM $table", "")
	want := "SELECT t, toString(k) AS k, c AS Value FROM (SELECT $__timeInterval(ts) AS t, concat(a, ', b') AS k, sum(if(x IN (1, 2), 1, 0)) c\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY t, k) ORDER BY t, k"
	if got != want {
		t.Fatalf("query = %q\nwant    %q", got, want)
	}
}

func TestAltinityUseWindowFuncForMacrosDoesNotChangeTheExpansion(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			panel, line := altinityPanel(t, "$rate(count() c) FROM db.t", `,"useWindowFuncForMacros":`+value)
			if !strings.Contains(panel.Queries[0].Query, "lagInFrame") || strings.Contains(panel.Queries[0].Query, "runningDifference") {
				t.Fatalf("query = %q, want the window function form", panel.Queries[0].Query)
			}
			for _, path := range line.Paths {
				if strings.Contains(path, "useWindowFuncForMacros") {
					t.Fatalf("paths = %v, want the option consumed silently", line.Paths)
				}
			}
		})
	}
}

func TestAltinityWholeQueryRewriteLineRestoresTheGrafanaText(t *testing.T) {
	const query = "$columns(k, sum(a) c)\nFROM $table WHERE $timeFilter"
	panel, line := altinityPanel(t, query, "")
	rewrites := line.Queries[0].Rewrites
	if len(rewrites) != 1 || rewrites[0] != query+" -> "+panel.Queries[0].Query {
		t.Fatalf("rewrites = %q, want the Grafana text and the Spec text in one line", rewrites)
	}
}

func TestAltinityMalformedWholeQueryMacrosKeepTheirDropReason(t *testing.T) {
	cases := []struct{ name, query string }{
		{"missing closing parenthesis", "$rate(count() c FROM db.t"},
		{"no source after the call", "$rate(count() c)"},
		{"no arguments", "$columns() FROM db.t"},
		{"not at the start of a query", "SELECT 1 FROM (SELECT 2) WHERE x IN ($rate(count() c) FROM db.t)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertDoc(t, altinityDoc(tc.query, "", ""))
			line := panelLine(t, res, "p1")
			if line.Status != statusPlaceholder {
				t.Fatalf("status = %s, want placeholder: %+v", line.Status, line)
			}
			dropped := line.Queries[0].Dropped
			if dropped == nil || !strings.Contains(*dropped, "builds the whole query and is not supported") {
				t.Fatalf("dropped = %v, want the whole-query reason", dropped)
			}
		})
	}
}

func TestAltinityTimeFilterGoesAfterTheWhereAndGuardsOr(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{
			"no where",
			"$columns(k, count() c) FROM $table",
			"FROM db.t\nWHERE $__dateTimeFilter(d, ts)\nGROUP BY t, k",
		},
		{
			"where with or",
			"$rate(count() c) FROM t WHERE a = 1 OR b = 2",
			"FROM t WHERE $__dateTimeFilter(d, ts) AND ( a = 1 OR b = 2\n)",
		},
		{
			"time filter already present",
			"$columns(k, count() c) FROM $table WHERE $timeFilter AND x = 1",
			"FROM db.t WHERE $__dateTimeFilter(d, ts) AND x = 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := altinityQuery(t, tc.query, "")
			if !strings.Contains(got, tc.want) {
				t.Fatalf("query = %q\nwant it to contain %q", got, tc.want)
			}
			if strings.Count(got, "$__dateTimeFilter") != 1 {
				t.Fatalf("query = %q, want exactly one time filter", got)
			}
		})
	}
}

func hasPathSuffix(paths []string, suffix string) bool {
	for _, path := range paths {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

const (
	fromRounded = "intDiv(${__from:date:seconds}, 60) * 60"
	toRounded   = "intDiv(${__to:date:seconds} + 60 - 1, 60) * 60"
)

func TestAltinityRoundWritesTheRoundedRangeIntoTheTimeFilter(t *testing.T) {
	const query = "SELECT $timeSeries AS t, count() FROM $table WHERE $timeFilter GROUP BY t"
	rangeFilter := "(ts >= toDateTime(" + fromRounded + ") AND ts <= toDateTime(" + toRounded + "))"
	dateRange := "(d >= toDate(toDateTime(" + fromRounded + ")) AND d <= toDate(toDateTime(" + toRounded + ")))"
	cases := []struct{ name, members, want string }{
		{"no rounding", `,"round":"0s"`, "SELECT $__timeInterval(ts) AS t, count() FROM db.t WHERE $__dateTimeFilter(d, ts) GROUP BY t"},
		{"one minute", `,"round":"1m"`, "SELECT $__timeInterval(ts) AS t, count() FROM db.t WHERE (" + dateRange + " AND " + rangeFilter + ") GROUP BY t"},
		{
			"five minutes",
			`,"round":"5m"`,
			"SELECT $__timeInterval(ts) AS t, count() FROM db.t WHERE ((d >= toDate(toDateTime(intDiv(${__from:date:seconds}, 300) * 300)) AND d <= toDate(toDateTime(intDiv(${__to:date:seconds} + 300 - 1, 300) * 300))) AND " +
				"(ts >= toDateTime(intDiv(${__from:date:seconds}, 300) * 300) AND ts <= toDateTime(intDiv(${__to:date:seconds} + 300 - 1, 300) * 300))) GROUP BY t",
		},
		{
			"the step",
			`,"round":"$step"`,
			"SELECT $__timeInterval(ts) AS t, count() FROM db.t WHERE ((d >= toDate(toDateTime(intDiv(${__from:date:seconds}, $__interval_s) * $__interval_s)) AND d <= toDate(toDateTime(intDiv(${__to:date:seconds} + $__interval_s - 1, $__interval_s) * $__interval_s))) AND " +
				"(ts >= toDateTime(intDiv(${__from:date:seconds}, $__interval_s) * $__interval_s) AND ts <= toDateTime(intDiv(${__to:date:seconds} + $__interval_s - 1, $__interval_s) * $__interval_s))) GROUP BY t",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := altinityPanel(t, query, tc.members)
			if got := panel.Queries[0].Query; got != tc.want {
				t.Fatalf("query = %q\nwant    %q", got, tc.want)
			}
			if hasPathSuffix(line.Paths, ".round") {
				t.Fatalf("paths = %v, want round consumed", line.Paths)
			}
		})
	}
}

func TestAltinityRoundRewritesFromAndToAndTheFilterByColumn(t *testing.T) {
	got := altinityQuery(t, "SELECT $from, $to FROM t WHERE $timeFilterByColumn(created_at)", `,"round":"1m"`)
	want := "SELECT (" + fromRounded + "), (" + toRounded + ") FROM t WHERE (created_at >= toDateTime(" + fromRounded + ") AND created_at <= toDateTime(" + toRounded + "))"
	if got != want {
		t.Fatalf("query = %q\nwant    %q", got, want)
	}
}

func TestAltinityRoundThatIsNotAWholeNumberOfSecondsIsReported(t *testing.T) {
	for _, round := range []string{"soon", "1500ms", "-1m"} {
		t.Run(round, func(t *testing.T) {
			panel, line := altinityPanel(t, "SELECT 1 FROM t WHERE $timeFilter", `,"round":"`+round+`"`)
			if !hasPathSuffix(line.Paths, ".round") {
				t.Fatalf("paths = %v, want round reported", line.Paths)
			}
			if got := panel.Queries[0].Query; got != "SELECT 1 FROM t WHERE $__dateTimeFilter(d, ts)" {
				t.Fatalf("query = %q, want the unrounded filter", got)
			}
		})
	}
}

func TestAltinityDateTimeTypeWrapsTheTimeColumn(t *testing.T) {
	const query = "SELECT $timeSeries AS t, count() FROM $table WHERE $timeFilter GROUP BY t"
	unixFilter := func(low, high string) string {
		return "WHERE ($__dateFilter(d) AND (ts >= " + low + " AND ts <= " + high + ")) GROUP BY t"
	}
	cases := []struct{ kind, series, filter string }{
		{"DATETIME", "$__timeInterval(ts)", "WHERE $__dateTimeFilter(d, ts) GROUP BY t"},
		{"DATETIME64", "$__timeInterval_ms(ts)", "WHERE $__dateTimeFilter(d, ts) GROUP BY t"},
		{"TIMESTAMP", "toStartOfInterval(toDateTime(ts), INTERVAL $__interval_s second)", unixFilter("$__from / 1000", "$__to / 1000")},
		{"FLOAT", "toStartOfInterval(toDateTime(ts), INTERVAL $__interval_s second)", unixFilter("$__from / 1000", "$__to / 1000")},
		{"TIMESTAMP64_3", "toStartOfInterval(toDateTime(fromUnixTimestamp64Milli(toInt64(ts))), INTERVAL $__interval_s second)", unixFilter("$__from", "$__to")},
		{"TIMESTAMP64_6", "toStartOfInterval(toDateTime(fromUnixTimestamp64Micro(toInt64(ts))), INTERVAL $__interval_s second)", unixFilter("$__from * 1000", "$__to * 1000")},
		{"TIMESTAMP64_9", "toStartOfInterval(toDateTime(fromUnixTimestamp64Nano(toInt64(ts))), INTERVAL $__interval_s second)", unixFilter("$__from * 1000000", "$__to * 1000000")},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			panel, line := altinityPanel(t, query, `,"dateTimeType":"`+tc.kind+`"`)
			want := "SELECT " + tc.series + " AS t, count() FROM db.t " + tc.filter
			if got := panel.Queries[0].Query; got != want {
				t.Fatalf("query = %q\nwant    %q", got, want)
			}
			if line.Status != statusImported {
				t.Fatalf("line = %+v, want imported", line)
			}
		})
	}
}

func TestAltinityMillisecondMacrosFollowTheTimeColumnType(t *testing.T) {
	cases := []struct{ kind, want string }{
		{"DATETIME", "SELECT $__timeInterval_ms(ts) AS t FROM t WHERE $__timeFilter_ms(ts)"},
		{"TIMESTAMP", "SELECT toStartOfInterval(toDateTime64(ts, 3), INTERVAL $__interval_ms millisecond) AS t FROM t WHERE (ts >= $__from / 1000 AND ts <= $__to / 1000)"},
		{"TIMESTAMP64_6", "SELECT toStartOfInterval(fromUnixTimestamp64Micro(toInt64(ts)), INTERVAL $__interval_ms millisecond) AS t FROM t WHERE (ts >= $__from * 1000 AND ts <= $__to * 1000)"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			got := altinityQuery(t, "SELECT $timeSeriesMs AS t FROM t WHERE $timeFilterMs", `,"dateTimeType":"`+tc.kind+`"`)
			if got != tc.want {
				t.Fatalf("query = %q\nwant    %q", got, tc.want)
			}
		})
	}
}

func TestAltinityUnknownDateTimeTypeIsReportedAndReadAsDateTime(t *testing.T) {
	panel, line := altinityPanel(t, "SELECT $timeSeries AS t FROM t", `,"dateTimeType":"DATE"`)
	if !hasPathSuffix(line.Paths, ".dateTimeType") {
		t.Fatalf("paths = %v, want dateTimeType reported", line.Paths)
	}
	if got := panel.Queries[0].Query; got != "SELECT $__timeInterval(ts) AS t FROM t" {
		t.Fatalf("query = %q", got)
	}
}

func TestAltinityTimeFilter64ByColumnUsesTheMillisecondFilter(t *testing.T) {
	got := altinityQuery(t, "SELECT 1 FROM t WHERE $timeFilter64ByColumn(created_at) AND x = 1", "")
	if want := "SELECT 1 FROM t WHERE $__timeFilter_ms(created_at) AND x = 1"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestAltinityNaturalTimeSeriesBucketsByCalendarUnitsForLongRanges(t *testing.T) {
	got := altinityQuery(t, "SELECT $naturalTimeSeries AS t, count() FROM t GROUP BY t", "")
	want := "SELECT multiIf($__range_s >= 94608000, toDateTime(toStartOfYear(ts)), $__range_s >= 15552000, toDateTime(toStartOfMonth(ts)), " +
		"$__range_s >= 2592000, toDateTime(toStartOfWeek(ts, 1)), $__range_s >= 604800, toStartOfDay(ts), $__timeInterval(ts)) AS t, count() FROM t GROUP BY t"
	if got != want {
		t.Fatalf("query = %q\nwant    %q", got, want)
	}
}

func TestAltinityLTTBMsReadsTheMillisecondSeries(t *testing.T) {
	got := altinityQuery(t, "$lttbMs(100, t, c) FROM (SELECT $timeSeriesMs AS t, count() AS c FROM $table GROUP BY t)", "")
	want := "SELECT point.1 AS t, point.2 AS c FROM (SELECT arrayJoin(largestTriangleThreeBuckets(100)(t, c)) AS point\nFROM (SELECT $__timeInterval_ms(ts) AS t, count() AS c FROM db.t GROUP BY t)\nWHERE $__timeFilter_ms(ts)\n) ORDER BY t"
	if got != want {
		t.Fatalf("query = %q\nwant    %q", got, want)
	}
}

func TestAltinityRateColumnsAcceptsAValueWithoutAnAlias(t *testing.T) {
	got := altinityQuery(t, "$rateColumns(k, count()) FROM db.t", "")
	want := "SELECT t, toString(k) AS k, value / (" + sinceTick + ") AS Value FROM (SELECT $__timeInterval(ts) AS t, k, count() AS value\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY t, k)" + counterWindow("PARTITION BY k ") + " ORDER BY t, k"
	if got != want {
		t.Fatalf("query = %q\nwant    %q", got, want)
	}
}

func TestAltinityRateFamilyExtrapolatesByDefaultAndDropsTheFirstBucketWhenOff(t *testing.T) {
	first := "(row_number() OVER w) = 1"
	cases := []struct{ name, query, want string }{
		{
			"rate",
			"$rate(count() c) FROM db.t",
			"SELECT t, if(" + first + ", nan, c / (" + sinceTick + ")) AS cRate FROM (SELECT $__timeInterval(ts) AS t, count() c\nFROM db.t\n" + dateFilter + "\nGROUP BY t ORDER BY t)" + tickWindow,
		},
		{
			"delta",
			"$delta(x) FROM db.t",
			"SELECT t, if(" + first + ", nan, max_0 - lagInFrame(max_0, 1, max_0) OVER w) AS max_0_Delta " +
				"FROM (SELECT $__timeInterval(ts) AS t, toFloat64(max(x)) AS max_0\nFROM db.t\n" + dateFilter + "\nGROUP BY t ORDER BY t)" + tickWindow,
		},
		{
			"rateColumns",
			"$rateColumns(k, count() c) FROM db.t",
			"SELECT t, toString(k) AS k, if(" + first + ", nan, c / (" + sinceTick + ")) AS Value FROM (SELECT $__timeInterval(ts) AS t, k, count() c\nFROM db.t\n" + dateFilter + "\nGROUP BY t, k\nORDER BY t, k)" + counterWindow("PARTITION BY k ") + " ORDER BY t, k",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			off, line := altinityPanel(t, tc.query, `,"extrapolate":false`)
			if got := off.Queries[0].Query; got != tc.want {
				t.Fatalf("query = %q\nwant    %q", got, tc.want)
			}
			if hasPathSuffix(line.Paths, ".extrapolate") {
				t.Fatalf("paths = %v, want extrapolate consumed", line.Paths)
			}
			for _, members := range []string{"", `,"extrapolate":true`} {
				on, line := altinityPanel(t, tc.query, members)
				if strings.Contains(on.Queries[0].Query, "row_number") || hasPathSuffix(line.Paths, ".extrapolate") {
					t.Fatalf("members %q: query = %q, paths = %v, want the extrapolated form with nothing reported", members, on.Queries[0].Query, line.Paths)
				}
			}
		})
	}
}

func TestAltinityAdhocInsideAQueryNeedsExactlyOneAdhocVariable(t *testing.T) {
	const adhoc = `{"name":"filters","type":"adhoc","datasource":` + altinityRef + `}`
	const second = `{"name":"more","type":"adhoc","datasource":` + altinityRef + `}`
	cases := []struct {
		name      string
		variables []string
		want      string
		reported  bool
	}{
		{"one variable", []string{adhoc}, "SELECT 1 WHERE $filters", false},
		{"no variable", nil, "SELECT 1 WHERE 1", false},
		{"two variables", []string{adhoc, second}, "SELECT 1 WHERE 1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, altinityDoc("SELECT 1 WHERE $adhoc", "", "", tc.variables...))
			if got := panel.Queries[0].Query; got != tc.want {
				t.Fatalf("query = %q, want %q", got, tc.want)
			}
			if reported := hasPathSuffix(line.Paths, ".query"); reported != tc.reported {
				t.Fatalf("paths = %v, want reported = %v", line.Paths, tc.reported)
			}
		})
	}
}

func TestAltinityTimeFilterByColumnBecomesTheTimeFilterMacro(t *testing.T) {
	got := altinityQuery(t, "SELECT 1 FROM t WHERE $timeFilterByColumn(created_at) AND x = 1", "")
	if want := "SELECT 1 FROM t WHERE $__timeFilter(created_at) AND x = 1"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	res := convertDoc(t, altinityDoc("SELECT 1 FROM t WHERE $timeFilterByColumn(a + b)", "", ""))
	line := panelLine(t, res, "p1")
	if line.Status != statusPlaceholder || line.Queries[0].Dropped == nil {
		t.Fatalf("line = %+v, want the malformed call dropped", line)
	}
}

func TestClickHouseBuilderOptionsDropSilentlyWhenRawSQLIsKept(t *testing.T) {
	const chRef = `{"type":"grafana-clickhouse-datasource","uid":"ch"}`
	doc := fmt.Sprintf(`{"title":"T","schemaVersion":41,"panels":[{"id":1,"type":"timeseries","title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":%s,"targets":[{"refId":"A","datasource":%s,"editorType":"builder","rawSql":"SELECT timestamp, value FROM metrics WHERE $__timeFilter(timestamp)","builderOptions":{"database":"otel","table":"metrics","queryType":"timeseries","mode":"trend","settings":{"max_threads":"4"},"limit":100}}]}]}`, chRef, chRef)
	panel, line := convertedPanel(t, doc)
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths = %v, want imported with no paths", line.Status, line.Paths)
	}
	if len(panel.Queries) != 1 || strings.Contains(panel.Queries[0].Query, "max_threads") {
		t.Fatalf("queries = %+v", panel.Queries)
	}
}

func TestClickHouseMinIntervalBecomesTheQueryInterval(t *testing.T) {
	const chRef = `{"type":"grafana-clickhouse-datasource","uid":"ch"}`
	const interval = `{"name":"res","type":"interval","query":"1m,10m","current":{"text":"1m","value":"1m"},"options":[]}`
	cases := []struct {
		name, members, want string
		reported            bool
	}{
		{"a duration", `,"minInterval":"1m"`, "1m", false},
		{"a built-in interval", `,"minInterval":"$__interval"`, "", false},
		{"an interval variable", `,"minInterval":"$res"`, "${res}", false},
		{"empty", `,"minInterval":""`, "", false},
		{"unreadable", `,"minInterval":"soon"`, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := fmt.Sprintf(`{"title":"T","schemaVersion":41,%s,"panels":[{"id":1,"type":"timeseries","title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":%s,"targets":[{"refId":"A","datasource":%s,"rawSql":"SELECT timestamp, value FROM metrics WHERE $__timeFilter(timestamp)"%s}]}]}`,
				templating(interval), chRef, chRef, tc.members)
			panel, line := convertedPanel(t, doc)
			if got := panel.Queries[0].Interval; got != tc.want {
				t.Fatalf("interval = %q, want %q", got, tc.want)
			}
			if reported := hasPathSuffix(line.Paths, ".minInterval"); reported != tc.reported {
				t.Fatalf("paths = %v, want reported = %v", line.Paths, tc.reported)
			}
		})
	}
}

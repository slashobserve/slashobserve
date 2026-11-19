package clickhouse

import (
	"math/big"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTableFrameKeepsEveryDigitOfAnIntegerAbove2To53(t *testing.T) {
	rs := &resultSet{
		columns: []column{{name: "id", typ: datasource.FieldNumber}, {name: "n", typ: datasource.FieldNumber}},
		rows:    [][]any{{int64(9007199254740993), int64(1)}, {uint64(18446744073709551615), int64(2)}, {nil, int64(3)}},
	}

	frame := rs.tableFrame()

	if got := frame.Fields[0]; got.Type != datasource.FieldString || got.Values[0] != "9007199254740993" || got.Values[1] != "18446744073709551615" || got.Values[2] != nil {
		t.Fatalf("id field = %+v", got)
	}
	if got := frame.Fields[1]; got.Type != datasource.FieldNumber || got.Values[0] != 1.0 {
		t.Fatalf("n field = %+v, want a number", got)
	}
}

func TestTableFrameKeepsAWideIntegerFromABigInt(t *testing.T) {
	huge, _ := new(big.Int).SetString("340282366920938463463374607431768211455", 10)
	rs := &resultSet{columns: []column{{name: "id", typ: datasource.FieldNumber}}, rows: [][]any{{huge}}}

	if got := rs.tableFrame().Fields[0]; got.Type != datasource.FieldString || got.Values[0] != huge.String() {
		t.Fatalf("id field = %+v", got)
	}
}

func TestTableFrameLeavesIntegersThatFitAsNumbers(t *testing.T) {
	rs := &resultSet{columns: []column{{name: "n", typ: datasource.FieldNumber}}, rows: [][]any{{int64(9007199254740992)}, {int64(-9007199254740992)}, {uint8(7)}}}

	if got := rs.tableFrame().Fields[0]; got.Type != datasource.FieldNumber {
		t.Fatalf("n field = %+v", got)
	}
}

func TestSeriesFramesWarnWhenAValueIsRounded(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	rs := &resultSet{
		columns: []column{{name: "Time", typ: datasource.FieldTime}, {name: "count", typ: datasource.FieldNumber}},
		rows:    [][]any{{at, int64(9007199254740993)}},
	}

	frames := rs.seriesFrames()

	if frames[0].Meta == nil || frames[0].Meta.Warning != roundedValueWarning || frames[0].Fields[1].Type != datasource.FieldNumber {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestTruncationKeepsTheRoundingWarning(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	rs := &resultSet{
		columns: []column{{name: "Time", typ: datasource.FieldTime}, {name: "count", typ: datasource.FieldNumber}},
		rows:    [][]any{{at, int64(9007199254740993)}},
	}

	frames := markTruncated(rs.seriesFrames(), rs, datasource.Limits{MaxRows: 1})

	if meta := frames[0].Meta; meta == nil || !meta.Truncated || meta.Warning != roundedValueWarning {
		t.Fatalf("meta = %+v", meta)
	}
}

type textDecimal struct {
	text  string
	float float64
}

func (d textDecimal) String() string          { return d.text }
func (d textDecimal) InexactFloat64() float64 { return d.float }

func TestTableFrameKeepsADecimalWithMoreDigitsThanAFloatHolds(t *testing.T) {
	rs := &resultSet{
		columns: []column{{name: "wide", typ: datasource.FieldNumber}, {name: "price", typ: datasource.FieldNumber}},
		rows:    [][]any{{textDecimal{"12345678901234567.89", 12345678901234568}, textDecimal{"19.99", 19.99}}},
	}

	frame := rs.tableFrame()

	if got := frame.Fields[0]; got.Type != datasource.FieldString || got.Values[0] != "12345678901234567.89" {
		t.Fatalf("wide field = %+v", got)
	}
	if got := frame.Fields[1]; got.Type != datasource.FieldNumber || got.Values[0] != 19.99 {
		t.Fatalf("price field = %+v", got)
	}
}

func TestTableFrameKeepsAFractionalDecimalAsANumber(t *testing.T) {
	rs := &resultSet{
		columns: []column{{name: "avg_price", typ: datasource.FieldNumber}},
		rows:    [][]any{{textDecimal{"12.345678901234567890", 12.345678901234568}}},
	}
	if rs.roundsInColumn(0) {
		t.Fatal("a decimal with a small integer part is held by a float to the precision a chart shows")
	}
}

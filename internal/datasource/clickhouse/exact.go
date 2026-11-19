package clickhouse

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	maxExactInteger     = 1 << 53
	roundedValueWarning = "values above 2^53 were rounded to fit a number; select them as strings to keep every digit"
)

func beyondFloat(v any) bool {
	switch n := v.(type) {
	case nil:
		return false
	case *big.Int:
		return n != nil && n.CmpAbs(big.NewInt(maxExactInteger)) > 0
	case big.Int:
		return n.CmpAbs(big.NewInt(maxExactInteger)) > 0
	case interface{ InexactFloat64() float64 }:
		text, ok := n.(fmt.Stringer)
		return ok && integerPartBeyondFloat(text.String())
	}
	rv := reflect.ValueOf(v)
	switch {
	case rv.CanInt():
		return rv.Int() > maxExactInteger || rv.Int() < -maxExactInteger
	case rv.CanUint():
		return rv.Uint() > maxExactInteger
	}
	return false
}

func integerPartBeyondFloat(text string) bool {
	whole, _, _ := strings.Cut(text, ".")
	n, ok := new(big.Int).SetString(whole, 10)
	return ok && n.CmpAbs(big.NewInt(maxExactInteger)) > 0
}

func (rs *resultSet) roundsInColumn(i int) bool {
	if rs.columns[i].typ != datasource.FieldNumber {
		return false
	}
	for _, row := range rs.rows {
		if beyondFloat(row[i]) {
			return true
		}
	}
	return false
}

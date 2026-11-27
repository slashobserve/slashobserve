package spec_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDefaultsWritesNeverReachTheNextCall(t *testing.T) {
	want := marshalDefaults(t)
	got := spec.Defaults()
	pointers := 0
	writeThrough(reflect.ValueOf(&got).Elem(), &pointers)
	if pointers == 0 {
		t.Fatal("the defaults table has no pointer field to write through")
	}
	if next := marshalDefaults(t); !bytes.Equal(next, want) {
		t.Fatalf("writing through one Defaults result changed the next one (%d pointers written)", pointers)
	}
}

func marshalDefaults(t *testing.T) []byte {
	t.Helper()
	out, err := json.Marshal(spec.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeThrough(v reflect.Value, pointers *int) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			*pointers++
			writeThrough(v.Elem(), pointers)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			writeThrough(v.Field(i), pointers)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			writeThrough(v.Index(i), pointers)
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(v.MapIndex(key))
			writeThrough(elem, pointers)
			v.SetMapIndex(key, elem)
		}
	case reflect.Interface:
		if !v.IsNil() {
			elem := reflect.New(v.Elem().Type()).Elem()
			elem.Set(v.Elem())
			writeThrough(elem, pointers)
			v.Set(elem)
		}
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.String:
		v.SetString(v.String() + "!")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(v.Uint() + 1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(v.Float() + 1)
	}
}

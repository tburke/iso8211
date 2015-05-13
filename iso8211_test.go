package iso8211

import (
	"reflect"
	"testing"
)

func TestFieldTypeFormat(t *testing.T) {
	var f FieldType
	f.Format_controls = "(A)"
	v := f.Format()
	e := SubFieldType{reflect.String, 0, ""}
	if len(v) != 1 || v[0] != e {
		t.Error("Expected ", e, ", got ", v)
	}
	f.Format_controls = "(b11,2b24,A(3),B(40))"
	f.Array_descriptor = "A!B!C!D!E"
	f.SubFields = nil
	v = f.Format()
	a := []SubFieldType{
		{reflect.Uint8, 1, "A"},
		{reflect.Int32, 4, "B"},
		{reflect.Int32, 4, "C"},
		{reflect.String, 3, "D"},
		{reflect.Array, 5, "E"}}
	if len(v) != len(a) {
		t.Error("Format did not return the expected number of values")
	} else {
		for i, o := range v {
			if o != a[i] {
				t.Error("At ", i, " Expected ", a[i], ", got ", o)
			}
		}
	}
}

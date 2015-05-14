// Copyright 2015 Thomas Burke <tburke@tb99.com>. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package iso8211

import (
	"os"
	"reflect"
	"testing"
)

func TestFieldTypeFormat(t *testing.T) {
	var f FieldType
	f.Format_controls = []byte("(A)")
	v := f.Format()
	e := SubFieldType{reflect.String, 0, nil}
	if len(v) != 1 || !reflect.DeepEqual(v[0], e) {
		t.Error("Expected ", e, ", got ", v)
	}
	var f2 FieldType
	f2.Format_controls = []byte("(b11,2b24,A(3),B(40))")
	f2.Array_descriptor = []byte("A!B!C!D!E")
	v = f2.Format()
	a := []SubFieldType{
		{reflect.Uint8, 1, []byte{'A'}},
		{reflect.Int32, 4, []byte{'B'}},
		{reflect.Int32, 4, []byte{'C'}},
		{reflect.String, 3, []byte{'D'}},
		{reflect.Array, 5, []byte{'E'}}}
	if len(v) != len(a) {
		t.Error("Format did not return the expected number of values")
	} else {
		for i, o := range v {
			if !reflect.DeepEqual(o, a[i]) {
				t.Error("At ", i, " Expected ", a[i], ", got ", o)
			}
		}
	}
}

func TestS57File(t *testing.T) {
	f, err := os.Open("testdata/US5MD12M.001")
	if err != nil {
		t.Error("Unexpected error: ", err)
	}
	defer f.Close()
	var l LeadRecord
	if l.Read(f) != nil {
		t.Error("Error reading the lead record")
	}
	var d DataRecord
	d.Lead = &l
	if d.Read(f) != nil {
		t.Error("Error reading Data record 1")
	}
	if len(d.Fields) != 3 && d.Fields[0].SubFields[0] != 1 {
		t.Error("Data record 1 is not what we expected.")
	}
	if d.Read(f) != nil {
		t.Error("Error reading Data record 2")
	}
	if len(d.Fields) != 4 && d.Fields[0].SubFields[0] != 2 {
		t.Error("Data record 2 is not what we expected.")
	}
	if len(d.Fields[3].SubFields) != 6 && d.Fields[3].SubFields[4] != 148 {
		t.Error("Data record 2, Field 4 is not what we expected.", d.Fields[3])
	}
	if d.Read(f) == nil {
		t.Error("Should be at EOF")
	}
}

// Copyright 2015 Thomas Burke <tburke@tb99.com>. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package iso8211 implements ISO 8211 parsing.
// It is targeted to NOAA IHO S-57 format vector chart files.
//
// ISO 8211 is one of those baroque 1990's era binary file formats.
//   file: LeadRecord, DataRecord...
//   Record : Header, data
//   LeadRecord : Header, FieldType...
//   DataRecord : Header, Field...
//   FieldType : FieldHeader, SubField tags and formats
//   Field : SubFields
//
// References:
//   http://www.iho.int/iho_pubs/standard/S-57Ed3.1/31Main.pdf
//   http://sourceforge.net/projects/py-iso8211/
//   https://www.iso.org/obp/ui/#iso:std:iso-iec:8211:ed-2:v1:en
//   http://mcmcweb.er.usgs.gov/sdts/SDTS_standard_nov97/p3body.html
//   http://www.charts.noaa.gov/ENCs/ENCs.shtml
package iso8211

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strconv"
)

// RawHeader is a convenience for directly loading the on-disk
// binary Header format.
type RawHeader struct {
	RecordLength                  [5]byte
	InterchangeLevel              byte
	LeaderId                      byte
	InLineCode                    byte
	Version                       byte
	ApplicationIndicator          byte
	FieldControlLength            [2]byte
	BaseAddress                   [5]byte
	ExtendedCharacterSetIndicator [3]byte
	SizeOfFieldLength             byte
	SizeOfFieldPosition           byte
	Reserved                      byte
	SizeOfFieldTag                byte
}

// DirEntry describes each following Field
type DirEntry struct {
	Tag      []byte
	Length   int
	Position int
}

// Header holds the overall layout for a Record.
type Header struct {
	RecordLength                      uint64
	InterchangeLevel                  byte
	LeaderId                          byte
	InLineCode                        byte
	Version                           byte
	ApplicationIndicator              byte
	FieldControlLength                uint64
	BaseAddress                       uint64
	ExtendedCharacterSetIndicator     []byte
	LengthSize, PositionSize, TagSize int8
	Entries                           []DirEntry
}

// LeadRecord is the first Record in a file. It has metadata for each
// Field in the file.
type LeadRecord struct {
	Header     Header
	FieldTypes map[string]FieldType
}

type Field struct {
	Tag       string
	Length    int
	Position  int
	FieldType FieldType
	SubFields []interface{}
}

// DataRecord contains data for a set of Fields and their SubFields.
type DataRecord struct {
	Header Header
	Lead   *LeadRecord
	Fields []Field
}

type RawFieldHeader struct {
	DataStructure     byte
	DataType          byte
	AuxiliaryControls [2]byte
	PrintableFt       byte
	PrintableUt       byte
	EscapeSeq         [3]byte
}

type SubFieldType struct {
	Kind reflect.Kind
	Size int
	Tag  []byte
}

type FieldType struct {
	Tag               string
	Length            int
	Position          int
	DataStructure     byte
	DataType          byte
	AuxiliaryControls []byte
	PrintableFt       byte
	PrintableUt       byte
	EscapeSeq         []byte
	Name              []byte
	ArrayDescriptor   []byte
	FormatControls    []byte
	SubFields         []SubFieldType
}

// Read loads a binary format RawHeader and its DirEntries into
// the Header model.
func (header *Header) Read(file io.Reader) error {
	var err error
	var ddr RawHeader
	ddrSize := uint64(binary.Size(ddr))
	// Read the header
	err = binary.Read(file, binary.LittleEndian, &ddr)
	if err != nil {
		return err
	}
	header.RecordLength, _ = strconv.ParseUint(string(ddr.RecordLength[:]), 10, 64)
	header.InterchangeLevel = ddr.InterchangeLevel
	header.LeaderId = ddr.LeaderId
	header.InLineCode = ddr.InLineCode
	header.Version = ddr.Version
	header.ApplicationIndicator = ddr.ApplicationIndicator
	header.FieldControlLength, _ = strconv.ParseUint(string(ddr.FieldControlLength[:]), 10, 64)
	header.BaseAddress, _ = strconv.ParseUint(string(ddr.BaseAddress[:]), 10, 64)
	header.ExtendedCharacterSetIndicator = ddr.ExtendedCharacterSetIndicator[:]
	header.LengthSize = int8(ddr.SizeOfFieldLength - '0')
	header.PositionSize = int8(ddr.SizeOfFieldPosition - '0')
	header.TagSize = int8(ddr.SizeOfFieldTag - '0')
	// Read the directory
	entries := (header.BaseAddress - 1 - ddrSize) / uint64(header.LengthSize+header.PositionSize+header.TagSize)
	header.Entries = make([]DirEntry, entries)
	dir := make([]byte, header.BaseAddress-ddrSize)
	file.Read(dir)
	buf := bytes.NewBuffer(dir)
	for idx := uint64(0); idx < entries; idx++ {
		header.Entries[idx].Tag = buf.Next(int(header.TagSize))
		header.Entries[idx].Length, _ = strconv.Atoi(string(buf.Next(int(header.LengthSize))[:]))
		header.Entries[idx].Position, _ = strconv.Atoi(string(buf.Next(int(header.PositionSize))[:]))
	}
	return err
}

// Read loads the LeadRecord Header and the FieldTypes
func (lead *LeadRecord) Read(file io.Reader) error {
	var err error
	err = lead.Header.Read(file)
	if err != nil {
		return err
	}
	if lead.Header.LeaderId != 'L' {
		return errors.New("Record is not a Lead record")
	}
	err = lead.ReadFields(file)
	return err
}

func (lead *LeadRecord) ReadFields(file io.Reader) error {
	var err error
	lead.FieldTypes = make(map[string]FieldType, len(lead.Header.Entries))
	for _, d := range lead.Header.Entries {
		field := FieldType{Tag: string(d.Tag), Length: d.Length, Position: d.Position}
		field.Read(file)
		lead.FieldTypes[field.Tag] = field
	}
	return err
}

func (field *Field) Read(file io.Reader) error {
	var err error
	data := make([]byte, field.Length)
	file.Read(data)
	if field.FieldType.Tag != "" {
		field.SubFields = field.FieldType.Decode(data[:field.Length-1])
	}
	return err
}

func (data *DataRecord) Read(file io.Reader) error {
	var err error
	err = data.Header.Read(file)
	if err != nil {
		return err
	}
	if data.Header.LeaderId != 'D' {
		return errors.New("Record is not a Data record")
	}
	err = data.ReadFields(file)
	return err
}

func (data *DataRecord) ReadFields(file io.Reader) error {
	var err error
	data.Fields = make([]Field, len(data.Header.Entries))
	for i, d := range data.Header.Entries {
		field := Field{Tag: string(d.Tag), Length: d.Length, Position: d.Position}
		if data.Lead != nil {
			field.FieldType = data.Lead.FieldTypes[field.Tag]
		}
		err = field.Read(file)
		data.Fields[i] = field
	}
	return err
}

func (dir *FieldType) Read(file io.Reader) error {
	var field RawFieldHeader
	err := binary.Read(file, binary.LittleEndian, &field)
	dir.DataStructure = field.DataStructure
	dir.DataType = field.DataType
	dir.AuxiliaryControls = field.AuxiliaryControls[:]
	dir.PrintableFt = field.PrintableFt
	dir.PrintableUt = field.PrintableUt
	dir.EscapeSeq = field.EscapeSeq[:]
	fdata := make([]byte, dir.Length-9)
	file.Read(fdata)
	desc := bytes.Split(fdata[:dir.Length-10], []byte{'\x1f'})
	dir.Name = desc[0]
	dir.ArrayDescriptor = desc[1]
	if len(desc) > 2 {
		dir.FormatControls = desc[2]
	}
	return err
}

/*
Format parses the ISO-8211 format controls and array descriptors.

Based on Section 7.2.2.1 of the IHO S-57 Publication.
http://www.iho.int/iho_pubs/standard/S-57Ed3.1/31Main.pdf

Array Descriptor and Format Controls. The array descriptor is a ! separated
list of tags describing the data field. If it begins with a * the tag list
is repeated. The format controls decribe the format of the data for each tag.

eg: Descriptor AGEN!FIDN!FIDS , Format (b12,b14,b12) is three binary encoded
integers. AGEN is an int16, FIDN an int32 and FIDS an int16. The 'b' indicates
binary int, '1' indicates unsigned, the second digit indicates the number of
bytes.
Decriptor *YCOO!XCOO, Format (2b24) is two binary encoded integers. Both are
int32s, the '2' after the 'b' indicates signed. The * in the descriptor
indicates that pair is repeated to fill the data field.
*/
func (dir *FieldType) Format() []SubFieldType {
	if dir.SubFields != nil {
		return dir.SubFields
	}
	var re = regexp.MustCompile(`(\d*)(\w+)\(*(\d*)\)*`)

	if len(dir.FormatControls) > 2 {
		Tags := bytes.Split(dir.ArrayDescriptor, []byte{'!'})
		Tagidx := 0
		types := make([]SubFieldType, len(Tags))
		for _, a := range re.FindAllSubmatch(dir.FormatControls, -1) {
			i := 1
			if len(a[1]) > 0 {
				i, _ = strconv.Atoi(string(a[1]))
			}
			var size int
			if len(a[3]) > 0 {
				size, _ = strconv.Atoi(string(a[3]))
			}
			for ; i > 0; i-- {
				switch a[2][0] {
				case 'A':
					types[Tagidx] = SubFieldType{reflect.String, size, Tags[Tagidx]}
				case 'I':
				case 'R':
					types[Tagidx] = SubFieldType{reflect.String, size, Tags[Tagidx]}
				case 'B':
					types[Tagidx] = SubFieldType{reflect.Array, size / 8, Tags[Tagidx]}
				case 'b':
					switch string(a[2][1:]) {
					case "11":
						types[Tagidx] = SubFieldType{reflect.Uint8, 1, Tags[Tagidx]}
					case "12":
						types[Tagidx] = SubFieldType{reflect.Uint16, 2, Tags[Tagidx]}
					case "14":
						types[Tagidx] = SubFieldType{reflect.Uint32, 4, Tags[Tagidx]}
					case "21":
						types[Tagidx] = SubFieldType{reflect.Int8, 1, Tags[Tagidx]}
					case "22":
						types[Tagidx] = SubFieldType{reflect.Int16, 2, Tags[Tagidx]}
					case "24":
						types[Tagidx] = SubFieldType{reflect.Int32, 4, Tags[Tagidx]}
					}
				}
				Tagidx++
			}
		}
		dir.SubFields = types
	}
	return dir.SubFields
}

// Decode uses the FieldType Format to convert the binary file format
// SubFields into an array of Go data types.
func (dir FieldType) Decode(buffer []byte) []interface{} {
	buf := bytes.NewBuffer(buffer)
	var values []interface{}
	for buf.Len() > 0 {
		for _, ftype := range dir.Format() {
			switch ftype.Kind {
			case reflect.Uint8:
				{
					var v uint8
					binary.Read(buf, binary.LittleEndian, &v)
					values = append(values, v)
				}
			case reflect.Uint16:
				{
					var v uint16
					binary.Read(buf, binary.LittleEndian, &v)
					values = append(values, v)
				}
			case reflect.Uint32:
				{
					var v uint32
					binary.Read(buf, binary.LittleEndian, &v)
					values = append(values, v)
				}
			case reflect.Int8:
				{
					var v int8
					binary.Read(buf, binary.LittleEndian, &v)
					values = append(values, v)
				}
			case reflect.Int16:
				{
					var v int16
					binary.Read(buf, binary.LittleEndian, &v)
					values = append(values, v)
				}
			case reflect.Int32:
				{
					var v int32
					binary.Read(buf, binary.LittleEndian, &v)
					values = append(values, v)
				}
			default:
				{
					if ftype.Size == 0 {
						i, _ := buf.ReadString('\x1f')
						if len(i) > 0 {
							values = append(values, i[:len(i)-1])
						} else {
							values = append(values, "")
						}
					} else {
						i := buf.Next(ftype.Size)
						values = append(values, string(i))
					}
				}
			}
		}
	}
	return values
}

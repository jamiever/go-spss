package gospss

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

var endian = binary.LittleEndian
var count = 0

type spssWriter struct {
	*bufio.Writer                 // Buffered writer
	seeker        io.WriteSeeker  // Original writer
	bytecode      *bytecodeWriter // Special writer for compressed cases
	// Dict          []*Var          // Variables
	// DictMap       map[string]*Var // Long variable names index
	// ShortMap      map[string]*Var // Short variable names index
	// Count         int32           // Number of cases
	index int32
}

func newSpssWriter(w io.WriteSeeker) *spssWriter {
	writer := bufio.NewWriter(w)
	byteCode := newBytecodeWriter(writer, 100.0)
	return &spssWriter{
		seeker:   w,
		Writer:   writer,
		bytecode: byteCode,
		// DictMap:  make(map[string]*Var),
		// ShortMap: make(map[string]*Var),
		index: 1,
	}
}

func stob(s string, l int) []byte {
	if len(s) > l {
		s = s[:l]
	} else if len(s) < l {
		s += strings.Repeat(" ", l-len(s))
	}
	return []byte(s)
}

func stobp(s string, l int, pad byte) []byte {
	if len(s) > l {
		s = s[:l]
	} else if len(s) < l {
		s += strings.Repeat(string([]byte{pad}), l-len(s))
	}
	return []byte(s)
}

func trim(s string, l int) string {
	if len(s) > l {
		return s[:l]
	}
	return s
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'E', -1, 64)
}

func atof(s string) float64 {
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		log.Fatalln(err)
	}
	return v
}

func elementCount(width int32) int32 {
	return ((width - 1) / 8) + 1
}

func caseSize() int32 {
	size := int32(0)
	for _, v := range variables {
		for s := 0; s < int(v.segments); s++ {
			size += elementCount(v.segmentWidth(s))
		}
	}
	return size
}

func (s *spssWriter) writeString(v variable, val string) error {
	for se := 0; se < int(v.segments); se++ {
		var p string
		if len(val) > 255 {
			p = val[:255]
			val = val[255:]
		} else {
			p = val
			val = ""
		}

		if err := s.bytecode.WriteString(p, int(elementCount(v.segmentWidth(se)))); err != nil {
			return err
		}
	}

	return nil
}

func (s *spssWriter) writeValues(values map[string]string) error {
	for _, v := range variables {
		s.Flush()
		var val, hasVal = values[v.name]

		if !hasVal {
			if v.spssType == SpssTypeString {
				s.writeString(v, "")
			} else {
				s.bytecode.WriteMissing()
			}

			continue
		}

		switch v.spssType {
		case SpssTypeString:
			if len(val) > int(v.width) {
				val = val[:v.width]
			}
			s.writeString(v, val)
		case SpssTypeDate:
			t, err := time.Parse("1-Jan-2019", val)
			if err != nil {
				// log.Printf("Writing missing value: %s", v.name)
				s.bytecode.WriteMissing()
			} else {
				s.bytecode.WriteNumber(float64(t.Unix()))
			}
		case SpssTypeDatetime:
			t, err := time.Parse("1-Jan-2019 14:00:00", val)
			if err != nil {
				// log.Printf("Writing missing value: %s", v.name)
				s.bytecode.WriteMissing()
			} else {
				s.bytecode.WriteNumber(float64(t.Unix()))
			}
		default:
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				// log.Printf("Writing missing value: %s", v.name)
				s.bytecode.WriteMissing()
			} else {
				s.bytecode.WriteNumber(f)
			}
		}
	}

	count++
	return nil
}

func (s *spssWriter) start() error {
	s.headerRecord("testing")
	// Write variables
	if err := s.writeVariables(variables); err != nil {
		return fmt.Errorf("Error during writing of variables: %s", err.Error())
	}

	s.valueLabelRecords()
	s.machineIntegerInfoRecord()
	s.machineFloatingPointInfoRecord()
	s.variableDisplayParameterRecord()
	s.longVarNameRecords()
	s.veryLongStringRecord()
	s.encodingRecord()
	s.longStringValueLabelsRecord()
	s.terminationRecord()

	return nil
}

func (s *spssWriter) headerRecord(fileLabel string) {
	c := time.Now()
	s.Write(stob("$FL2", 4))                               // rec_tyoe
	s.Write(stob("@(#) SPSS DATA FILE - xml2sav 2.0", 60)) // prod_name
	binary.Write(s, endian, int32(2))                      // layout_code
	binary.Write(s, endian, caseSize())                    // nominal_case_size
	binary.Write(s, endian, int32(1))                      // compression
	binary.Write(s, endian, int32(0))                      // weight_index
	binary.Write(s, endian, int32(-1))                     // ncases
	binary.Write(s, endian, float64(100))                  // bias
	s.Write(stob(c.Format("02 Jan 06"), 9))                // creation_date
	s.Write(stob(c.Format("15:04:05"), 8))                 // creation_time
	s.Write(stob(fileLabel, 64))                           // file_label
	s.Write(stob("\x00\x00\x00", 3))                       // padding
}

func (s *spssWriter) writeVariables(vars []variable) error {
	for _, v := range vars {
		s.Flush()
		// log.Printf("Adding variable: %+v\n", v)
		for segment := 0; segment < int(v.segments); segment++ {
			width := v.segmentWidth(segment)
			binary.Write(s, endian, int32(2)) // rec_type
			binary.Write(s, endian, width)

			if segment == 0 && len(v.label) > 0 {
				binary.Write(s, endian, int32(1)) // Has label
			} else {
				binary.Write(s, endian, int32(0)) // No label
			}
			binary.Write(s, endian, int32(0)) // Missing values

			var format int32
			if v.spssType == SpssTypeString {
				format = int32(v.format)<<16 | width<<8
			} else {
				format = int32(v.format)<<16 | int32(v.width)<<8 | int32(v.decimal)
			}

			binary.Write(s, endian, format)
			binary.Write(s, endian, format)

			s.Write(stob(v.shortName, 8))

			if segment == 0 && len(v.label) > 0 {
				binary.Write(s, endian, int32(len(v.label))) // Label length
				s.Write([]byte(v.label))
				pad := (4 - len(v.label)) % 4

				if pad < 0 {
					pad += 4
				}

				for i := 0; i < pad; i++ {
					s.Write([]byte{0})
				}
			}

			if width > 8 {
				count := int(elementCount(width) - 1) // Number of extra variables to store string
				for i := 0; i < count; i++ {
					binary.Write(s, endian, int32(2))  // rec_type
					binary.Write(s, endian, int32(-1)) // extended string part
					binary.Write(s, endian, int32(0))  // has_var_label
					binary.Write(s, endian, int32(0))  // n_missing_values
					binary.Write(s, endian, int32(0))  // print
					binary.Write(s, endian, int32(0))  // write
					s.Write(stob("        ", 8))       // name
				}
			}
		}
	}

	return nil
}

func (s *spssWriter) valueLabelRecords() {
	for _, v := range variables {
		if len(v.labels) > 0 && v.spssType != SpssTypeString {
			binary.Write(s, endian, int32(3))             // rec_type
			binary.Write(s, endian, int32(len(v.labels))) // label_count

			for _, label := range v.labels {
				if v.spssType != SpssTypeNumeric {
					binary.Write(s, endian, stob(label.Value, 8)) // value
				} else {
					binary.Write(s, endian, atof(label.Value)) // value

				}
				l := len(label.Desc)
				if l > 120 {
					l = 120
				}
				binary.Write(s, endian, byte(l)) // label_len
				s.Write(stob(label.Desc, l))     // label
				pad := (8 - l - 1) % 8
				if pad < 0 {
					pad += 8
				}
				for i := 0; i < pad; i++ {
					s.Write([]byte{32})
				}
			}

			binary.Write(s, endian, int32(4)) // rec_type
			binary.Write(s, endian, int32(1)) // var_count
			// log.Printf("Index: %d", v.index)
			binary.Write(s, endian, v.index) // vars
		}
	}
}

func (s *spssWriter) machineIntegerInfoRecord() {
	binary.Write(s, endian, int32(7))     // rec_type
	binary.Write(s, endian, int32(3))     // subtype
	binary.Write(s, endian, int32(4))     // size
	binary.Write(s, endian, int32(8))     // count
	binary.Write(s, endian, int32(0))     // version_major
	binary.Write(s, endian, int32(10))    // version_minor
	binary.Write(s, endian, int32(1))     // version_revision
	binary.Write(s, endian, int32(-1))    // machine_code
	binary.Write(s, endian, int32(1))     // floating_point_rep
	binary.Write(s, endian, int32(1))     // compression_code
	binary.Write(s, endian, int32(2))     // endianness
	binary.Write(s, endian, int32(65001)) // character_code
}

func (s *spssWriter) machineFloatingPointInfoRecord() {
	binary.Write(s, endian, int32(7))                  // rec_type
	binary.Write(s, endian, int32(4))                  // subtype
	binary.Write(s, endian, int32(8))                  // size
	binary.Write(s, endian, int32(3))                  // count
	binary.Write(s, endian, float64(-math.MaxFloat64)) // sysmis
	binary.Write(s, endian, float64(math.MaxFloat64))  // highest
	binary.Write(s, endian, float64(-math.MaxFloat64)) // lowest
}

func varCount() int32 {
	var count int32
	for _, v := range variables {
		count += int32(v.segments)
	}
	return count
}

func (s *spssWriter) variableDisplayParameterRecord() {
	binary.Write(s, endian, int32(7))     // rec_type
	binary.Write(s, endian, int32(11))    // subtype
	binary.Write(s, endian, int32(4))     // size
	binary.Write(s, endian, varCount()*3) // count
	for _, v := range variables {
		for se := 0; se < int(v.segments); se++ {
			binary.Write(s, endian, int32(v.measure)) // measure
			if v.spssType == SpssTypeString {
				if se != 0 {
					binary.Write(s, endian, int32(8)) // width
				} else if int(v.width) <= 40 {
					binary.Write(s, endian, int32(v.width)) // width
				} else {
					binary.Write(s, endian, int32(40))
				}
				binary.Write(s, endian, int32(0)) // alignment (left)
			} else {
				binary.Write(s, endian, int32(8)) // width
				binary.Write(s, endian, int32(1)) // alignment (right)
			}
		}
	}
}

func (s *spssWriter) longVarNameRecords() {
	binary.Write(s, endian, int32(7))  // rec_type
	binary.Write(s, endian, int32(13)) // subtype
	binary.Write(s, endian, int32(1))  // size

	buf := bytes.Buffer{}
	for i, v := range variables {
		buf.Write([]byte(v.shortName))
		buf.Write([]byte("="))
		buf.Write([]byte(v.name))
		if i < len(variables)-1 {
			buf.Write([]byte{9})
		}
	}
	binary.Write(s, endian, int32(buf.Len()))
	s.Write(buf.Bytes())
}

func (s *spssWriter) veryLongStringRecord() {
	b := false
	for _, v := range variables {
		if int(v.segments) > 1 {
			b = true
			break
		}
	}

	if !b {
		// There are no very long strings so don't write the record
		return
	}

	binary.Write(s, endian, int32(7))  // rec_type
	binary.Write(s, endian, int32(14)) // subtype
	binary.Write(s, endian, int32(1))  // size

	buf := bytes.Buffer{}
	for _, v := range variables {
		if v.segments > 1 {
			buf.Write([]byte(v.shortName))
			buf.Write([]byte("="))
			buf.Write(stobp(strconv.Itoa(0), 5, 0))
			buf.Write([]byte{0, 9})
		}
	}
	binary.Write(s, endian, int32(buf.Len())) // count
	s.Write(buf.Bytes())
}

func (s *spssWriter) encodingRecord() {
	binary.Write(s, endian, int32(7))  // rec_type
	binary.Write(s, endian, int32(20)) // subtype
	binary.Write(s, endian, int32(1))  // size
	binary.Write(s, endian, int32(5))  // filler
	s.Write(stob("UTF-8", 5))          // encoding
}

func (s *spssWriter) longStringValueLabelsRecord() {
	// Check if we have any
	any := false
	for _, v := range variables {
		if len(v.labels) > 0 && v.spssType == SpssTypeString {
			any = true
			break
		}
	}
	if !any {
		return
	}

	// Create record
	buf := new(bytes.Buffer)
	for _, v := range variables {
		if len(v.labels) > 0 && v.spssType == SpssTypeString {
			binary.Write(buf, endian, int32(len(v.shortName))) // var_name_len
			buf.Write([]byte(v.shortName))                     // var_name
			binary.Write(buf, endian, int32(v.width))          // var_width
			binary.Write(buf, endian, int32(len(v.labels)))    // n_labels
			for _, l := range v.labels {
				binary.Write(buf, endian, int32(len(l.Value))) // value_len
				buf.Write([]byte(l.Value))                     // value
				binary.Write(buf, endian, int32(len(l.Desc)))  // label_len
				buf.Write([]byte(l.Desc))                      //label
			}
		}
	}

	binary.Write(s, endian, int32(7))         // rec_type
	binary.Write(s, endian, int32(21))        // subtype
	binary.Write(s, endian, int32(1))         // size
	binary.Write(s, endian, int32(buf.Len())) // count
	s.Write(buf.Bytes())
}

func (s *spssWriter) terminationRecord() {
	binary.Write(s, endian, int32(999)) // rec_type
	binary.Write(s, endian, int32(0))   // filler
}

// If you use a buffer, supply it as the flusher argument
// After this close the file
func (s *spssWriter) updateHeaderNCases() {
	s.bytecode.Flush()
	s.Flush()
	s.seeker.Seek(80, 0)
	binary.Write(s.seeker, endian, int32(count)) // ncases in headerRecord
}

func (s *spssWriter) finish() {
	s.updateHeaderNCases()
	count = 0
}

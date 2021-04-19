package gospss

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fedom/writerseeker"
)

var endian = binary.LittleEndian

const TimeOffset = 12219379200

// SpssWriter defines the struct to write SPSS objects
type SpssWriter struct {
	*bufio.Writer                   // Buffered writer
	seeker        io.WriteSeeker    // Original writer
	bytecode      *bytecodeWriter   // Special writer for compressed cases
	names         map[string]string // Mapping of names for easy access
	count         int               // Count of values
	index         int32             // Writing index
	endian        binary.ByteOrder  // Endian
	variables     []variable        // Written variables
	valCount      int               // Number of value rows
	productName   string            // name to place in header denoting the product generating this file
}

// NewSpssWriter - Returns an SPSS Writer struct given a file
func NewSpssWriter(file *os.File) (*SpssWriter, error) {
	writer := bufio.NewWriter(file)

	byteCode := newBytecodeWriter(writer, 100.0)

	spssWriter := &SpssWriter{
		seeker:      file,
		Writer:      writer,
		bytecode:    byteCode,
		names:       make(map[string]string),
		variables:   make([]variable, 0, 1),
		index:       1,
		endian:      binary.LittleEndian,
		count:       0,
		productName: "xml2sav 2.0",
	}

	spssWriter.headerRecord()

	return spssWriter, nil
}

// NewSpssWriter - Returns an SPSS Writer struct using an in memory buffer
func NewSpssInMemoryWriter(f *writerseeker.WriterSeeker, productName string) (*SpssWriter, error) {
	writer := bufio.NewWriter(f)

	byteCode := newBytecodeWriter(writer, 100.0)

	if productName == "" {
		productName = "xml2sav 2.0"
	}
	spssWriter := &SpssWriter{
		seeker:      f,
		Writer:      writer,
		bytecode:    byteCode,
		names:       make(map[string]string),
		variables:   make([]variable, 0, 1),
		index:       1,
		endian:      binary.LittleEndian,
		count:       0,
		productName: productName,
	}

	spssWriter.headerRecord()

	return spssWriter, nil
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

func (s *SpssWriter) caseSize() int32 {
	size := int32(0)
	for _, v := range s.variables {
		for s := 0; s < int(v.segments); s++ {
			size += elementCount(v.segmentWidth(s))
		}
	}
	return size
}

func (s *SpssWriter) writeString(v variable, val string) error {
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

// AddValueRow - Add a row of values to the SPSS file
// CAUTION: All variables must be written before adding values
func (s *SpssWriter) AddValueRow(values map[string]string) error {
	if s.valCount == 0 {
		s.writeInfoRecords()
	}

	for _, v := range s.variables {
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
			t, err := time.Parse("02-Jan-2006", val)
			if err != nil {
				// log.Printf("Writing missing value: %s", v.name)
				s.bytecode.WriteMissing()
			} else {
				s.bytecode.WriteNumber(float64(t.Unix() + TimeOffset))
			}
		case SpssTypeDatetime:
			t, err := time.Parse("02-Jan-2006 15:04:05", val)
			if err != nil {
				// log.Printf("Writing missing value: %s", v.name)
				s.bytecode.WriteMissing()
			} else {
				s.bytecode.WriteNumber(float64(t.Unix() + TimeOffset))
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

	s.valCount++
	return nil
}

func (s *SpssWriter) writeInfoRecords() {
	s.valueLabelRecords()
	s.machineIntegerInfoRecord()
	s.machineFloatingPointInfoRecord()
	s.variableDisplayParameterRecord()
	s.longVarNameRecords()
	s.veryLongStringRecord()
	s.encodingRecord()
	s.longStringValueLabelsRecord()
	s.terminationRecord()
}

func (s *SpssWriter) headerRecord() {
	c := time.Now()
	s.Write(stob("$FL2", 4))                                                  // rec_tyoe
	s.Write(stob(fmt.Sprintf("@(#) SPSS DATA FILE - %s", s.productName), 60)) // prod_name
	binary.Write(s, endian, int32(2))                                         // layout_code
	binary.Write(s, endian, s.caseSize())                                     // nominal_case_size
	binary.Write(s, endian, int32(1))                                         // compression
	binary.Write(s, endian, int32(0))                                         // weight_index
	binary.Write(s, endian, int32(-1))                                        // ncases
	binary.Write(s, endian, float64(100))                                     // bias
	s.Write(stob(c.Format("02 Jan 06"), 9))                                   // creation_date
	s.Write(stob(c.Format("15:04:05"), 8))                                    // creation_time
	s.Write(stob("Generated SPSS", 64))                                       // file_label
	s.Write(stob("\x00\x00\x00", 3))                                          // padding
}

// AddVariable - Add variables to the SPSS file
// CAUTION: Once values are being written you cannot add any more variables
func (s *SpssWriter) AddVariable(V *Variable) error {
	// Check if name is empty
	if V.Name == "" {
		return fmt.Errorf("Name cannot be empty")
	}

	if len(V.Name) > 64 {
		return fmt.Errorf("Name cannot exceed 64 characters: %s", V.Name)
	}

	matched := nameValidatorRegex.MatchString(V.Name)

	if !matched {
		return fmt.Errorf("Name %s does not meet the requirements for SPSS, please refer to https://www.ibm.com/support/knowledgecenter/en/SSLVMB_24.0.0/spss/base/syn_variables_variable_names.html", V.Name)
	}

	// Check if name already exists (duplicate)
	_, exists := s.names[V.Name]

	if exists {
		return fmt.Errorf("Cannot add variable with name %s since it already exists", V.Name)
	}

	// Check decimal range
	if V.Decimal < 0 || V.Decimal > 16 {
		return fmt.Errorf("Cannot set decimal of %d, value must be between 0 and 16", V.Decimal)
	}

	if V.Width < 0 || V.Width > 32767 {
		return fmt.Errorf("Cannot set width of %d, value must be between 0 and 32676", V.Width)
	}

	if V.Type != SpssTypeString && V.Width > 40 {
		return fmt.Errorf("Cannot set width of %d on type %s, value must be between 1 and 40", V.Width, V.Type)
	}

	// Check if width is set, get the default otherwise
	if V.Width == 0 {
		if err := V.setDefaultWidth(); err != nil {
			return err
		}
	} else {
		if V.Width <= int16(V.Decimal) {
			return fmt.Errorf("Width cannot be less or equal to decimal")
		}
	}

	v := variable{
		index:     s.index,
		name:      V.Name,
		shortName: V.checkAndGetShortName(s, V.ShortName),
		spssType:  V.Type,
		measure:   V.getMeasure(),
		decimal:   V.Decimal,
		width:     V.Width,
		format:    V.getPrint(),
		segments:  V.getSegments(),
		labels:    V.Labels,
		label:     V.Label,
	}

	for i := 0; i < int(v.segments); i++ {
		s.index += int32(elementCount(v.segmentWidth(i)))
	}

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

		s.variables = append(s.variables, v)
	}

	return nil
}

func (s *SpssWriter) valueLabelRecords() {
	for _, v := range s.variables {
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

func (s *SpssWriter) machineIntegerInfoRecord() {
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

func (s *SpssWriter) machineFloatingPointInfoRecord() {
	binary.Write(s, endian, int32(7))                  // rec_type
	binary.Write(s, endian, int32(4))                  // subtype
	binary.Write(s, endian, int32(8))                  // size
	binary.Write(s, endian, int32(3))                  // count
	binary.Write(s, endian, float64(-math.MaxFloat64)) // sysmis
	binary.Write(s, endian, float64(math.MaxFloat64))  // highest
	binary.Write(s, endian, float64(-math.MaxFloat64)) // lowest
}

func (s *SpssWriter) varCount() int32 {
	var count int32
	for _, v := range s.variables {
		count += int32(v.segments)
	}
	return count
}

func (s *SpssWriter) variableDisplayParameterRecord() {
	binary.Write(s, endian, int32(7))       // rec_type
	binary.Write(s, endian, int32(11))      // subtype
	binary.Write(s, endian, int32(4))       // size
	binary.Write(s, endian, s.varCount()*3) // count
	for _, v := range s.variables {
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

func (s *SpssWriter) longVarNameRecords() {
	binary.Write(s, endian, int32(7))  // rec_type
	binary.Write(s, endian, int32(13)) // subtype
	binary.Write(s, endian, int32(1))  // size

	buf := bytes.Buffer{}
	i := 0
	for short, long := range s.names {
		buf.Write([]byte(short))
		buf.Write([]byte("="))
		buf.Write([]byte(long))
		if i < len(s.names)-1 {
			buf.Write([]byte{9})
		}
		i++
	}
	binary.Write(s, endian, int32(buf.Len()))
	s.Write(buf.Bytes())
}

func (s *SpssWriter) veryLongStringRecord() {
	b := false
	for _, v := range s.variables {
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
	for _, v := range s.variables {
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

func (s *SpssWriter) encodingRecord() {
	binary.Write(s, endian, int32(7))  // rec_type
	binary.Write(s, endian, int32(20)) // subtype
	binary.Write(s, endian, int32(1))  // size
	binary.Write(s, endian, int32(5))  // filler
	s.Write(stob("UTF-8", 5))          // encoding
}

func (s *SpssWriter) longStringValueLabelsRecord() {
	// Check if we have any
	any := false
	for _, v := range s.variables {
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
	for _, v := range s.variables {
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

func (s *SpssWriter) terminationRecord() {
	binary.Write(s, endian, int32(999)) // rec_type
	binary.Write(s, endian, int32(0))   // filler
}

// If you use a buffer, supply it as the flusher argument
// After this close the file
func (s *SpssWriter) updateHeaderNCases() {
	s.bytecode.Flush()
	s.Flush()
	s.seeker.Seek(80, 0)
	binary.Write(s.seeker, endian, int32(s.valCount)) // ncases in headerRecord
}

// Finish - Execute this once all variables and values are written to complete the file
func (s *SpssWriter) Finish() {
	s.updateHeaderNCases()
	s.Flush()
}

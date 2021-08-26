package gospss

import (
	"regexp"
	"strconv"
	"strings"
)

// SpssType declares different types of fields
type SpssType string

const (
	// SpssTypeNumeric is the default numeric type
	SpssTypeNumeric SpssType = "NUMERIC"
	// SpssTypeDate is the date type
	SpssTypeDate SpssType = "DATE"
	// SpssTypeDatetime is the datetime type
	SpssTypeDatetime SpssType = "DATETIME"
	// SpssTypeString is the string type
	SpssTypeString SpssType = "STRING"
)

// SpssMeasure declares different types of measures
type SpssMeasure string

const (
	// SpssMeasureOrdinal constant, refer to https://www.ibm.com/support/knowledgecenter/en/SSLVMB_23.0.0/spss/base/chart_creation_vartypes.html
	SpssMeasureOrdinal SpssMeasure = "ORDINAL"
	// SpssMeasureNominal constant, refer to https://www.ibm.com/support/knowledgecenter/en/SSLVMB_23.0.0/spss/base/chart_creation_vartypes.html
	SpssMeasureNominal SpssMeasure = "NOMINAL"
	// SpssMeasureScale constant, refer to https://www.ibm.com/support/knowledgecenter/en/SSLVMB_23.0.0/spss/base/chart_creation_vartypes.html
	SpssMeasureScale SpssMeasure = "SCALE"
)

// Label defines the structure for value labels on variables
type Label struct {
	Value string
	Desc  string
}

// // SpssConfig defines the structure for generating your SPSS file
// type SpssConfig struct {
// 	Variables []Variable
// 	Values    [][]Value
// }

// Variable defines the configuration for adding variables to the SPSS configuration
type Variable struct {
	Name      string
	ShortName string
	Type      SpssType
	Measure   SpssMeasure
	Decimal   int8
	Width     int16
	Label     string
	Labels    []Label
}

type variable struct {
	index     int32
	name      string
	shortName string
	spssType  SpssType
	calcType  int32
	measure   int8
	decimal   int8
	width     int16
	format    int8
	segments  int16
	label     string
	labels    []Label
}

// Value defines the values for each field
type Value struct {
	Name  string
	Value string
}

var nameValidatorRegex = regexp.MustCompile(`(?si)^[a-z@][a-z0-9!._#@$]*[^\.]$`)

func (v *Variable) getMeasure() int8 {
	switch v.Measure {
	case SpssMeasureScale:
		return 3
	case SpssMeasureOrdinal:
		return 2
	default:
		return 1
	}
}

// checkAndGetShortName returns the specified shortName otherwise generates one
func (v *Variable) checkAndGetShortName(s *SpssWriter, shortName string) string {
	if shortName == "" {
		return v.getShortName(s)
	}
	s.names[shortName] = v.Name
	return shortName
}

// Create a short name and make sure there are no duplicates
func (v *Variable) getShortName(s *SpssWriter) string {
	short := strings.ToUpper(v.Name)

	if len(short) > 8 {
		short = short[:8]
	}

	i := 1

	for {
		_, found := s.names[short]

		if !found {
			break
		}

		iString := strconv.Itoa(i)

		short = short[:8-len(iString)] + iString
		i++
	}

	s.names[short] = v.Name

	return short
}

func (v *variable) segmentWidth(index int) int32 {
	if v.spssType == SpssTypeString {
		if len(v.labels) <= 0 {
			return int32(v.width)
		}
		// value labels cannot be larger than 40
		return 40
	}

	return 0
}

func (v *Variable) getSegments() int16 {
	return 1
}

func (v *Variable) getPrint() int8 {
	switch v.Type {
	case SpssTypeNumeric:
		return 5
	case SpssTypeDate:
		return 20
	case SpssTypeDatetime:
		return 22
	default: // string
		return 1
	}
}

func (v *Variable) setDefaultWidth() error {
	switch v.Type {
	case SpssTypeDate:
		v.Decimal = 0
		v.Width = 11
	case SpssTypeDatetime:
		v.Decimal = 0
		v.Width = 20
	case SpssTypeString:
		v.Decimal = 0
		v.Width = 40
	default:
		v.Width = 8 + int16(v.Decimal)
	}

	return nil
}

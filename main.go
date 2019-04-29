package gospss

import (
	"fmt"
	"log"
	"os"
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

// SpssConfig defines the structure for generating your SPSS file
type SpssConfig struct {
	Variables []Variable
	Values    [][]Value
}

// Variable defines the configuration for adding variables to the SPSS configuration
type Variable struct {
	Name    string
	Type    SpssType
	Measure SpssMeasure
	Decimal int8
	Width   int16
	Label   string
	Labels  []Label
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

var variables []variable
var values = make(map[string]string)
var shortNames = make(map[string]bool)
var index = int32(1)

// Generate will generate
func Generate(filepath string, config *SpssConfig) error {
	file, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, 0644)
	defer func() {
		file.Close()

		// Reset all global variables
		// TODO: Refactor to include in struct
		variables = []variable{}
		values = make(map[string]string)
		shortNames = make(map[string]bool)
		index = int32(1)
	}()

	if err != nil {
		return fmt.Errorf("Unable to create or open %s: %s", filepath, err.Error())
	}
	writer := newSpssWriter(file)
	log.Println("Start writing")

	// Parse all user provided variables and add them to the slice
	for _, v := range config.Variables {
		log.Printf("Index: %d", index)
		if err := addVariable(&v); err != nil {
			return fmt.Errorf("Error during variable parsing for variable %s: %s", v.Name, err.Error())
		}
	}

	if err := writer.start(); err != nil {
		return fmt.Errorf("Something went wrong: %s", err.Error())
	}

	// Write cases
	for _, va := range config.Values {
		for _, v := range va {
			values[v.Name] = v.Value
		}

		// log.Printf("Writing values: %+v", values)
		writer.writeValues(values)
		values = make(map[string]string)
	}

	writer.finish()

	return nil
}

var nameValidatorRegex = regexp.MustCompile(`(?si)^[a-z@][a-z0-9!._#@$]*[^\.]$`)

// createVariable converts the user given variable into something parsed and ready for the SpssWriter
func addVariable(v *Variable) error {
	// Check if name is empty
	if v.Name == "" {
		return fmt.Errorf("Name cannot be empty")
	}

	if len(v.Name) > 64 {
		return fmt.Errorf("Name cannot exceed 64 characters: %s", v.Name)
	}

	matched := nameValidatorRegex.MatchString(v.Name)

	if !matched {
		return fmt.Errorf("Name %s does not meet the requirements for SPSS, please refer to https://www.ibm.com/support/knowledgecenter/en/SSLVMB_24.0.0/spss/base/syn_variables_variable_names.html", v.Name)
	}

	// Check if name already exists (duplicate)
	for _, variable := range variables {
		if variable.name == v.Name {
			return fmt.Errorf("Cannot add variable with name %s since it already exists", v.Name)
		}
	}

	// Check decimal range
	if v.Decimal < 0 || v.Decimal > 16 {
		return fmt.Errorf("Cannot set decimal of %d, value must be between 0 and 16", v.Decimal)
	}

	if v.Width < 0 || v.Width > 32767 {
		return fmt.Errorf("Cannot set width of %d, value must be between 0 and 32676", v.Width)
	}

	if v.Type != SpssTypeString && v.Width > 40 {
		return fmt.Errorf("Cannot set width of %d on type %s, value must be between 1 and 40", v.Width, v.Type)
	}

	// Check if width is set, get the default otherwise
	if v.Width == 0 {
		if err := v.setDefaultWidth(); err != nil {
			return err
		}
	} else {
		if v.Width <= int16(v.Decimal) {
			return fmt.Errorf("Width cannot be less or equal to decimal")
		}
	}

	newVar := variable{
		index:     index,
		name:      v.Name,
		shortName: v.getShortName(),
		spssType:  v.Type,
		measure:   v.getMeasure(),
		decimal:   v.Decimal,
		width:     v.Width,
		format:    v.getPrint(),
		segments:  v.getSegments(),
		labels:    v.Labels,
		label:     v.Label,
	}

	variables = append(variables, newVar)

	for i := 0; i < int(newVar.segments); i++ {
		index += int32(elementCount(newVar.segmentWidth(i)))
	}

	return nil
}

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

// Create a short name and make sure there are no duplicates
func (v *Variable) getShortName() string {
	short := strings.ToUpper(v.Name)

	if len(short) > 8 {
		short = short[:8]
	}

	i := 1

	for {
		_, found := shortNames[short]

		if !found {
			break
		}

		iString := strconv.Itoa(i)

		short = short[:8-len(iString)] + iString
		i++
	}

	shortNames[short] = true

	return short
}

func (v *variable) segmentWidth(index int) int32 {
	if v.spssType == SpssTypeString {
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

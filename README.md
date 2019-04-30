# go-spss
Go library for generating SPSS files

**Inspired by and based on https://github.com/j0ran/xml2sav**

## Install

`go get -u github.com/jamiever/go-spss`

## Usage

1. Open a file to write to
```go
file, _ := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
```

2. Create a new SpssWriter
```go
spssWriter, _ := gospss.NewSpssWriter(file)
```

3. Write all variables
```go
spssWriter.AddVariable(&gospss.Variable{
    Name:    "VAR1",
    Type:    gospss.SpssTypeNumeric,
    Measure: gospss.SpssMeasureOrdinal,
    Decimal: int8(0),
    Width:   int16(10),
    Label:   "VARIABLE LABEL",
    Labels:  []gospss.Label{
        gospss.Label{
            Value: "1",
            Desc: "My Value Label",
        },
    },
})
```

4. Write all values
```go
values := make(map[string]string)

// Values must point to the right variable name
for _, variable := range variables {
    values[variable.Name] = "1"
}

w.AddValueRow(values)
```

5. Call the Finish func
```go
spssWriter.Finish()
```
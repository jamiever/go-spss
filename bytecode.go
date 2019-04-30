/*
Package gospss - converts a custom xml document to a SPSS binary file.
Copyright (C) 2016-2017 A.J. Jessurun
This file is part of xml2sav.
Xml2sav is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
Xml2sav is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.
You should have received a copy of the GNU General Public License
along with xml2sav.  If not, see <http://www.gnu.org/licenses/>.
*/
package gospss

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
)

type bytecodeWriter struct {
	io.Writer
	bias    float64
	command [8]byte
	index   int
	data    bytes.Buffer
}

func newBytecodeWriter(w io.Writer, bias float64) *bytecodeWriter {
	return &bytecodeWriter{Writer: w, bias: bias}
}

func (w *bytecodeWriter) checkAndWrite() error {
	if w.index >= len(w.command) {
		if _, err := w.Write(w.command[:]); err != nil {
			return err
		}
		if _, err := w.Write(w.data.Bytes()); err != nil {
			return err
		}
		w.index = 0
		w.data.Truncate(0)
	}
	return nil
}

func (w *bytecodeWriter) WriteMissing() error {
	w.command[w.index] = 255
	w.index++
	return w.checkAndWrite()
}

func (w *bytecodeWriter) WriteNumber(number float64) error {
	for i := 1.0; i <= 251; i++ {
		if number == i-w.bias {
			w.command[w.index] = byte(i)
			w.index++
			return w.checkAndWrite()
		}
	}
	w.command[w.index] = 253
	w.index++
	binary.Write(&w.data, endian, number)
	return w.checkAndWrite()
}

func (w *bytecodeWriter) WriteString(val string, elements int) error {
	for i := 0; i < elements; i++ {
		var p string
		if len(val) > 8 {
			p = val[:8]
			val = val[8:]
		} else {
			p = val
			val = ""
		}

		if len(p) < 8 {
			p += strings.Repeat(" ", 8-len(p))
		}

		if p == "        " {
			w.command[w.index] = 254
			w.index++
		} else {
			w.command[w.index] = 253
			w.index++
			w.data.Write([]byte(p))
		}

		if err := w.checkAndWrite(); err != nil {
			return err
		}
	}

	return nil
}

func (w *bytecodeWriter) Flush() error {
	for w.index < 8 {
		w.command[w.index] = 0
		w.index++
	}
	return w.checkAndWrite()
}

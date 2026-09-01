// Copyright 2015 geOps
// Authors: patrick.brosi@geops.de
//
// Use of this source code is governed by a GPL v2
// license that can be found in the LICENSE file

package gtfsparserwr

import (
	"fmt"
)

// A ParseError indicates an error during parsing
type ParseError struct {
	filename string
	line     int
	msg      string
}

func (e ParseError) Error() string {
	return fmt.Sprintf("%s:%d - %s", e.filename, e.line, e.msg)
}

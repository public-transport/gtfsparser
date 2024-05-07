// Copyright 2015 geOps
// Authors: patrick.brosi@geops.de
//
// Use of this source code is governed by a GPL v2
// license that can be found in the LICENSE file

package gtfs

import (
	"math"
	url "net/url"
	"time"
)

// A Stop object describes a single stop location
type Stop struct {
	Id                  string
	Code                string
	Name                string
	Desc                string
	Lat                 float32
	Lon                 float32
	Location_type       int8
	Wheelchair_boarding int8
	Zone_id             string
	Url                 *url.URL
	Parent_station      *Stop
	Translations        []*Translation
	Level               *Level
	Platform_code       string
	Timezone            *time.Location
}

// HasLatLon returns true if this Stop has a latitude and longitude
func (s *Stop) HasLatLon() bool {
	return !math.IsNaN(float64(s.Lat)) && !math.IsNaN(float64(s.Lon))

}

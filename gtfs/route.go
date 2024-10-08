// Copyright 2015 geOps
// Authors: patrick.brosi@geops.de
//
// Use of this source code is governed by a GPL v2
// license that can be found in the LICENSE file

package gtfs

import (
	url "net/url"
)

// A Route describes a group of trips that are presented to passangers as
// a single service
type Route struct {
	Id                  string
	Agency              *Agency
	Short_name          string
	Long_name           string
	Desc                string
	Type                int16
	Url                 *url.URL
	Color               string
	Text_color          string
	Sort_order          int
	Continuous_pickup   int8
	Continuous_drop_off int8
	Attributions        []*Attribution
}

func GetTypeFromExtended(t int16) int16 {
	switch t {
	case 2:
		fallthrough
	case 100:
		fallthrough
	case 101:
		fallthrough
	case 102:
		fallthrough
	case 103:
		fallthrough
	case 104:
		fallthrough
	case 105:
		fallthrough
	case 106:
		fallthrough
	case 107:
		fallthrough
	case 108:
		fallthrough
	case 109:
		fallthrough
	case 110:
		fallthrough
	case 111:
		fallthrough
	case 112:
		fallthrough
	case 113:
		fallthrough
	case 114:
		fallthrough
	case 115:
		fallthrough
	case 117:
		fallthrough
	case 300:
		fallthrough
	case 1503:
		return 2 // rail
	case 3:
		return 3 // bus
	case 200:
		fallthrough
	case 201:
		fallthrough
	case 202:
		fallthrough
	case 203:
		fallthrough
	case 204:
		fallthrough
	case 205:
		fallthrough
	case 206:
		fallthrough
	case 207:
		fallthrough
	case 208:
		fallthrough
	case 209:
		return 3 // bus
	case 700:
		fallthrough
	case 701:
		fallthrough
	case 702:
		fallthrough
	case 703:
		fallthrough
	case 704:
		fallthrough
	case 705:
		fallthrough
	case 706:
		fallthrough
	case 707:
		fallthrough
	case 708:
		fallthrough
	case 709:
		fallthrough
	case 710:
		fallthrough
	case 711:
		fallthrough
	case 712:
		fallthrough
	case 713:
		fallthrough
	case 714:
		fallthrough
	case 715:
		fallthrough
	case 716:
		fallthrough
	case 717:
		fallthrough
	case 1500:
		fallthrough
	case 1501:
		fallthrough
	case 1505:
		fallthrough
	case 1506:
		fallthrough
	case 1507:
		return 3 // bus
	case 1:
		fallthrough
	case 400:
		fallthrough
	case 401:
		fallthrough
	case 402:
		fallthrough
	case 403:
		fallthrough
	case 404:
		fallthrough
	case 500:
		fallthrough
	case 600:
		return 1 // subway
	case 0:
		fallthrough
	case 900:
		fallthrough
	case 901:
		fallthrough
	case 902:
		fallthrough
	case 903:
		fallthrough
	case 904:
		fallthrough
	case 905:
		fallthrough
	case 906:
		return 0 // tram
	case 4:
		fallthrough
	case 1000:
		fallthrough
	case 1001:
		fallthrough
	case 1002:
		fallthrough
	case 1003:
		fallthrough
	case 1004:
		fallthrough
	case 1005:
		fallthrough
	case 1006:
		fallthrough
	case 1007:
		fallthrough
	case 1008:
		fallthrough
	case 1009:
		fallthrough
	case 1010:
		fallthrough
	case 1011:
		fallthrough
	case 1012:
		fallthrough
	case 1013:
		fallthrough
	case 1014:
		fallthrough
	case 1015:
		fallthrough
	case 1016:
		fallthrough
	case 1017:
		fallthrough
	case 1018:
		fallthrough
	case 1019:
		fallthrough
	case 1020:
		fallthrough
	case 1021:
		fallthrough
	case 1200:
		fallthrough
	case 1502:
		return 4 // ferry
	case 6:
		fallthrough
	case 1300:
		fallthrough
	case 1301:
		fallthrough
	case 1304:
		fallthrough
	case 1306:
		fallthrough
	case 1307:
		return 6 // gondola
	case 7:
		fallthrough
	case 116:
		fallthrough
	case 1303:
		fallthrough
	case 1302:
		fallthrough
	case 1400:
		return 7 // funicular
	case 5:
		return 5 // cable car
	case 11:
		fallthrough
	case 800:
		return 11
	case 12:
		fallthrough
	case 405:
		return 12
	}

	return 2 // fallback
}

func GetSupportedExtendedTypeFromExtended(t int16) int16 {
	switch t {
	case 104:
		return 100
	case 110:
		return 100
	case 111:
		return 100
	case 112:
		return 100
	case 113:
		return 100
	case 114:
		return 100
	case 115:
		return 100
	case 116:
		return 100
	case 117:
		return 100
	case 203:
		return 200
	case 204:
		return 200
	case 205:
		return 200
	case 206:
		return 200
	case 207:
		return 200
	case 208:
		return 200
	case 209:
		return 200
	case 300:
		return 109
	case 404:
		return 400
	case 703:
		return 700
	case 705:
		return 700
	case 706:
		return 700
	case 707:
		return 700
	case 708:
		return 700
	case 709:
		return 700
	case 710:
		return 700
	case 711:
		return 700
	case 712:
		return 700
	case 713:
		return 700
	case 714:
		return 700
	case 716:
		return 700
	case 901:
		return 900
	case 902:
		return 900
	case 903:
		return 900
	case 904:
		return 900
	case 905:
		return 900
	case 906:
		return 900
	case 1302:
		return 900
	default:
		return t
	}

	return t // fallback
}

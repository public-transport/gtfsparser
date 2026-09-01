package gtfsparserwr

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	url "net/url"
	"sort"
	"strings"

	"github.com/Leocraft1/gtfsparser-with-reader/gtfs"
)

// Parse the GTFS data from the specified io.Reader into the feed
func (feed *Feed) ParseReader(reader io.Reader) error {
	return feed.PrefixParseReader(reader, "")
}

// Parse the GTFS data from the specified io.Reader into the feed, with prefix
func (feed *Feed) PrefixParseReader(reader io.Reader, prefix string) error {
	var e error

	//Reads the reader parameter and uncompresses it if it's a directory
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("reading gtfs archive: %w", err)
	}

	zip_reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("opening gtfs zip: %w", err)
	}

	// holds stops that are dropped because of geometric filtering.
	// if these are referenced later, we quietly ignore the error like
	// with -De
	geofilteredStops := make(map[string]struct{}, 0)

	// holds routes that are dropped because of MOT filtering.
	// if these are referenced later, we quietly ignore the error like
	// with -De
	filteredRoutes := make(map[string]struct{}, 0)

	// holds trips that are dropped because of MOT filtering.
	// if these are referenced later, we quietly ignore the error like
	// with -De
	filteredTrips := make(map[string]struct{}, 0)

	e = feed.withFile(zip_reader, "agency.txt", true, func(r io.Reader) error {
		return feed.parseAgenciesReader(r, prefix, feed.Opts.EmptyAgencyUrlRepl)
	})
	if e == nil {
		e = feed.withFile(zip_reader, "feed_info.txt", false, feed.parseFeedInfosReader)
	}
	if e == nil {
		e = feed.withFile(zip_reader, "levels.txt", false, func(r io.Reader) error {
			return feed.parseLevelsReader(r, prefix)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "stops.txt", true, func(r io.Reader) error {
			return feed.parseStopsReader(r, prefix, geofilteredStops)
		})
	}
	if e == nil && !feed.Opts.DropShapes {
		e = feed.withFile(zip_reader, "shapes.txt", false, func(r io.Reader) error {
			return feed.reserveShapesReader(r, prefix)
		})
		if e == nil {
			e = feed.withFile(zip_reader, "shapes.txt", false, func(r io.Reader) error {
				return feed.parseShapesReader(r, prefix)
			})
		}
	}
	if e == nil {
		e = feed.withFile(zip_reader, "routes.txt", true, func(r io.Reader) error {
			return feed.parseRoutesReader(r, prefix, filteredRoutes)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "calendar.txt", false, func(r io.Reader) error {
			return feed.parseCalendarReader(r, prefix)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "calendar_dates.txt", false, func(r io.Reader) error {
			return feed.parseCalendarDatesReader(r, prefix)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "trips.txt", true, func(r io.Reader) error {
			return feed.parseTripsReader(r, prefix, filteredRoutes, filteredTrips)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "stop_times.txt", true, func(r io.Reader) error {
			return feed.reserveStopTimesReader(r, prefix)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "stop_times.txt", true, func(r io.Reader) error {
			return feed.parseStopTimesReader(r, prefix, geofilteredStops, filteredTrips)
		})
	}
	if e == nil {
		// Remove reservation markers
		for tripId, t := range feed.Trips {
			if t != nil && t.Id != tripId {
				t.Id = tripId
				t.StopTimes = make(gtfs.StopTimes, 0)
			}
		}
	}
	if e == nil {
		e = feed.withFile(zip_reader, "fare_attributes.txt", false, func(r io.Reader) error {
			return feed.parseFareAttributesReader(r, prefix)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "fare_rules.txt", false, func(r io.Reader) error {
			return feed.parseFareAttributeRulesReader(r, prefix, filteredRoutes)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "frequencies.txt", false, func(r io.Reader) error {
			return feed.parseFrequenciesReader(r, prefix, filteredTrips)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "transfers.txt", false, func(r io.Reader) error {
			return feed.parseTransfersReader(r, prefix, geofilteredStops)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "pathways.txt", false, func(r io.Reader) error {
			return feed.parsePathwaysReader(r, prefix, geofilteredStops)
		})
	}
	if e == nil {
		e = feed.withFile(zip_reader, "attributions.txt", false, func(r io.Reader) error {
			return feed.parseAttributionsReader(r, prefix, filteredRoutes, filteredTrips)
		})
	}

	// Nessun file/zip handle da chiudere qui: ogni chiamata a withFile
	// apre e chiude il proprio reader internamente.

	if e == nil && (!feed.Opts.DateFilterStart.IsEmpty() || !feed.Opts.DateFilterEnd.IsEmpty()) {
		feed.filterServices()
	}

	if e == nil && feed.Opts.DropSingleStopTrips {
		for _, t := range feed.Trips {
			if len(t.StopTimes) < 2 {
				feed.DeleteTrip(t.Id)
			}
		}
	}

	//At this point, all possible GTFS is parsed, if there are extra files, it puts them under AdditionalFiles or AdditionalCsvFiles
	//Deprecated because of lack of volunty to adapt it

	//Close open readers
	if feed.zipFileCloser != nil {
		feed.zipFileCloser.Close()
		feed.zipFileCloser = nil
	}

	if feed.curFileHandle != nil {
		feed.curFileHandle.Close()
		feed.curFileHandle = nil
	}

	if !feed.Opts.DateFilterStart.IsEmpty() || !feed.Opts.DateFilterEnd.IsEmpty() {
		feed.filterServices()
	}

	if feed.Opts.DropSingleStopTrips {
		for _, t := range feed.Trips {
			if len(t.StopTimes) < 2 {
				feed.DeleteTrip(t.Id)
			}
		}
	}

	return e
}

func (feed *Feed) parseAgenciesReader(file io.Reader, prefix string, fallbackUrl string) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"agency.txt", reader.Curline, r.(error).Error()}
		}
	}()
	var e error

	var record []string
	flds := AgencyFields{
		agencyId:       reader.headeridx.GetFldId("agency_id", -1),
		agencyName:     reader.headeridx.GetFldId("agency_name", -2),
		agencyUrl:      reader.headeridx.GetFldId("agency_url", -3),
		agencyTimezone: reader.headeridx.GetFldId("agency_timezone", -4),
		agencyLang:     reader.headeridx.GetFldId("agency_lang", -5),
		agencyPhone:    reader.headeridx.GetFldId("agency_phone", -6),
		agencyFareUrl:  reader.headeridx.GetFldId("agency_fare_url", -7),
		agencyEmail:    reader.headeridx.GetFldId("agency_email", -8),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}
	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		agency, e := createAgency(record, flds, feed, prefix)
		if e == nil {
			if _, ok := feed.Agencies[agency.Id]; ok {
				e = errors.New("ID collision, agency_id '" + agency.Id + "' already used")
			}
		}

		if e == nil {
			existingAgId := ""

			for k := range feed.Agencies {
				existingAgId = k
				break
			}

			if len(existingAgId) > 0 && feed.Agencies[existingAgId].Timezone.String() != agency.Timezone.String() {
				e = fmt.Errorf("Agency '%s' has a different timezone (%s) than existing agencies (%s). All agencies must have the same timezone.", agency.Id, agency.Timezone.String(), feed.Agencies[existingAgId].Timezone.String())
			}
		}

		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedAgencies++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		if agency.Url == nil {
			fbUrl, err := url.Parse(fallbackUrl)
			if err != nil {
				panic(err)
			}
			agency.Url = fbUrl
		}

		feed.Agencies[agency.Id] = agency

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.AgenciesAddFlds[reader.header[i]]; !ok {
					feed.AgenciesAddFlds[reader.header[i]] = make(map[string]string)
				}

				feed.AgenciesAddFlds[reader.header[i]][agency.Id] = record[i]
			}
		}
	}

	feed.ColOrders.Agencies = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseStopsReader(file io.Reader, prefix string, geofiltered map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"stops.txt", reader.Curline, r.(error).Error()}
		}
	}()
	var e error

	var record []string
	flds := StopFields{
		stopId:             reader.headeridx.GetFldId("stop_id", -1),
		stopCode:           reader.headeridx.GetFldId("stop_code", -2),
		locationType:       reader.headeridx.GetFldId("location_type", -3),
		stopName:           reader.headeridx.GetFldId("stop_name", -4),
		stopDesc:           reader.headeridx.GetFldId("stop_desc", -5),
		stopLat:            reader.headeridx.GetFldId("stop_lat", -6),
		stopLon:            reader.headeridx.GetFldId("stop_lon", -7),
		zoneId:             reader.headeridx.GetFldId("zone_id", -8),
		stopUrl:            reader.headeridx.GetFldId("stop_url", -9),
		parentStation:      reader.headeridx.GetFldId("parent_station", -10),
		stopTimezone:       reader.headeridx.GetFldId("stop_timezone", -11),
		levelId:            reader.headeridx.GetFldId("level_id", -12),
		platformCode:       reader.headeridx.GetFldId("platform_code", -13),
		wheelchairBoarding: reader.headeridx.GetFldId("wheelchair_boarding", -14),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	parentStopIds := make(map[string]string, 0)
	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		stop, parentId, e := createStop(record, flds, feed, prefix)
		if e == nil {
			if _, ok := feed.Stops[stop.Id]; ok {
				e = errors.New("ID collision, stop_id '" + stop.Id + "' already used")
			}
		}
		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedStops++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		// check if any defined PolygonFilter contains the stop
		contains := true
		for _, poly := range feed.Opts.PolygonFilter {
			contains = false
			if poly.PolyContains(float64(stop.Lon), float64(stop.Lat)) {
				contains = true
				break
			}
		}

		if !contains {
			geofiltered[stop.Id] = struct{}{}
			continue
		}

		if len(parentId) > len(prefix) {
			parentStopIds[stop.Id] = parentId
		}

		feed.Stops[stop.Id] = stop

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.StopsAddFlds[reader.header[i]]; !ok {
					feed.StopsAddFlds[reader.header[i]] = make(map[string]string)
				}

				feed.StopsAddFlds[reader.header[i]][stop.Id] = record[i]
			}
		}
	}

	feed.ColOrders.Stops = append([]string(nil), reader.header...)

	// write the parent stop ids
	for id, pid := range parentStopIds {
		pstop, ok := feed.Stops[pid]
		if !ok {
			locErr := errors.New("(for stop id " + id + ") No station with id " + pid + " found, cannot use as parent station here")
			_, wasFiltered := geofiltered[pid]

			// note: if type >= 2, a parent Id is *required*
			if wasFiltered && feed.Stops[id].Location_type < 2 {
				// continue, the default value "nil" has already be written above
				continue
			} else if feed.Opts.UseDefValueOnError && feed.Stops[id].Location_type < 2 {
				// continue, the default value "nil" has already be written above
				feed.warn(locErr)
				continue
			} else if feed.Opts.DropErroneous {
				// delete the erroneous entry
				delete(feed.Stops, id)
				feed.ErrorStats.DroppedStops++
				feed.warn(locErr)
				continue
			} else {
				return locErr
			}
		}

		if (feed.Stops[id].Location_type == 0 || feed.Stops[id].Location_type == 2 || feed.Stops[id].Location_type == 3) && pstop.Location_type != 1 {
			locErr := fmt.Errorf("(for stop id %s) Station with id %s has location_type=%d, cannot use as parent station here for stop with location_type=%d (must be 1)", id, pid, pstop.Location_type, feed.Stops[id].Location_type)
			if feed.Opts.UseDefValueOnError && !(feed.Stops[id].Location_type == 2 || feed.Stops[id].Location_type == 3) {
				// continue, the default value "nil" has already be written above
				feed.warn(locErr)
				continue
			} else if feed.Opts.DropErroneous {
				// delete the erroneous entry
				delete(feed.Stops, id)
				feed.ErrorStats.DroppedStops++
				feed.warn(locErr)
				continue
			} else {
				return (locErr)
			}
		}

		if feed.Stops[id].Location_type == 4 && pstop.Location_type != 0 {
			locErr := fmt.Errorf("(for stop id %s) Station with id %s has location_type=%d, cannot use as parent station here for stop with location_type=4 (boarding area), which expects a parent station with location_type=0 (stop/platform)", id, pid, pstop.Location_type)
			if feed.Opts.DropErroneous {
				// delete the erroneous entry
				delete(feed.Stops, id)
				feed.ErrorStats.DroppedStops++
				feed.warn(locErr)
				continue
			} else {
				panic(locErr)
			}
		}

		feed.Stops[id].Parent_station = pstop
	}

	return e
}

func (feed *Feed) parseRoutesReader(file io.Reader, prefix string, filtered map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"routes.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := RouteFields{
		routeId:           reader.headeridx.GetFldId("route_id", -1),
		agencyId:          reader.headeridx.GetFldId("agency_id", -2),
		routeShortName:    reader.headeridx.GetFldId("route_short_name", -3),
		routeLongName:     reader.headeridx.GetFldId("route_long_name", -4),
		routeDesc:         reader.headeridx.GetFldId("route_desc", -5),
		routeType:         reader.headeridx.GetFldId("route_type", -6),
		routeUrl:          reader.headeridx.GetFldId("route_url", -7),
		routeColor:        reader.headeridx.GetFldId("route_color", -8),
		routeTextColor:    reader.headeridx.GetFldId("route_text_color", -9),
		routeSortOrder:    reader.headeridx.GetFldId("route_sort_order", -10),
		continuousDropOff: reader.headeridx.GetFldId("continuous_drop_off", -11),
		continuousPickup:  reader.headeridx.GetFldId("continuous_pickup", -12),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		route, e := createRoute(record, flds, feed, prefix)
		if e == nil {
			if _, ok := feed.Routes[route.Id]; ok {
				e = errors.New("ID collision, route_id '" + route.Id + "' already used")
			}
		}
		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedRoutes++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		if feed.Opts.UseStandardRouteTypes {
			route.Type = gtfs.GetTypeFromExtended(route.Type)
		}

		if feed.Opts.UseGoogleSupportedRouteTypes {
			route.Type = gtfs.GetSupportedExtendedTypeFromExtended(route.Type)
		}

		if len(feed.Opts.MOTFilter) != 0 {
			if _, ok := feed.Opts.MOTFilter[route.Type]; !ok {
				filtered[route.Id] = struct{}{}
				continue
			}
		}

		if len(feed.Opts.MOTFilterNeg) != 0 {
			if _, ok := feed.Opts.MOTFilterNeg[route.Type]; ok {
				filtered[route.Id] = struct{}{}
				continue
			}
		}

		if feed.Opts.DryRun {
			feed.Routes[route.Id] = route
		} else {
			feed.Routes[route.Id] = route

			for _, i := range addFlds {
				if i < len(record) {
					if _, ok := feed.RoutesAddFlds[reader.header[i]]; !ok {
						feed.RoutesAddFlds[reader.header[i]] = make(map[string]string)
					}

					feed.RoutesAddFlds[reader.header[i]][route.Id] = record[i]
				}
			}
		}
	}

	feed.ColOrders.Routes = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseCalendarReader(file io.Reader, prefix string) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"calendar.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := CalendarFields{
		serviceId: reader.headeridx.GetFldId("service_id", -1),
		monday:    reader.headeridx.GetFldId("monday", -2),
		tuesday:   reader.headeridx.GetFldId("tuesday", -3),
		wednesday: reader.headeridx.GetFldId("wednesday", -4),
		thursday:  reader.headeridx.GetFldId("thursday", -5),
		friday:    reader.headeridx.GetFldId("friday", -6),
		saturday:  reader.headeridx.GetFldId("saturday", -7),
		sunday:    reader.headeridx.GetFldId("sunday", -8),
		startDate: reader.headeridx.GetFldId("start_date", -9),
		endDate:   reader.headeridx.GetFldId("end_date", -10),
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		service, e := createServiceFromCalendar(record, flds, feed, prefix)

		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedServices++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		// if service was parsed in-place, nil was returned
		if service != nil {
			if feed.Opts.DryRun {
				feed.Services[service.Id()] = nil
			} else {
				feed.Services[service.Id()] = service

				// check if service is completely out of range
				if !feed.Opts.DateFilterStart.IsEmpty() && service.End_date().GetTime().Before(feed.Opts.DateFilterStart.GetTime()) || !feed.Opts.DateFilterEnd.IsEmpty() && service.Start_date().GetTime().After(feed.Opts.DateFilterEnd.GetTime()) {
					service.SetRawDaymap(0)
				} else {
					// we overlap, there are now two cases:

					// 1. A start date is defined, and the service starts before the start time. Set the start time to the new start time
					if !feed.Opts.DateFilterStart.IsEmpty() && service.Start_date().GetTime().Before(feed.Opts.DateFilterStart.GetTime()) {
						service.SetStart_date(feed.Opts.DateFilterStart)
						// note: because of the check above, End_date is guaranteed to >= DateFilterStart, so our service remains valid
					}

					// 2. An end date is defined, and the service ends after the start time. Set the end  time to the new end time
					if !feed.Opts.DateFilterEnd.IsEmpty() && service.End_date().GetTime().After(feed.Opts.DateFilterEnd.GetTime()) {
						service.SetEnd_date(feed.Opts.DateFilterEnd)
						// note: because of the check above, Start_date is guaranteed to <= DateFilterEnd, so our service remains valid
					}
				}
			}
		}
	}

	feed.ColOrders.Calendar = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseCalendarDatesReader(file io.Reader, prefix string) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"calendar_dates.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := CalendarDatesFields{
		serviceId:     reader.headeridx.GetFldId("service_id", -1),
		exceptionType: reader.headeridx.GetFldId("exception_type", -2),
		date:          reader.headeridx.GetFldId("date", -3),
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		service, e := createServiceFromCalendarDates(record, flds, feed, feed.Opts.DateFilterStart, feed.Opts.DateFilterEnd, prefix)

		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedServices++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		// if service was parsed in-place, nil was returned
		if service != nil {
			if feed.Opts.DryRun {
				feed.Services[service.Id()] = nil
			} else {
				feed.Services[service.Id()] = service
			}
		}
	}

	feed.ColOrders.CalendarDates = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseTripsReader(file io.Reader, prefix string, filteredRoutes map[string]struct{}, filteredTrips map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"trips.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := TripFields{
		tripId:               reader.headeridx.GetFldId("trip_id", -1),
		routeId:              reader.headeridx.GetFldId("route_id", -2),
		serviceId:            reader.headeridx.GetFldId("service_id", -3),
		tripHeadsign:         reader.headeridx.GetFldId("trip_headsign", -4),
		tripShortName:        reader.headeridx.GetFldId("trip_short_name", -5),
		directionId:          reader.headeridx.GetFldId("direction_id", -6),
		blockId:              reader.headeridx.GetFldId("block_id", -7),
		shapeId:              reader.headeridx.GetFldId("shape_id", -8),
		wheelchairAccessible: reader.headeridx.GetFldId("wheelchair_accessible", -9),
		bikesAllowed:         reader.headeridx.GetFldId("bikes_allowed", -10),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		trip, e := createTrip(record, flds, feed, prefix)

		tripId := ""

		if e == nil {
			tripId = trip.Id
			trip.Id = ""
			dummy := gtfs.StopTime{}
			dummy.SetSequence(0)
			trip.StopTimes = append(trip.StopTimes, dummy)
			if _, ok := feed.Trips[tripId]; ok {
				e = errors.New("ID collision, trip_id '" + tripId + "' already used")
			}
		} else {
			routeNotFoundErr, routeNotFound := e.(*RouteNotFoundErr)
			wasFiltered := false
			if routeNotFound {
				_, wasFiltered = filteredRoutes[routeNotFoundErr.RouteId()]
			}

			if wasFiltered {
				filteredTrips[routeNotFoundErr.PayloadId()] = struct{}{}
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedTrips++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}
		feed.Trips[tripId] = trip

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.TripsAddFlds[reader.header[i]]; !ok {
					feed.TripsAddFlds[reader.header[i]] = make(map[string]string)
				}

				feed.TripsAddFlds[reader.header[i]][tripId] = record[i]
			}
		}
	}

	feed.ColOrders.Trips = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) reserveShapesReader(file io.Reader, prefix string) (err error) {
	if feed.Opts.DropShapes {
		return
	}

	data, e := io.ReadAll(file)
    if e != nil {
        return errors.New("could not read shapes.txt")
    }

    reader := NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"shapes.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var record []string
	flds := ShapeFields{
		shapeId:           reader.headeridx.GetFldId("shape_id", -1),
		shapeDistTraveled: reader.headeridx.GetFldId("shape_dist_traveled", -2),
		shapePtLat:        reader.headeridx.GetFldId("shape_pt_lat", -3),
		shapePtLon:        reader.headeridx.GetFldId("shape_pt_lon", -4),
		shapePtSequence:   reader.headeridx.GetFldId("shape_pt_sequence", -5),
	}

    reader = NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, feed.Opts.AssumeCleanCsv && !feed.Opts.KeepAddFlds)
	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		e := reserveShapePoint(record, flds, feed, prefix)
		if e != nil {
			if feed.Opts.DropErroneous {
				continue
			} else {
				panic(e)
			}
		}
	}

	return e
}

func (feed *Feed) parseShapesReader(file io.Reader, prefix string) (err error) {
	if feed.Opts.DropShapes {
		return
	}

	data, e := io.ReadAll(file)
    if e != nil {
        return errors.New("could not read shapes.txt")
    }

    reader := NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"shapes.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var record []string
	flds := ShapeFields{
		shapeId:           reader.headeridx.GetFldId("shape_id", -1),
		shapeDistTraveled: reader.headeridx.GetFldId("shape_dist_traveled", -2),
		shapePtLat:        reader.headeridx.GetFldId("shape_pt_lat", -3),
		shapePtLon:        reader.headeridx.GetFldId("shape_pt_lon", -4),
		shapePtSequence:   reader.headeridx.GetFldId("shape_pt_sequence", -5),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

    reader = NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, feed.Opts.AssumeCleanCsv && !feed.Opts.KeepAddFlds)

	i := 0

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		i += 1

		shape, sp, e := createShapePoint(record, flds, feed, prefix)

		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedShapes++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		} else if sp != nil {
			for _, i := range addFlds {
				if i < len(record) {
					if _, ok := feed.ShapesAddFlds[reader.header[i]]; !ok {
						feed.ShapesAddFlds[reader.header[i]] = make(map[string]map[int]string)
					}
					if _, ok := feed.ShapesAddFlds[reader.header[i]][shape.Id]; !ok {
						feed.ShapesAddFlds[reader.header[i]][shape.Id] = make(map[int]string)
					}

					feed.ShapesAddFlds[reader.header[i]][shape.Id][int(sp.Sequence)] = record[i]
				}
			}
		}
	}

	feed.ColOrders.Shapes = append([]string(nil), reader.header...)

	if e == nil {
		// sort points in shapes, drop empty shapes
		for id, shape := range feed.Shapes {
			if len(shape.Points) == 0 {
				loce := fmt.Errorf("shape #%s has no points", id)
				if feed.Opts.DropErroneous || len(feed.Opts.PolygonFilter) > 0 {
					// dont warn here, because this can only happen if a shape point
					// has been deleted before
					delete(feed.Shapes, id)
					continue
				} else {
					panic(loce)
				}
			}
			sort.Sort(shape.Points)
			e = feed.checkShapeMeasure(shape, &feed.Opts)
			feed.NumShpPoints += len(shape.Points)
			if e != nil {
				break
			}
		}
		if feed.Opts.DryRun {
			// clear space
			for id := range feed.Shapes {
				feed.Shapes[id] = nil
			}
		}
	}

	return e
}

func (feed *Feed) reserveStopTimesReader(file io.Reader, prefix string) (err error) {
	data, e := io.ReadAll(file)
    if e != nil {
        return errors.New("could not read stop_times.txt")
    }

	reader := NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"stop_times.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var record []string
	flds := StopTimeFields{
		tripId:            reader.headeridx.GetFldId("trip_id", -1),
		stopId:            reader.headeridx.GetFldId("stop_id", -2),
		arrivalTime:       reader.headeridx.GetFldId("arrival_time", -3),
		departureTime:     reader.headeridx.GetFldId("departure_time", -4),
		stopSequence:      reader.headeridx.GetFldId("stop_sequence", -5),
		stopHeadsign:      reader.headeridx.GetFldId("stop_headsign", -6),
		pickupType:        reader.headeridx.GetFldId("pickup_type", -7),
		dropOffType:       reader.headeridx.GetFldId("drop_off_type", -8),
		continuousDropOff: reader.headeridx.GetFldId("continuous_drop_off", -9),
		continuousPickup:  reader.headeridx.GetFldId("continuous_pickup", -10),
		shapeDistTraveled: reader.headeridx.GetFldId("shape_dist_traveled", -11),
		timepoint:         reader.headeridx.GetFldId("timepoint", -12),
	}

	reader = NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, feed.Opts.AssumeCleanCsv && flds.stopHeadsign < 0 && !feed.Opts.KeepAddFlds)
    
	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
        reserveStopTime(record, flds, feed, prefix)
    }

	return e
}

func (feed *Feed) parseStopTimesReader(file io.Reader, prefix string, geofiltered map[string]struct{}, filteredTrips map[string]struct{}) (err error) {
	data, e := io.ReadAll(file)
    if e != nil {
        return errors.New("could not read stop_times.txt")
    }

	reader := NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, feed.Opts.AssumeCleanCsv && !feed.Opts.KeepAddFlds)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"stop_times.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var record []string
	flds := StopTimeFields{
		tripId:            reader.headeridx.GetFldId("trip_id", -1),
		stopId:            reader.headeridx.GetFldId("stop_id", -2),
		arrivalTime:       reader.headeridx.GetFldId("arrival_time", -3),
		departureTime:     reader.headeridx.GetFldId("departure_time", -4),
		stopSequence:      reader.headeridx.GetFldId("stop_sequence", -5),
		stopHeadsign:      reader.headeridx.GetFldId("stop_headsign", -6),
		pickupType:        reader.headeridx.GetFldId("pickup_type", -7),
		dropOffType:       reader.headeridx.GetFldId("drop_off_type", -8),
		continuousDropOff: reader.headeridx.GetFldId("continuous_drop_off", -9),
		continuousPickup:  reader.headeridx.GetFldId("continuous_pickup", -10),
		shapeDistTraveled: reader.headeridx.GetFldId("shape_dist_traveled", -11),
		timepoint:         reader.headeridx.GetFldId("timepoint", -12),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	reader = NewCsvParser(bytes.NewReader(data), feed.Opts.DropErroneous, feed.Opts.AssumeCleanCsv && flds.stopHeadsign < 0)

	i := 0

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		i += 1

		trip, stopTimeSeq, e := createStopTime(record, &flds, feed, prefix)

		if e != nil {
			wasFiltered := false
			stopNotFoundErr, stopNotFound := e.(*StopNotFoundErr)
			if stopNotFound {
				_, wasFiltered = geofiltered[stopNotFoundErr.StopId()]
			}

			tripNotFoundErr, tripNotFound := e.(*TripNotFoundErr)
			if tripNotFound {
				_, wasFiltered = filteredTrips[tripNotFoundErr.TripId()]
			}

			if wasFiltered {
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedStopTimes++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		} else {
			for _, i := range addFlds {
				if i < len(record) {
					if _, ok := feed.StopTimesAddFlds[reader.header[i]]; !ok {
						feed.StopTimesAddFlds[reader.header[i]] = make(map[string]map[int]string)
					}
					if _, ok := feed.StopTimesAddFlds[reader.header[i]][trip.Id]; !ok {
						feed.StopTimesAddFlds[reader.header[i]][trip.Id] = make(map[int]string)
					}

					feed.StopTimesAddFlds[reader.header[i]][trip.Id][stopTimeSeq] = record[i]
				}
			}
		}
	}

	feed.ColOrders.StopTimes = append([]string(nil), reader.header...)

	if e == nil {
		// sort stoptimes in trips
		for _, trip := range feed.Trips {
			sort.Sort(trip.StopTimes)
			e = feed.checkStopTimeMeasure(trip, &feed.Opts)
			feed.NumStopTimes += len(trip.StopTimes)
			if e != nil {
				break
			}
		}
	}

	return e
}

func (feed *Feed) parseFrequenciesReader(file io.Reader, prefix string, filteredTrips map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"frequencies.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := FrequencyFields{
		tripId:      reader.headeridx.GetFldId("trip_id", -1),
		exactTimes:  reader.headeridx.GetFldId("exact_times", -2),
		startTime:   reader.headeridx.GetFldId("start_time", -3),
		endTime:     reader.headeridx.GetFldId("end_time", -4),
		headwaySecs: reader.headeridx.GetFldId("headway_secs", -5),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		trip, freq, e := createFrequency(record, flds, feed, prefix)
		if e != nil {
			tripNotFoundErr, tripNotFound := e.(*TripNotFoundErr)
			wasFiltered := false
			if tripNotFound {
				_, wasFiltered = filteredTrips[tripNotFoundErr.TripId()]
			}

			if wasFiltered {
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedFrequencies++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.FrequenciesAddFlds[reader.header[i]]; !ok {
					feed.FrequenciesAddFlds[reader.header[i]] = make(map[string]map[*gtfs.Frequency]string)
				}
				if _, ok := feed.FrequenciesAddFlds[reader.header[i]][trip.Id]; !ok {
					feed.FrequenciesAddFlds[reader.header[i]][trip.Id] = make(map[*gtfs.Frequency]string)
				}

				feed.FrequenciesAddFlds[reader.header[i]][trip.Id][freq] = record[i]
			}
		}
	}

	feed.ColOrders.Frequencies = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseFareAttributesReader(file io.Reader, prefix string) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"fare_attributes.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := FareAttributeFields{
		fareId:           reader.headeridx.GetFldId("fare_id", -1),
		price:            reader.headeridx.GetFldId("price", -2),
		currencyType:     reader.headeridx.GetFldId("currency_type", -3),
		paymentMethod:    reader.headeridx.GetFldId("payment_method", -4),
		transfers:        reader.headeridx.GetFldId("transfers", -5),
		transferDuration: reader.headeridx.GetFldId("transfer_duration", -6),
		agencyId:         reader.headeridx.GetFldId("agency_id", -7),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		fa, e := createFareAttribute(record, flds, feed, prefix)
		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedFareAttributes++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}
		feed.FareAttributes[fa.Id] = fa

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.FareAttributesAddFlds[reader.header[i]]; !ok {
					feed.FareAttributesAddFlds[reader.header[i]] = make(map[string]string)
				}

				feed.FareAttributesAddFlds[reader.header[i]][fa.Id] = record[i]
			}
		}
	}

	feed.ColOrders.FareAttributes = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseFareAttributeRulesReader(file io.Reader, prefix string, filteredRoutes map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"fare_rules.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := FareRuleFields{
		fareId:        reader.headeridx.GetFldId("fare_id", -1),
		routeId:       reader.headeridx.GetFldId("route_id", -2),
		originId:      reader.headeridx.GetFldId("origin_id", -3),
		destinationId: reader.headeridx.GetFldId("destination_id", -4),
		containsId:    reader.headeridx.GetFldId("contains_id", -5),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		fare, rule, e := createFareRule(record, flds, feed, prefix)
		if e != nil {
			routeNotFoundErr, routeNotFound := e.(*RouteNotFoundErr)
			wasFiltered := false
			if routeNotFound {
				_, wasFiltered = filteredRoutes[routeNotFoundErr.RouteId()]
			}

			if wasFiltered {
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedFareAttributeRules++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		} else {
			for _, i := range addFlds {
				if i < len(record) {
					if _, ok := feed.FareRulesAddFlds[reader.header[i]]; !ok {
						feed.FareRulesAddFlds[reader.header[i]] = make(map[string]map[*gtfs.FareAttributeRule]string)
					}
					if _, ok := feed.FareRulesAddFlds[reader.header[i]][fare.Id]; !ok {
						feed.FareRulesAddFlds[reader.header[i]][fare.Id] = make(map[*gtfs.FareAttributeRule]string)
					}

					feed.FareRulesAddFlds[reader.header[i]][fare.Id][rule] = record[i]
				}
			}

		}
	}

	feed.ColOrders.FareAttributeRules = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseTransfersReader(file io.Reader, prefix string, geofiltered map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"transfers.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := TransferFields{
		FromStopId:      reader.headeridx.GetFldId("from_stop_id", -1),
		ToStopId:        reader.headeridx.GetFldId("to_stop_id", -2),
		FromRouteId:     reader.headeridx.GetFldId("from_route_id", -3),
		ToRouteId:       reader.headeridx.GetFldId("to_route_id", -4),
		FromTripId:      reader.headeridx.GetFldId("from_trip_id", -5),
		ToTripId:        reader.headeridx.GetFldId("to_trip_id", -6),
		TransferType:    reader.headeridx.GetFldId("transfer_type", -7),
		MinTransferTime: reader.headeridx.GetFldId("min_transfer_time", -8),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}
	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		tk, tv, e := createTransfer(record, flds, feed, prefix)
		if e == nil {
			if _, ok := feed.Transfers[tk]; ok {
				e = errors.New("ID collision, transfer already defined")
			}
		}
		if e != nil {
			stopNotFoundErr, stopNotFound := e.(*StopNotFoundErr)
			wasFiltered := false
			if stopNotFound {
				_, wasFiltered = geofiltered[stopNotFoundErr.StopId()]
			}

			if wasFiltered {
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedTransfers++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		feed.Transfers[tk] = tv

		if !feed.Opts.DryRun {
			// add additional CSV fields
			for _, i := range addFlds {
				if i < len(record) {
					if _, ok := feed.TransfersAddFlds[reader.header[i]]; !ok {
						feed.TransfersAddFlds[reader.header[i]] = make(map[gtfs.TransferKey]string)
					}

					feed.TransfersAddFlds[reader.header[i]][tk] = record[i]
				}
			}
		}
	}

	feed.ColOrders.Transfers = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parsePathwaysReader(file io.Reader, prefix string, geofiltered map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"pathways.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := PathwayFields{
		pathwayId:            reader.headeridx.GetFldId("pathway_id", -1),
		fromStopId:           reader.headeridx.GetFldId("from_stop_id", -2),
		toStopId:             reader.headeridx.GetFldId("to_stop_id", -3),
		pathwayMode:          reader.headeridx.GetFldId("pathway_mode", -4),
		isBidirectional:      reader.headeridx.GetFldId("is_bidirectional", -5),
		length:               reader.headeridx.GetFldId("length", -6),
		traversalTime:        reader.headeridx.GetFldId("traversal_time", -7),
		stairCount:           reader.headeridx.GetFldId("stair_count", -8),
		maxSlope:             reader.headeridx.GetFldId("max_slope", -9),
		minWidth:             reader.headeridx.GetFldId("min_width", -10),
		signpostedAs:         reader.headeridx.GetFldId("signposted_as", -11),
		reversedSignpostedAs: reader.headeridx.GetFldId("reversed_signposted_as", -12),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		pw, e := createPathway(record, flds, feed, prefix)
		if e == nil {
			if _, ok := feed.Pathways[pw.Id]; ok {
				e = errors.New("ID collision, pathway_id '" + pw.Id + "' already used")
			}
		}
		if e != nil {
			stopNotFoundErr, stopNotFound := e.(*StopNotFoundErr)
			wasFiltered := false
			if stopNotFound {
				_, wasFiltered = geofiltered[stopNotFoundErr.StopId()]
			}

			if wasFiltered {
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedPathways++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}
		feed.Pathways[pw.Id] = pw

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.PathwaysAddFlds[reader.header[i]]; !ok {
					feed.PathwaysAddFlds[reader.header[i]] = make(map[string]string)
				}

				feed.PathwaysAddFlds[reader.header[i]][pw.Id] = record[i]
			}
		}
	}

	feed.ColOrders.Pathways = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseAttributionsReader(file io.Reader, prefix string, filteredRoutes map[string]struct{}, filteredTrips map[string]struct{}) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"attributions.txt", reader.Curline, r.(error).Error()}
		}
	}()

	ids := make(map[string]bool)

	var e error
	var record []string
	flds := AttributionFields{
		attributionId:    reader.headeridx.GetFldId("attribution_id", -1),
		organizationName: reader.headeridx.GetFldId("organization_name", -2),
		isProducer:       reader.headeridx.GetFldId("is_producer", -3),
		isOperator:       reader.headeridx.GetFldId("is_operator", -4),
		isAuthority:      reader.headeridx.GetFldId("is_authority", -5),
		attributionUrl:   reader.headeridx.GetFldId("attribution_url", -6),
		attributionEmail: reader.headeridx.GetFldId("attribution_email", -7),
		attributionPhone: reader.headeridx.GetFldId("attribution_phone", -8),
		routeId:          reader.headeridx.GetFldId("route_id", -9),
		agencyId:         reader.headeridx.GetFldId("agency_id", -10),
		tripId:           reader.headeridx.GetFldId("trip_id", -11),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		attr, ag, route, trip, e := createAttribution(record, flds, feed, prefix)
		if e == nil {
			if len(attr.Id) == len(prefix) {
				attr.Id = ""
			}
			if _, ok := ids[attr.Id]; ok {
				e = errors.New("ID collision, attribution_id '" + attr.Id + "' already used")
			}
			if len(attr.Id) > 0 {
				ids[attr.Id] = true
			}
		}

		if e != nil {
			routeNotFoundErr, routeNotFound := e.(*RouteNotFoundErr)
			wasFiltered := false
			if routeNotFound {
				_, wasFiltered = filteredRoutes[routeNotFoundErr.RouteId()]
			}

			tripNotFoundErr, tripNotFound := e.(*TripNotFoundErr)
			if tripNotFound {
				_, wasFiltered = filteredTrips[tripNotFoundErr.TripId()]
			}

			if wasFiltered {
				continue
			} else if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedAttributions++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}

		if ag != nil {
			ag.Attributions = append(ag.Attributions, attr)
		} else if route != nil {
			route.Attributions = append(route.Attributions, attr)
		} else if trip != nil {
			if trip.Attributions == nil {
				attrs := make([]*gtfs.Attribution, 0)
				trip.Attributions = &attrs
			}
			*trip.Attributions = append(*trip.Attributions, attr)
		} else {
			// if the attribution is not for a specific agency, route or trip,
			// add it to feed-wide
			feed.Attributions = append(feed.Attributions, attr)
		}

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.AttributionsAddFlds[reader.header[i]]; !ok {
					feed.AttributionsAddFlds[reader.header[i]] = make(map[*gtfs.Attribution]string)
				}

				feed.AttributionsAddFlds[reader.header[i]][attr] = record[i]
			}
		}
	}

	feed.ColOrders.Attributions = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseLevelsReader(file io.Reader, idprefix string) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"levels.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := LevelFields{
		levelId:    reader.headeridx.GetFldId("level_id", -1),
		levelIndex: reader.headeridx.GetFldId("level_index", -2),
		levelName:  reader.headeridx.GetFldId("level_name", -3),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}
	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		lvl, e := createLevel(record, flds, feed, idprefix)
		if e == nil {
			if _, ok := feed.Levels[lvl.Id]; ok {
				e = errors.New("ID collision, level_id '" + lvl.Id + "' already used")
			}
		}

		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedLevels++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}
		feed.Levels[lvl.Id] = lvl

		for _, i := range addFlds {
			if i < len(record) {
				if _, ok := feed.LevelsAddFlds[reader.header[i]]; !ok {
					feed.LevelsAddFlds[reader.header[i]] = make(map[string]string)
				}

				feed.LevelsAddFlds[reader.header[i]][lvl.Id] = record[i]
			}
		}
	}

	feed.ColOrders.Levels = append([]string(nil), reader.header...)

	return e
}

func (feed *Feed) parseFeedInfosReader(file io.Reader) (err error) {
	reader := NewCsvParser(file, feed.Opts.DropErroneous, false)

	defer func() {
		if r := recover(); r != nil {
			err = ParseError{"feed_info.txt", reader.Curline, r.(error).Error()}
		}
	}()

	var e error
	var record []string
	flds := FeedInfoFields{
		feedPublisherName: reader.headeridx.GetFldId("feed_publisher_name", -1),
		feedPublisherUrl:  reader.headeridx.GetFldId("feed_publisher_url", -2),
		feedLang:          reader.headeridx.GetFldId("feed_lang", -3),
		feedStartDate:     reader.headeridx.GetFldId("feed_start_date", -4),
		feedEndDate:       reader.headeridx.GetFldId("feed_end_date", -5),
		feedVersion:       reader.headeridx.GetFldId("feed_version", -6),
		feedContactEmail:  reader.headeridx.GetFldId("feed_contact_email", -7),
		feedContactUrl:    reader.headeridx.GetFldId("feed_contact_url", -8),
	}

	addFlds := make([]int, 0)

	if feed.Opts.KeepAddFlds {
		addFlds = addiFields(reader.header, flds)
	}

	for record = reader.ParseCsvLine(); record != nil; record = reader.ParseCsvLine() {
		fi, e := createFeedInfo(record, flds, feed)
		if e != nil {
			if feed.Opts.DropErroneous {
				feed.ErrorStats.DroppedFeedInfos++
				feed.warn(e)
				continue
			} else {
				panic(e)
			}
		}
		if !feed.Opts.DryRun {
			for _, i := range addFlds {
				if i < len(record) {
					if _, ok := feed.FeedInfosAddFlds[reader.header[i]]; !ok {
						feed.FeedInfosAddFlds[reader.header[i]] = make(map[*gtfs.FeedInfo]string)
					}

					feed.FeedInfosAddFlds[reader.header[i]][fi] = record[i]
				}
			}
			feed.FeedInfos = append(feed.FeedInfos, fi)
		}
	}

	feed.ColOrders.FeedInfos = append([]string(nil), reader.header...)

	return e
}

// withFile locates name inside the zip archive, opens it, invokes fn with
// its content reader, and closes it afterwards. If the file is missing and
// required is false, fn is silently skipped.
// If required is true and the file is missing, an error is returned.
func (feed *Feed) withFile(zr *zip.Reader, name string, required bool, fn func(io.Reader) error) error {
	f := findZipFile(zr, name)
	if f == nil {
		if required {
			return fmt.Errorf("required file %s not found in gtfs archive", name)
		}
		return nil
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening %s: %w", name, err)
	}
	defer rc.Close()

	return fn(rc)
}

// findZipFile looks up a GTFS file by name. Falls back to matching the
// basename in case the .txt files are nested inside a subfolder within
// the zip (some non-conformant feeds do this).
func findZipFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/"+name) {
			return f
		}
	}
	return nil
}

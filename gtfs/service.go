// Copyright 2023 Patrick Brosi
// Authors: info@patrickbrosi.de
//
// Use of this source code is governed by a GPL v2
// license that can be found in the LICENSE file

package gtfs

import (
	"time"
)

// A Service object describes exactly on what days a trip is served
type Service struct {
	id         string
	daymap     uint8
	start_date Date
	end_date   Date
	exceptions map[Date]bool
}

func EmptyService() *Service {
	return &Service{"", 0, Date{}, Date{}, make(map[Date]bool, 0)}
}

func (s *Service) Daymap(i int) bool {
	return (s.daymap & (1 << uint8(i))) > 0
}

func (s *Service) SetDaymap(i int, v bool) {
	if v {
		s.daymap |= (1 << uint8(i))
	} else {
		s.daymap &= ^(1 << uint8(i))
	}
}

func (s *Service) SetRawDaymap(i uint8) {
	s.daymap = i
}

func (s *Service) RawDaymap() uint8 {
	return s.daymap
}

func (s *Service) Id() string {
	return s.id
}

func (s *Service) SetId(id string) {
	s.id = id
}

func (s *Service) Start_date() Date {
	return s.start_date
}

func (s *Service) SetStart_date(d Date) {
	s.start_date = d
}

func (s *Service) End_date() Date {
	return s.end_date
}

func (s *Service) SetEnd_date(d Date) {
	s.end_date = d
}

func (s *Service) Exceptions() map[Date]bool {
	return s.exceptions
}

func (s *Service) SetExceptions(e map[Date]bool) {
	s.exceptions = e
}

// A Date object as used in GTFS
type Date struct {
	day   uint8
	month uint8
	year  uint8
}

func NewDate(day uint8, month uint8, year uint16) Date {
	return Date{day, month, uint8(year - 1900)}
}

func (d Date) IsEmpty() bool {
	return d.day == 0 && d.month == 0 && d.year == 0
}

func (d Date) Day() uint8 {
	return d.day
}

func (d *Date) SetDay(day uint8) {
	d.day = day
}

func (d Date) Month() uint8 {
	return d.month
}

func (d *Date) SetMonth(month uint8) {
	d.month = month
}

func (d Date) Year() uint16 {
	return uint16(d.year) + 1900
}

func (d *Date) SetYear(year uint16) {
	d.year = uint8(year - 1900)
}

// IsActiveOn returns true if the service is active on a particular date
func (s *Service) IsActiveOn(d Date) bool {
	exType := s.GetExceptionTypeOn(d)
	if exType == 1 {
		return true
	}
	if exType == 2 {
		return false
	}
	t := d.GetTime()
	return s.Daymap(int(t.Weekday())) && !(t.Before(s.Start_date().GetTime())) && !(t.After(s.End_date().GetTime()))
}

// GetExceptionTypeOn returns the expection type on a particular day
func (s *Service) GetExceptionTypeOn(d Date) int8 {
	if t, ok := s.Exceptions()[d]; ok {
		if t {
			return 1
		} else {
			return 2
		}
	}

	return 0
}

// SetExceptionTypeOn sets the expection type on a particular day
func (s *Service) SetExceptionTypeOn(d Date, t int8) {
	if t == 1 {
		s.Exceptions()[d] = true
	} else if t == 2 {
		s.Exceptions()[d] = false
	}
}

// GetGtfsDateFromTime constructs a GTFS Date object from a Time object
func GetGtfsDateFromTime(t time.Time) Date {
	return NewDate(uint8(t.Day()), uint8(t.Month()), uint16(t.Year()))
}

// GetOffsettedDate returns a date offsetted by a certain number of days
func (d Date) GetOffsettedDate(offset int) Date {
	if (offset == 1 || offset == -1) && d.Day() > 1 && d.Day() < 27 {
		// shortcut
		d.SetDay(uint8(int(d.Day()) + offset))
		return d
	}
	return GetGtfsDateFromTime((d.GetTime().AddDate(0, 0, offset)))
}

// Equals returns true if the service is exactly the same - that is
// if it is active on exactly the same days
func (s *Service) Equals(b *Service) bool {
	if s == b {
		// shortcut
		return true
	}

	// shortcut
	if !s.Start_date().IsEmpty() && !b.Start_date().IsEmpty() && len(s.Exceptions()) == 0 && len(b.Exceptions()) == 0 {
		return s.Start_date() == b.Start_date() && s.End_date() == b.End_date() && s.RawDaymap() == b.RawDaymap()
	}

	startA := s.GetFirstDefinedDate()
	endB := b.GetLastDefinedDate()

	if startA.GetTime().After(endB.GetTime()) {
		return false
	}

	startB := b.GetFirstDefinedDate()
	endA := s.GetLastDefinedDate()

	if startB.GetTime().After(endA.GetTime()) {
		return false
	}

	if endA.GetTime().Before(endB.GetTime()) {
		endA = endB
	}

	if startA.GetTime().After(startB.GetTime()) {
		startA = startB
	}

	for d := startA; !d.GetTime().After(endA.GetTime()); d = d.GetOffsettedDate(1) {
		if s.IsActiveOn(d) != b.IsActiveOn(d) {
			return false
		}
	}

	return true
}

// GetFirstDefinedDate returns the first date something is defined
// (either positively or negatively) in this service
func (s *Service) GetFirstDefinedDate() Date {
	var first Date

	for date := range s.Exceptions() {
		if first.IsEmpty() || date.GetTime().Before(first.GetTime()) {
			first = date
		}
	}

	if first.IsEmpty() || (!s.Start_date().IsEmpty() && s.Start_date().GetTime().Before(first.GetTime())) {
		first = s.Start_date()
	}

	return first
}

// GetLastDefinedDate returns the last date something is defined
// (either positively or negatively) in this service
func (s *Service) GetLastDefinedDate() Date {
	var last Date

	for date := range s.Exceptions() {
		if last.IsEmpty() || date.GetTime().After(last.GetTime()) {
			last = date
		}
	}

	if last.IsEmpty() || (!s.End_date().IsEmpty() && s.End_date().GetTime().After(last.GetTime())) {
		last = s.End_date()
	}

	return last
}

// GetFirstActiveDate returns the first active date of this service
func (s *Service) GetFirstActiveDate() Date {
	start := s.GetFirstDefinedDate()
	end := s.GetLastDefinedDate()
	for d := start; !d.GetTime().After(end.GetTime()); d = d.GetOffsettedDate(1) {
		if s.IsActiveOn(d) {
			return d
		}
	}

	return Date{}
}

// GetLastActiveDate returns the first active date of this service
func (s *Service) GetLastActiveDate() Date {
	start := s.GetFirstDefinedDate()
	end := s.GetLastDefinedDate()
	for d := end; !d.GetTime().Before(start.GetTime()); d = d.GetOffsettedDate(-1) {
		if s.IsActiveOn(d) {
			return d
		}
	}

	return Date{}
}

func (d *Service) IsEmpty() bool {
	return d.RawDaymap() == 0 && len(d.Exceptions()) == 0
}

// GetTime constructs a time object from this date, at 12:00:00 noon
func (d Date) GetTime() time.Time {
	return time.Date(int(d.Year()), time.Month(d.Month()), int(d.Day()), 12, 0, 0, 0, time.UTC)
}

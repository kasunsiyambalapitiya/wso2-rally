// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

// Package alerts records vehicle problems — a breakdown, a failing phone, or
// anything else — raised either by an organizer from the dashboard or by a
// crew from the in-car app.
//
// Raising an alert also moves the vehicle's status, so the dashboard, the live
// monitor, and the alert list always agree about which cars are in trouble.
package alerts

import (
	"fmt"
	"slices"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// Sentinel errors, wrapping the shared categories.
var (
	// ErrNotFound means no alert exists with the requested id.
	ErrNotFound = fmt.Errorf("%w: alert", apperr.ErrNotFound)
)

// Type is what went wrong.
type Type string

const (
	// TypeBreakdown is a mechanically stranded vehicle.
	TypeBreakdown Type = "breakdown"
	// TypeDeviceIssue is a failing in-car phone.
	TypeDeviceIssue Type = "device_issue"
	// TypeOther is anything else the crew wants to flag.
	TypeOther Type = "other"
)

var allTypes = []Type{TypeBreakdown, TypeDeviceIssue, TypeOther}

// IsValid reports whether t is a known alert type.
func (t Type) IsValid() bool { return slices.Contains(allTypes, t) }

// Source is who raised the alert.
type Source string

const (
	// SourceOrganizer is an alert raised from the organizer dashboard.
	SourceOrganizer Source = "organizer"
	// SourceCrew is an alert raised from the in-car app.
	SourceCrew Source = "crew"
)

var allSources = []Source{SourceOrganizer, SourceCrew}

// IsValid reports whether s is a known alert source.
func (s Source) IsValid() bool { return slices.Contains(allSources, s) }

// Alert is one reported vehicle problem.
type Alert struct {
	ID        string
	VehicleID string
	Type      Type
	Note      string
	Source    Source
	// RaisedBy is the organizer's email, or the crew's session id.
	RaisedBy string
	// Lat and Lng are where the vehicle was when the alert went out. They are
	// absent for an organizer-raised alert, which is filed from the pavilion.
	Lat        *float64
	Lng        *float64
	RaisedAt   time.Time
	ResolvedAt *time.Time
}

// IsOpen reports whether the alert still needs attention.
func (a Alert) IsOpen() bool { return a.ResolvedAt == nil }

// RaiseAlertInput is a request to report a vehicle problem.
type RaiseAlertInput struct {
	VehicleID string
	Type      Type
	Note      string
	Source    Source
	RaisedBy  string
	Lat       *float64
	Lng       *float64
}

// SearchFilter narrows an alert search. The zero value matches every alert of
// the event.
type SearchFilter struct {
	// OpenOnly restricts the result to unresolved alerts, which is what the
	// dashboard's warning card counts.
	OpenOnly bool
}

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

// Package sessions is the in-car runtime: one phone binds itself to a vehicle
// and its crew, streams position, unlocks and submits tasks, reports problems,
// and finishes at Pearl Bay.
//
// Binding is what authenticates a crew — there is no participant login — so
// the team token minted here is the only credential the micro app holds.
package sessions

import (
	"fmt"
	"slices"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// Sentinel errors, wrapping the shared categories.
var (
	// ErrNotFound means no session exists with the requested id.
	ErrNotFound = fmt.Errorf("%w: session", apperr.ErrNotFound)
	// ErrVehicleNotFound means the vehicle being bound does not exist.
	ErrVehicleNotFound = fmt.Errorf("%w: vehicle", apperr.ErrNotFound)
	// ErrAlreadyBound is the one-active-phone rule: this vehicle already has a
	// live session on another device.
	ErrAlreadyBound = fmt.Errorf("%w: this vehicle is already bound to another phone", apperr.ErrConflict)
	// ErrEventNotActive means the event has not been published, so crews
	// cannot bind to it yet.
	ErrEventNotActive = fmt.Errorf("%w: this event is not open for crews yet", apperr.ErrConflict)
	// ErrSessionFinished means the session is over and no longer accepts input.
	ErrSessionFinished = fmt.Errorf("%w: this session has already finished", apperr.ErrConflict)
	// ErrNoVoucher means the session has not finished, so nothing was issued.
	ErrNoVoucher = fmt.Errorf("%w: voucher", apperr.ErrNotFound)
)

// Status is where a session is in its lifecycle.
type Status string

const (
	// StatusBound is a phone paired to a vehicle, waiting at the start grid.
	StatusBound Status = "bound"
	// StatusActive is a crew under way.
	StatusActive Status = "active"
	// StatusFinished is a crew that reached Pearl Bay; its score is locked.
	StatusFinished Status = "finished"
)

var allStatuses = []Status{StatusBound, StatusActive, StatusFinished}

// IsValid reports whether s is a known session status.
func (s Status) IsValid() bool { return slices.Contains(allStatuses, s) }

// IsLive reports whether the session still accepts input.
func (s Status) IsLive() bool { return s == StatusBound || s == StatusActive }

// Session is one phone's run of the rally.
type Session struct {
	ID        string
	EventID   string
	VehicleID string
	Status    Status
	// CurrentWaypointID is the furthest boundary the vehicle has been inside.
	CurrentWaypointID *string
	TotalScore        int
	BoundAt           *time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	// Last position reported, kept for the organizer's live monitor.
	LastLat    *float64
	LastLng    *float64
	LastPingAt *time.Time
}

// BindTarget is what the repository knows about a vehicle at bind time.
type BindTarget struct {
	EventID      string
	RouteID      string
	Code         string
	TeamName     string
	CrewMemberID []string
}

// EventInfo is the slice of an event the in-car runtime needs.
type EventInfo struct {
	Status string
	// Cipher is withheld until the event is active and the start signal fires.
	Cipher    string
	StartTime string
	Finish    GeoCircle
	Start     GeoCircle
}

// IsActive reports whether crews may bind and run.
func (e EventInfo) IsActive() bool { return e.Status == "active" }

// BindInput is a request to pair a phone with a vehicle.
type BindInput struct {
	VehicleID string
	// CrewMemberIDs are the crew aboard, chosen from the vehicle's roster on
	// the initialization screen.
	CrewMemberIDs []string
}

// TaskState is one task as the crew sees it: the definition's identity plus
// whether this session has already completed it.
type TaskState struct {
	TaskID     string
	WaypointID string
	Code       string
	Title      string
	Type       string
	Points     int
	Status     string
	Awarded    int
}

// SessionState is everything the micro app needs to render its current screen.
type SessionState struct {
	Session     Session
	VehicleCode string
	TeamName    string
	EventStatus string
	StartTime   string
	// Cipher is empty until the event is active.
	Cipher       string
	StartCircle  GeoCircle
	FinishCircle GeoCircle
	Waypoints    []WaypointGeo
	// NextWaypointID is the first waypoint the crew has not reached.
	NextWaypointID string
}

// Voucher is what a crew collects at the finish.
type Voucher struct {
	ID          string
	SessionID   string
	EntryCode   string
	LockerID    string
	LunchPasses int
}

// CrewAlertInput is a problem reported from the in-car app.
type CrewAlertInput struct {
	Type string
	Note string
	Lat  *float64
	Lng  *float64
}

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

// Package routes owns the courses a rally is run over: named routes (Inland,
// Wetlands) and the ordered waypoints along each, with their geofence radii and
// the tasks attached to them.
//
// The waypoint order is the leg sequence crews drive, so reordering is a
// first-class operation rather than a field update.
package routes

import (
	"fmt"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// Sentinel errors, wrapping the shared categories.
var (
	// ErrRouteNotFound means no route exists with the requested id.
	ErrRouteNotFound = fmt.Errorf("%w: route", apperr.ErrNotFound)
	// ErrWaypointNotFound means no waypoint exists with the requested id.
	ErrWaypointNotFound = fmt.Errorf("%w: waypoint", apperr.ErrNotFound)
	// ErrDuplicateName means the event already has a route with that name.
	ErrDuplicateName = fmt.Errorf("%w: a route with that name already exists in this event", apperr.ErrConflict)
)

// Route is one course through an event.
type Route struct {
	ID      string
	EventID string
	Name    string
	// Order positions the route in organizer listings.
	Order int
	// Waypoints is populated by GetRoute, in driving order.
	Waypoints []Waypoint
}

// Waypoint is a single geofenced stop on a route.
type Waypoint struct {
	ID      string
	RouteID string
	// Order is the leg sequence, dense and zero-based after any reorder.
	Order           int
	Label           string
	Lat             float64
	Lng             float64
	BoundaryRadiusM int
	// TaskIDs are the tasks that unlock when a vehicle enters this waypoint.
	TaskIDs []string
}

// CreateRouteInput is a validated request to add a route to an event.
type CreateRouteInput struct {
	EventID string
	Name    string
	Order   int
}

// UpdateRouteInput is a PATCH: nil fields are left untouched.
type UpdateRouteInput struct {
	Name  *string
	Order *int
}

// AddWaypointInput is a validated request to append a waypoint to a route.
type AddWaypointInput struct {
	RouteID         string
	Label           string
	Lat             float64
	Lng             float64
	BoundaryRadiusM int
}

// UpdateWaypointInput is a PATCH: nil fields are left untouched.
type UpdateWaypointInput struct {
	Label           *string
	Lat             *float64
	Lng             *float64
	BoundaryRadiusM *int
}

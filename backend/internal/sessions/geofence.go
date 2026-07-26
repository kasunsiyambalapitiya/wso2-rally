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

package sessions

import (
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/geo"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

// EventType names something the backend noticed about a reported position.
// The micro app switches its screen on these.
type EventType string

const (
	// EventGeofenceEnter means the vehicle is inside a waypoint boundary and
	// that waypoint's tasks are now available.
	EventGeofenceEnter EventType = "geofence"
	// EventRestLock means the crew must stay put until the rest task clears.
	EventRestLock EventType = "rest_lock"
	// EventTrivia means a timed question just opened.
	EventTrivia EventType = "trivia"
	// EventArrival means the vehicle reached the finish geofence.
	EventArrival EventType = "arrival"
)

// LatLng is a reported position.
type LatLng struct {
	Lat float64
	Lng float64
}

// GeoCircle is a circular boundary.
type GeoCircle struct {
	Lat     float64
	Lng     float64
	RadiusM int
	// Placed is false when the organizer has not dropped this pin yet, in
	// which case the circle never matches.
	Placed bool
}

// Contains reports whether p lies inside the circle.
func (c GeoCircle) Contains(p LatLng) bool {
	if !c.Placed {
		return false
	}

	return geo.PointInRadius(p.Lat, p.Lng, c.Lat, c.Lng, float64(c.RadiusM))
}

// WaypointTask is one task attached to a waypoint, with the type the engine
// needs to decide whether entering the boundary starts a rest or a timer.
type WaypointTask struct {
	ID   string
	Type tasks.TaskType
}

// WaypointGeo is a waypoint reduced to what geofence evaluation needs.
type WaypointGeo struct {
	ID     string
	Order  int
	Circle GeoCircle
	Tasks  []WaypointTask
}

// PingEvent is one thing that happened at a reported position.
type PingEvent struct {
	Type EventType
	// WaypointID is the boundary that produced the event, empty for arrival.
	WaypointID string
	// TaskID is the task the event refers to, for rest locks and trivia.
	TaskID string
}

// PingResult is the backend's answer to a location report: what the crew may
// now do, and what their screen should switch to.
type PingResult struct {
	// UnlockedTaskIDs are the tasks available at the vehicle's position.
	UnlockedTaskIDs []string
	// CurrentWaypointID is the furthest-along boundary the vehicle is inside,
	// or empty when it is between waypoints.
	CurrentWaypointID string
	Events            []PingEvent
	// Arrived is true when the vehicle is inside the finish geofence.
	Arrived bool
}

// EvaluatePing decides what a reported position means. It is deliberately
// pure: all the geofence maths is here, testable without a database, and the
// service only has to persist what it returns.
//
// A vehicle can sit inside overlapping boundaries, so every match contributes
// its tasks; the current waypoint is the furthest along the route, which is
// the one the crew is working through.
func EvaluatePing(position LatLng, waypoints []WaypointGeo, finish GeoCircle) PingResult {
	result := PingResult{}
	highestOrder := -1

	for _, waypoint := range waypoints {
		if !waypoint.Circle.Contains(position) {
			continue
		}

		if waypoint.Order > highestOrder {
			highestOrder = waypoint.Order
			result.CurrentWaypointID = waypoint.ID
		}
		result.Events = append(result.Events, PingEvent{Type: EventGeofenceEnter, WaypointID: waypoint.ID})

		for _, task := range waypoint.Tasks {
			result.UnlockedTaskIDs = append(result.UnlockedTaskIDs, task.ID)

			switch task.Type {
			case tasks.TypeRestLock:
				result.Events = append(result.Events,
					PingEvent{Type: EventRestLock, WaypointID: waypoint.ID, TaskID: task.ID})
			case tasks.TypeTimedTrivia:
				result.Events = append(result.Events,
					PingEvent{Type: EventTrivia, WaypointID: waypoint.ID, TaskID: task.ID})
			}
		}
	}

	if finish.Contains(position) {
		result.Arrived = true
		result.Events = append(result.Events, PingEvent{Type: EventArrival})
	}

	return result
}

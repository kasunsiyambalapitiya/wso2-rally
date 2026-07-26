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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

// Colombo-area fixtures: the two waypoints are kilometres apart, so a vehicle
// at one is unambiguously outside the other.
var (
	atKandy    = LatLng{Lat: 6.8901, Lng: 79.9200}
	atMatale   = LatLng{Lat: 6.8480, Lng: 79.9280}
	nowhere    = LatLng{Lat: 6.0000, Lng: 80.5000}
	finishLine = GeoCircle{Lat: 6.8480, Lng: 79.9280, RadiusM: 30, Placed: true}
)

func waypointAt(id string, order int, at LatLng, taskList ...WaypointTask) WaypointGeo {
	return WaypointGeo{
		ID:     id,
		Order:  order,
		Circle: GeoCircle{Lat: at.Lat, Lng: at.Lng, RadiusM: 50, Placed: true},
		Tasks:  taskList,
	}
}

func eventTypes(result PingResult) []EventType {
	types := make([]EventType, 0, len(result.Events))
	for _, e := range result.Events {
		types = append(types, e.Type)
	}

	return types
}

func TestEvaluatePing_UnlocksTasksInsideWaypoint(t *testing.T) {
	waypoints := []WaypointGeo{
		waypointAt("wp-kandy", 0, atKandy,
			WaypointTask{ID: "task-1", Type: tasks.TypeInputSelect},
			WaypointTask{ID: "task-2", Type: tasks.TypeGridFill}),
		waypointAt("wp-matale", 1, atMatale, WaypointTask{ID: "task-3", Type: tasks.TypeInputNumber}),
	}

	got := EvaluatePing(atKandy, waypoints, GeoCircle{})

	require.Equal(t, []string{"task-1", "task-2"}, got.UnlockedTaskIDs)
	require.Equal(t, "wp-kandy", got.CurrentWaypointID)
	require.Contains(t, eventTypes(got), EventGeofenceEnter)
	require.False(t, got.Arrived)
}

func TestEvaluatePing_OutsideEveryBoundaryUnlocksNothing(t *testing.T) {
	waypoints := []WaypointGeo{
		waypointAt("wp-kandy", 0, atKandy, WaypointTask{ID: "task-1", Type: tasks.TypeInputSelect}),
	}

	got := EvaluatePing(nowhere, waypoints, finishLine)

	require.Empty(t, got.UnlockedTaskIDs)
	require.Empty(t, got.CurrentWaypointID)
	require.Empty(t, got.Events)
	require.False(t, got.Arrived)
}

func TestEvaluatePing_ArrivalWhenInsideFinishGeofence(t *testing.T) {
	got := EvaluatePing(atMatale, nil, finishLine)

	require.True(t, got.Arrived)
	require.Contains(t, eventTypes(got), EventArrival)
}

// An unplaced finish pin must never fire arrival, or every crew would be
// auto-finished at coordinates 0,0.
func TestEvaluatePing_UnplacedFinishNeverArrives(t *testing.T) {
	got := EvaluatePing(LatLng{}, nil, GeoCircle{RadiusM: 30})

	require.False(t, got.Arrived)
	require.Empty(t, got.Events)
}

func TestEvaluatePing_RestLockRaisesItsOwnEvent(t *testing.T) {
	waypoints := []WaypointGeo{
		waypointAt("wp-rest", 0, atKandy, WaypointTask{ID: "task-rest", Type: tasks.TypeRestLock}),
	}

	got := EvaluatePing(atKandy, waypoints, GeoCircle{})

	require.Contains(t, eventTypes(got), EventRestLock)
	for _, e := range got.Events {
		if e.Type == EventRestLock {
			require.Equal(t, "task-rest", e.TaskID)
			require.Equal(t, "wp-rest", e.WaypointID)
		}
	}
}

func TestEvaluatePing_TimedTriviaRaisesItsOwnEvent(t *testing.T) {
	waypoints := []WaypointGeo{
		waypointAt("wp-trivia", 0, atKandy, WaypointTask{ID: "task-trivia", Type: tasks.TypeTimedTrivia}),
	}

	got := EvaluatePing(atKandy, waypoints, GeoCircle{})

	require.Contains(t, eventTypes(got), EventTrivia)
}

// Overlapping boundaries all contribute their tasks, and the crew's current
// waypoint is the one furthest along the route.
func TestEvaluatePing_OverlappingWaypointsPickTheFurthestAlong(t *testing.T) {
	waypoints := []WaypointGeo{
		waypointAt("wp-early", 0, atKandy, WaypointTask{ID: "task-early", Type: tasks.TypeInputSelect}),
		waypointAt("wp-late", 5, atKandy, WaypointTask{ID: "task-late", Type: tasks.TypeInputNumber}),
	}

	got := EvaluatePing(atKandy, waypoints, GeoCircle{})

	require.ElementsMatch(t, []string{"task-early", "task-late"}, got.UnlockedTaskIDs)
	require.Equal(t, "wp-late", got.CurrentWaypointID)
}

func TestEvaluatePing_ArrivalCanCoincideWithAWaypoint(t *testing.T) {
	waypoints := []WaypointGeo{
		waypointAt("wp-final", 9, atMatale, WaypointTask{ID: "task-final", Type: tasks.TypeInputSelect}),
	}

	got := EvaluatePing(atMatale, waypoints, finishLine)

	require.True(t, got.Arrived)
	require.Equal(t, "wp-final", got.CurrentWaypointID)
	require.ElementsMatch(t, []EventType{EventGeofenceEnter, EventArrival}, eventTypes(got))
}

func TestGeoCircle_Contains(t *testing.T) {
	circle := GeoCircle{Lat: 6.8901, Lng: 79.9200, RadiusM: 50, Placed: true}

	require.True(t, circle.Contains(LatLng{Lat: 6.8902, Lng: 79.9201}))
	require.False(t, circle.Contains(nowhere))
	require.False(t, GeoCircle{Lat: 6.8901, Lng: 79.9200, RadiusM: 50}.Contains(atKandy),
		"an unplaced circle contains nothing")
}

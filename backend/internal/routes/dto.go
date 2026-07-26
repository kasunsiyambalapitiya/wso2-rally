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

package routes

// WaypointDTO is a waypoint on the wire.
type WaypointDTO struct {
	ID              string   `json:"id"`
	RouteID         string   `json:"routeId"`
	Order           int      `json:"order"`
	Label           string   `json:"label"`
	Lat             float64  `json:"lat"`
	Lng             float64  `json:"lng"`
	BoundaryRadiusM int      `json:"boundaryRadiusM"`
	TaskIDs         []string `json:"taskIds"`
}

// RouteDTO is a route on the wire. Waypoints is omitted from list responses,
// which return the route rows only.
type RouteDTO struct {
	ID        string        `json:"id"`
	EventID   string        `json:"eventId"`
	Name      string        `json:"name"`
	Order     int           `json:"order"`
	Waypoints []WaypointDTO `json:"waypoints,omitempty"`
}

// CreateRouteRequest is the POST /events/{eventId}/routes body.
type CreateRouteRequest struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
}

// UpdateRouteRequest is the PATCH /routes/{routeId} body.
type UpdateRouteRequest struct {
	Name  *string `json:"name"`
	Order *int    `json:"order"`
}

// AddWaypointRequest is the POST /routes/{routeId}/waypoints body.
type AddWaypointRequest struct {
	Label           string  `json:"label"`
	Lat             float64 `json:"lat"`
	Lng             float64 `json:"lng"`
	BoundaryRadiusM int     `json:"boundaryRadiusM"`
}

// UpdateWaypointRequest is the PATCH /waypoints/{waypointId} body.
type UpdateWaypointRequest struct {
	Label           *string  `json:"label"`
	Lat             *float64 `json:"lat"`
	Lng             *float64 `json:"lng"`
	BoundaryRadiusM *int     `json:"boundaryRadiusM"`
}

// ReorderWaypointsRequest is the PATCH /routes/{routeId}/waypoints/order body.
// The list must name every waypoint on the route.
type ReorderWaypointsRequest struct {
	OrderedIDs []string `json:"orderedIds"`
}

// AttachTasksRequest is the POST /waypoints/{waypointId}/tasks body. An empty
// list detaches every task.
type AttachTasksRequest struct {
	TaskIDs []string `json:"taskIds"`
}

func toWaypointDTO(w Waypoint) WaypointDTO {
	taskIDs := w.TaskIDs
	if taskIDs == nil {
		taskIDs = []string{}
	}

	return WaypointDTO{
		ID:              w.ID,
		RouteID:         w.RouteID,
		Order:           w.Order,
		Label:           w.Label,
		Lat:             w.Lat,
		Lng:             w.Lng,
		BoundaryRadiusM: w.BoundaryRadiusM,
		TaskIDs:         taskIDs,
	}
}

func toRouteDTO(r Route) RouteDTO {
	dto := RouteDTO{ID: r.ID, EventID: r.EventID, Name: r.Name, Order: r.Order}
	for _, w := range r.Waypoints {
		dto.Waypoints = append(dto.Waypoints, toWaypointDTO(w))
	}

	return dto
}

func toRouteDTOs(list []Route) []RouteDTO {
	out := make([]RouteDTO, 0, len(list))
	for _, r := range list {
		out = append(out, toRouteDTO(r))
	}

	return out
}

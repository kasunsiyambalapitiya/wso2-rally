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

package alerts

import "time"

// AlertDTO is an alert on the wire, and also the payload broadcast to the
// organizer's live monitor.
type AlertDTO struct {
	ID         string   `json:"id"`
	VehicleID  string   `json:"vehicleId"`
	Type       string   `json:"type"`
	Note       string   `json:"note"`
	Source     string   `json:"source"`
	RaisedBy   string   `json:"raisedBy"`
	Lat        *float64 `json:"lat"`
	Lng        *float64 `json:"lng"`
	RaisedAt   string   `json:"raisedAt"`
	ResolvedAt *string  `json:"resolvedAt"`
}

// RaiseAlertRequest is the POST /vehicles/{vehicleId}/alerts body.
type RaiseAlertRequest struct {
	Type string   `json:"type"`
	Note string   `json:"note"`
	Lat  *float64 `json:"lat"`
	Lng  *float64 `json:"lng"`
}

// SearchAlertsRequest is the POST /events/{eventId}/alerts/search body.
type SearchAlertsRequest struct {
	Offset  int `json:"offset"`
	Limit   int `json:"limit"`
	Filters struct {
		OpenOnly bool `json:"openOnly"`
	} `json:"filters"`
}

// ToDTO converts an alert to its wire shape. It is exported because the
// sessions domain raises crew alerts and the realtime hub broadcasts them.
func ToDTO(a Alert) AlertDTO {
	dto := AlertDTO{
		ID:        a.ID,
		VehicleID: a.VehicleID,
		Type:      string(a.Type),
		Note:      a.Note,
		Source:    string(a.Source),
		RaisedBy:  a.RaisedBy,
		Lat:       a.Lat,
		Lng:       a.Lng,
		RaisedAt:  a.RaisedAt.UTC().Format(time.RFC3339),
	}
	if a.ResolvedAt != nil {
		resolvedAt := a.ResolvedAt.UTC().Format(time.RFC3339)
		dto.ResolvedAt = &resolvedAt
	}

	return dto
}

func toDTOs(list []Alert) []AlertDTO {
	out := make([]AlertDTO, 0, len(list))
	for _, a := range list {
		out = append(out, ToDTO(a))
	}

	return out
}

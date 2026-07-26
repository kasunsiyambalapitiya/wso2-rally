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

// Package geo provides the spherical-distance maths behind every geofence
// decision. All geofence evaluation happens server-side: the micro app reports
// coordinates and this package decides whether the vehicle is inside a
// boundary.
package geo

import "math"

// earthRadiusM is the mean Earth radius used by the haversine formula. Rally
// boundaries are tens of metres wide, so the spherical approximation is far
// more precise than consumer GPS.
const earthRadiusM = 6_371_000.0

// HaversineMeters returns the great-circle distance in metres between two
// WGS-84 coordinates.
func HaversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	phi1, phi2 := radians(lat1), radians(lat2)
	deltaPhi := radians(lat2 - lat1)
	deltaLambda := radians(lng2 - lng1)

	sinLat := math.Sin(deltaPhi / 2)
	sinLng := math.Sin(deltaLambda / 2)
	a := sinLat*sinLat + math.Cos(phi1)*math.Cos(phi2)*sinLng*sinLng

	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// PointInRadius reports whether (lat, lng) lies within radiusM metres of the
// given centre. The boundary itself counts as inside; a negative radius never
// matches, which keeps a misconfigured waypoint from unlocking everything.
func PointInRadius(lat, lng, centerLat, centerLng, radiusM float64) bool {
	if radiusM < 0 {
		return false
	}
	return HaversineMeters(lat, lng, centerLat, centerLng) <= radiusM
}

func radians(deg float64) float64 {
	return deg * math.Pi / 180
}

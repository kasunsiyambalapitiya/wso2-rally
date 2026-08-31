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

package geo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHaversineMeters_KnownDistance(t *testing.T) {
	// Two points north/south of each other in the Colombo area, ~4.7 km apart.
	d := HaversineMeters(6.8901, 79.9200, 6.8480, 79.9280)

	require.InDelta(t, 4700, d, 400)
}

func TestHaversineMeters_IsSymmetric(t *testing.T) {
	ab := HaversineMeters(6.8901, 79.9200, 6.8480, 79.9280)
	ba := HaversineMeters(6.8480, 79.9280, 6.8901, 79.9200)

	require.InDelta(t, ab, ba, 1e-6)
}

func TestHaversineMeters_SamePointIsZero(t *testing.T) {
	require.Zero(t, HaversineMeters(6.8901, 79.9200, 6.8901, 79.9200))
}

func TestHaversineMeters_AntipodalIsHalfCircumference(t *testing.T) {
	// Half the Earth's great circle, ~20 015 km.
	require.InDelta(t, 20_015_000, HaversineMeters(0, 0, 0, 180), 5_000)
}

func TestPointInRadius(t *testing.T) {
	tests := []struct {
		name                 string
		lat, lng             float64
		centerLat, centerLng float64
		radiusM              float64
		want                 bool
	}{
		{"inside a 50 m boundary", 6.8901, 79.9200, 6.8902, 79.9201, 50, true},
		{"kilometres away", 6.8901, 79.9200, 6.8480, 79.9280, 50, false},
		{"exactly at the centre", 6.8901, 79.9200, 6.8901, 79.9200, 0, true},
		{"negative radius never matches", 6.8901, 79.9200, 6.8901, 79.9200, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, PointInRadius(tt.lat, tt.lng, tt.centerLat, tt.centerLng, tt.radiusM))
		})
	}
}

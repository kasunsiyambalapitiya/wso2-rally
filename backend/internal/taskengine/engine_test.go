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

package taskengine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

const maxPoints = 50

// Every task type the library accepts must be scoreable, or an organizer could
// author a task that breaks mid-rally.
func TestRegistry_CoversEveryTaskType(t *testing.T) {
	for _, taskType := range tasks.AllTypes() {
		t.Run(string(taskType), func(t *testing.T) {
			require.Contains(t, SupportedTypes(), taskType)
		})
	}
}

func TestValidate_UnknownTypeIsRejected(t *testing.T) {
	_, err := Validate("NOPE", json.RawMessage(`{}`), json.RawMessage(`{}`), maxPoints)

	require.ErrorIs(t, err, apperr.ErrValidation)
}

func TestValidate_ScoresEveryType(t *testing.T) {
	tests := []struct {
		name        string
		taskType    tasks.TaskType
		config      string
		payload     string
		wantCorrect bool
		wantPoints  int
	}{
		{
			"input select correct", tasks.TypeInputSelect,
			`{"answer":"API Integration"}`, `{"answer":"API Integration"}`, true, maxPoints,
		},
		{
			"input select is case-insensitive", tasks.TypeInputSelect,
			`{"answer":"API Integration"}`, `{"answer":"  api integration "}`, true, maxPoints,
		},
		{
			"input select wrong", tasks.TypeInputSelect,
			`{"answer":"API Integration"}`, `{"answer":"Service Mesh"}`, false, 0,
		},
		{
			"input number exact", tasks.TypeInputNumber,
			`{"answer":42}`, `{"answer":42}`, true, maxPoints,
		},
		{
			"input number within tolerance", tasks.TypeInputNumber,
			`{"answer":120.5,"tolerance":1}`, `{"answer":121}`, true, maxPoints,
		},
		{
			"input number outside tolerance", tasks.TypeInputNumber,
			`{"answer":120.5,"tolerance":0.1}`, `{"answer":125}`, false, 0,
		},
		{
			"multi select ignores order", tasks.TypeMultiSelect,
			`{"answers":["Sri Lanka","India"]}`, `{"answers":["india","Sri Lanka"]}`, true, maxPoints,
		},
		{
			"multi select rejects an extra choice", tasks.TypeMultiSelect,
			`{"answers":["Sri Lanka"]}`, `{"answers":["Sri Lanka","India"]}`, false, 0,
		},
		{
			"barcode match", tasks.TypeScanBarcode,
			`{"payload":"WSO2-CP-4"}`, `{"payload":"wso2-cp-4"}`, true, maxPoints,
		},
		{
			"barcode mismatch", tasks.TypeScanBarcode,
			`{"payload":"WSO2-CP-4"}`, `{"payload":"WSO2-CP-9"}`, false, 0,
		},
		{
			"telematics clean run", tasks.TypeTelematics,
			`{}`, `{"hardStops":0,"sharpTurns":0,"km":42.5}`, true, maxPoints,
		},
		{
			// 2 hard stops + 1 sharp turn = 15% penalty of 50 → 43 (rounded).
			"telematics with harsh events", tasks.TypeTelematics,
			`{}`, `{"hardStops":2,"sharpTurns":1,"km":42.5}`, true, 43,
		},
		{
			"telematics floors at zero", tasks.TypeTelematics,
			`{}`, `{"hardStops":50,"sharpTurns":50,"km":42.5}`, false, 0,
		},
		{
			"geofence cross reached", tasks.TypeGeofenceCross,
			`{}`, `{"reached":true}`, true, maxPoints,
		},
		{
			"geofence cross not reached", tasks.TypeGeofenceCross,
			`{}`, `{"reached":false}`, false, 0,
		},
		{
			"proximity checkpoint hit", tasks.TypeProximity,
			`{}`, `{"reached":true}`, true, maxPoints,
		},
		{
			"rest lock awards nothing but completes", tasks.TypeRestLock,
			`{}`, `{"reached":true}`, true, 0,
		},
		{
			"rest lock incomplete", tasks.TypeRestLock,
			`{}`, `{"reached":false}`, false, 0,
		},
		{
			"grid fill complete", tasks.TypeGridFill,
			`{"solution":["A","B","C","D"]}`, `{"answer":["A","B","C","D"]}`, true, maxPoints,
		},
		{
			"grid fill half right earns half", tasks.TypeGridFill,
			`{"solution":["A","B","C","D"]}`, `{"answer":["A","B","X","Y"]}`, false, 25,
		},
		{
			"grid fill short answer", tasks.TypeGridFill,
			`{"solution":["A","B","C","D"]}`, `{"answer":["A"]}`, false, 13,
		},
		{
			"gate match in order", tasks.TypeGateMatch,
			`{"solution":["HTTP","gRPC","MQTT"]}`, `{"answer":["HTTP","gRPC","MQTT"]}`, true, maxPoints,
		},
		{
			"gate match out of order earns nothing", tasks.TypeGateMatch,
			`{"solution":["HTTP","gRPC","MQTT"]}`, `{"answer":["gRPC","HTTP","MQTT"]}`, false, 0,
		},
		{
			"blind timer exact", tasks.TypeBlindTimer,
			`{"targetSec":45}`, `{"elapsedSec":45}`, true, maxPoints,
		},
		{
			// 9s out of 45 is 20% off → 80% of 50 = 40.
			"blind timer close", tasks.TypeBlindTimer,
			`{"targetSec":45}`, `{"elapsedSec":54}`, true, 40,
		},
		{
			"blind timer double the target scores nothing", tasks.TypeBlindTimer,
			`{"targetSec":45}`, `{"elapsedSec":90}`, false, 0,
		},
		{
			"branch solve pays", tasks.TypeBranch,
			`{"solvePoints":40,"skipPoints":-40}`, `{"branch":"solve"}`, true, 40,
		},
		{
			"branch skip costs", tasks.TypeBranch,
			`{"solvePoints":40,"skipPoints":-40}`, `{"branch":"skip"}`, true, -40,
		},
		{
			"timed trivia in time", tasks.TypeTimedTrivia,
			`{"answer":"Ballerina","limitSec":30}`, `{"answer":"ballerina","elapsedSec":12}`, true, maxPoints,
		},
		{
			"timed trivia too slow", tasks.TypeTimedTrivia,
			`{"answer":"Ballerina","limitSec":30}`, `{"answer":"Ballerina","elapsedSec":31}`, false, 0,
		},
		{
			"timed trivia wrong but in time", tasks.TypeTimedTrivia,
			`{"answer":"Ballerina","limitSec":30}`, `{"answer":"Choreo","elapsedSec":5}`, false, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Validate(tt.taskType, json.RawMessage(tt.config), json.RawMessage(tt.payload), maxPoints)

			require.NoError(t, err)
			require.Equal(t, tt.wantCorrect, got.Correct)
			require.Equal(t, tt.wantPoints, got.AwardedPoints)
			require.NotEmpty(t, got.Detail)
		})
	}
}

func TestValidate_RejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name     string
		taskType tasks.TaskType
		config   string
		payload  string
	}{
		{"missing payload", tasks.TypeInputSelect, `{"answer":"a"}`, ``},
		{"unreadable payload", tasks.TypeInputSelect, `{"answer":"a"}`, `{oops`},
		{"non-numeric answer for a number task", tasks.TypeInputNumber, `{"answer":42}`, `{"answer":"forty-two"}`},
		{"non-numeric configured answer", tasks.TypeInputNumber, `{"answer":"x"}`, `{"answer":42}`},
		{"grid with no solution", tasks.TypeGridFill, `{}`, `{"answer":["A"]}`},
		{"gate with no solution", tasks.TypeGateMatch, `{}`, `{"answer":["A"]}`},
		{"blind timer with no target", tasks.TypeBlindTimer, `{}`, `{"elapsedSec":10}`},
		{"blind timer with negative elapsed", tasks.TypeBlindTimer, `{"targetSec":45}`, `{"elapsedSec":-1}`},
		{"trivia with no limit", tasks.TypeTimedTrivia, `{"answer":"a"}`, `{"answer":"a","elapsedSec":1}`},
		{"branch with an unknown choice", tasks.TypeBranch, `{"solvePoints":1,"skipPoints":-1}`, `{"branch":"maybe"}`},
		{"negative driving counts", tasks.TypeTelematics, `{}`, `{"hardStops":-1,"sharpTurns":0}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(tt.taskType, json.RawMessage(tt.config), json.RawMessage(tt.payload), maxPoints)

			require.ErrorIs(t, err, apperr.ErrValidation)
		})
	}
}

// A crew cannot earn more than the task is worth, whatever it reports.
func TestValidate_NeverExceedsTheTaskPoints(t *testing.T) {
	overreported := []struct {
		taskType tasks.TaskType
		config   string
		payload  string
	}{
		{tasks.TypeTelematics, `{}`, `{"hardStops":0,"sharpTurns":0,"km":9999}`},
		{tasks.TypeBlindTimer, `{"targetSec":45}`, `{"elapsedSec":45}`},
		{tasks.TypeGridFill, `{"solution":["A"]}`, `{"answer":["A","A","A","A"]}`},
	}

	for _, tt := range overreported {
		t.Run(string(tt.taskType), func(t *testing.T) {
			got, err := Validate(tt.taskType, json.RawMessage(tt.config), json.RawMessage(tt.payload), maxPoints)

			require.NoError(t, err)
			require.LessOrEqual(t, got.AwardedPoints, maxPoints)
		})
	}
}

func TestScale(t *testing.T) {
	require.Equal(t, 0, scale(50, -1), "a negative factor floors at zero")
	require.Equal(t, 50, scale(50, 2), "a factor above one caps at the maximum")
	require.Equal(t, 25, scale(50, 0.5))
	require.Equal(t, 43, scale(50, 0.85), "points round to the nearest whole")
}

func TestEqualJSON(t *testing.T) {
	require.True(t, equalJSON(float64(45), 45))
	require.True(t, equalJSON("a", "a"))
	require.False(t, equalJSON("a", "b"))
	require.False(t, equalJSON(45, 46))
}

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

// Package tasks owns the definitions of the fifteen rally challenges.
//
// A task is data, not code: its TaskType selects a validator in the taskengine
// and a screen body in the micro app, and its Config carries the per-type
// parameters (the cipher answer, the grid solution, the target seconds…).
// Retuning or adding a challenge is therefore an authoring change, not a
// deployment.
package tasks

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// Sentinel errors, wrapping the shared categories.
var (
	// ErrNotFound means no task exists with the requested id.
	ErrNotFound = fmt.Errorf("%w: task", apperr.ErrNotFound)
	// ErrDuplicateCode means the event already has a task with that code.
	ErrDuplicateCode = fmt.Errorf("%w: a task with that code already exists in this event", apperr.ErrConflict)
)

// TaskType selects the validator and scorer in the taskengine, and the body
// the micro app renders inside its shared task shell.
type TaskType string

// The thirteen task types behind the fifteen rally challenges. Three of the
// challenges (signpost arithmetic, milestone digit scan, odometer calibration)
// share INPUT_NUMBER.
const (
	// TypeInputSelect is a single choice from a list, scored on exact match.
	TypeInputSelect TaskType = "INPUT_SELECT"
	// TypeInputNumber is a numeric answer, optionally within a tolerance.
	TypeInputNumber TaskType = "INPUT_NUMBER"
	// TypeMultiSelect is a set of choices, scored on set equality.
	TypeMultiSelect TaskType = "MULTI_SELECT"
	// TypeScanBarcode matches a scanned barcode payload.
	TypeScanBarcode TaskType = "SCAN_BARCODE"
	// TypeTelematics scores driving smoothness from accelerometer data.
	TypeTelematics TaskType = "TELEMATICS"
	// TypeGeofenceCross awards points for crossing a tight radius.
	TypeGeofenceCross TaskType = "GEOFENCE_CROSS"
	// TypeProximity is the BLE-beacon stand-in: a QR or geofence checkpoint.
	TypeProximity TaskType = "PROXIMITY"
	// TypeGridFill is a crossword-style grid, scored per correct cell.
	TypeGridFill TaskType = "GRID_FILL"
	// TypeBlindTimer scores how close an untimed guess came to the target.
	TypeBlindTimer TaskType = "BLIND_TIMER"
	// TypeBranch awards or deducts points for the route branch chosen.
	TypeBranch TaskType = "BRANCH"
	// TypeRestLock enforces the mandatory rest stop and awards nothing.
	TypeRestLock TaskType = "REST_LOCK"
	// TypeTimedTrivia requires an answer within a time limit.
	TypeTimedTrivia TaskType = "TIMED_TRIVIA"
	// TypeGateMatch checks a sequence of connectors is in the right order.
	TypeGateMatch TaskType = "GATE_MATCH"
)

// allTypes is the canonical registry. AllTypes returns a copy of it.
var allTypes = []TaskType{
	TypeInputSelect, TypeInputNumber, TypeMultiSelect, TypeScanBarcode,
	TypeTelematics, TypeGeofenceCross, TypeProximity, TypeGridFill,
	TypeBlindTimer, TypeBranch, TypeRestLock, TypeTimedTrivia, TypeGateMatch,
}

// AllTypes returns every known task type, for enum exposure and validation.
func AllTypes() []TaskType { return slices.Clone(allTypes) }

// IsValid reports whether t is a known task type.
func (t TaskType) IsValid() bool { return slices.Contains(allTypes, t) }

// Trigger is what makes a task available to a crew.
type Trigger string

const (
	// TriggerGeofence unlocks the task on entering a waypoint boundary.
	TriggerGeofence Trigger = "geofence"
	// TriggerSensor unlocks the task from device sensor data.
	TriggerSensor Trigger = "sensor"
	// TriggerChoice presents the task as a decision point.
	TriggerChoice Trigger = "choice"
	// TriggerManual is started by the crew from the task list.
	TriggerManual Trigger = "manual"
	// TriggerTimed opens on the clock.
	TriggerTimed Trigger = "timed"
)

var allTriggers = []Trigger{TriggerGeofence, TriggerSensor, TriggerChoice, TriggerManual, TriggerTimed}

// IsValid reports whether t is a known trigger.
func (t Trigger) IsValid() bool { return slices.Contains(allTriggers, t) }

// Sensor is the device capability a task needs. It tells the micro app which
// permission to request before opening the task.
type Sensor string

const (
	// SensorNone needs no device capability.
	SensorNone Sensor = "none"
	// SensorGeolocation needs the GPS position stream.
	SensorGeolocation Sensor = "geolocation"
	// SensorDeviceMotion needs the accelerometer.
	SensorDeviceMotion Sensor = "devicemotion"
	// SensorCamera needs the camera, for barcode scanning.
	SensorCamera Sensor = "camera"
	// SensorQR needs the camera in QR mode, used by the proximity checkpoint.
	SensorQR Sensor = "qr"
)

var allSensors = []Sensor{SensorNone, SensorGeolocation, SensorDeviceMotion, SensorCamera, SensorQR}

// IsValid reports whether s is a known sensor.
func (s Sensor) IsValid() bool { return slices.Contains(allSensors, s) }

// Task is one authored challenge.
type Task struct {
	ID      string
	EventID string
	// Code is the organizer-facing label, T1 through T15.
	Code    string
	Title   string
	Type    TaskType
	Trigger Trigger
	// Points is the maximum award. BRANCH tasks may set it negative.
	Points int
	Sensor Sensor
	// Config holds the per-type parameters as a JSON object. The taskengine
	// interprets it; this package only guarantees it is a valid object.
	Config json.RawMessage
}

// CreateTaskInput is a request to author a task.
type CreateTaskInput struct {
	EventID string
	Code    string
	Title   string
	Type    TaskType
	Trigger Trigger
	Points  int
	Sensor  Sensor
	Config  json.RawMessage
}

// UpdateTaskInput is a PATCH: nil fields are left untouched. Config is a slice
// rather than a pointer — a nil slice means "unchanged".
type UpdateTaskInput struct {
	Code    *string
	Title   *string
	Type    *TaskType
	Trigger *Trigger
	Points  *int
	Sensor  *Sensor
	Config  json.RawMessage
}

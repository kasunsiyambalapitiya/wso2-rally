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

package tasks

import (
	"encoding/json"
	"slices"
)

// secretConfigKeys are the config entries that answer the task. Crews render
// tasks from the same definition organizers author, so these are removed
// before the definition leaves for an in-car phone.
//
// This is a deny-list on purpose: a task type's presentation keys (prompt,
// options, grid shape) are open-ended and safe, while the handful of scoring
// keys below are not. Every new key that decides a score must be added here.
var secretConfigKeys = []string{
	"answer",       // INPUT_SELECT, INPUT_NUMBER, TIMED_TRIVIA
	"answers",      // MULTI_SELECT
	"solution",     // GRID_FILL, GATE_MATCH
	"payload",      // SCAN_BARCODE
	"targetSec",    // BLIND_TIMER — the whole point is to guess it
	"tolerance",    // INPUT_NUMBER — reveals how close the answer must be
	"solvePoints",  // BRANCH
	"skipPoints",   // BRANCH
	"checkpointId", // PROXIMITY
}

// RedactForCrew strips the scoring secrets from a task config so it can be
// sent to an in-car phone.
//
// Config that is not a JSON object is replaced with an empty one rather than
// passed through: an unreadable config cannot be checked, so it is not shown.
func RedactForCrew(config json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(config, &fields); err != nil {
		return emptyConfig
	}

	for key := range fields {
		if slices.Contains(secretConfigKeys, key) {
			delete(fields, key)
		}
	}

	redacted, err := json.Marshal(fields)
	if err != nil {
		return emptyConfig
	}

	return redacted
}

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

// Package taskengine validates and scores task submissions.
//
// There is one validator per TaskType, looked up in a registry, so the fifteen
// rally challenges need thirteen small functions rather than fifteen branches
// scattered through the session code. Scoring happens here and nowhere else:
// the in-car app never decides whether it was right.
package taskengine

import (
	"encoding/json"
	"math"
	"reflect"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

// Result is the outcome of one submission.
type Result struct {
	// Correct is whether the crew satisfied the task at all. A partially
	// scored task (a half-filled grid) is not correct but still earns points.
	Correct bool
	// AwardedPoints is what the submission adds to the team's score. It may be
	// negative for a BRANCH shortcut.
	AwardedPoints int
	// Detail is a short, crew-safe explanation shown under the result.
	Detail string
}

// Validator checks one task type's payload against its config and scores it.
type Validator func(config, payload json.RawMessage, maxPoints int) (Result, error)

// registry maps each task type to its validator. A type missing from here
// cannot be scored, which is why tasks.Service refuses to store one.
var registry = map[tasks.TaskType]Validator{
	tasks.TypeInputSelect:   validateExactAnswer,
	tasks.TypeInputNumber:   validateNumericAnswer,
	tasks.TypeMultiSelect:   validateMultiSelect,
	tasks.TypeScanBarcode:   validateBarcode,
	tasks.TypeTelematics:    validateTelematics,
	tasks.TypeGeofenceCross: validateReached,
	tasks.TypeProximity:     validateReached,
	tasks.TypeRestLock:      validateRestLock,
	tasks.TypeGridFill:      validateGridFill,
	tasks.TypeGateMatch:     validateGateMatch,
	tasks.TypeBlindTimer:    validateBlindTimer,
	tasks.TypeBranch:        validateBranch,
	tasks.TypeTimedTrivia:   validateTimedTrivia,
}

// SupportedTypes reports the task types the engine can score. It exists so a
// test can assert the registry covers every type the task library accepts.
func SupportedTypes() []tasks.TaskType {
	out := make([]tasks.TaskType, 0, len(registry))
	for taskType := range registry {
		out = append(out, taskType)
	}

	return out
}

// Validate scores a submission against its task definition.
//
// maxPoints is the task's authored points value; each type decides how much of
// it the submission earned.
func Validate(taskType tasks.TaskType, config, payload json.RawMessage, maxPoints int) (Result, error) {
	validator, ok := registry[taskType]
	if !ok {
		return Result{}, apperr.Validationf("task type %q cannot be scored", taskType)
	}

	return validator(config, payload, maxPoints)
}

// decode unmarshals a config or payload, turning a parse failure into a
// client-safe validation error rather than a 500.
func decode(raw json.RawMessage, dst any, what string) error {
	if len(raw) == 0 {
		return apperr.Validationf("the task %s is missing", what)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return apperr.Validationf("the task %s could not be read", what)
	}

	return nil
}

// scale converts a 0..1 quality factor into points, rounding to the nearest
// whole point and never returning less than zero.
func scale(maxPoints int, factor float64) int {
	if factor <= 0 {
		return 0
	}
	if factor > 1 {
		factor = 1
	}

	return int(math.Round(float64(maxPoints) * factor))
}

// awarded builds the common "all or nothing" result.
func awarded(correct bool, maxPoints int, correctDetail, wrongDetail string) Result {
	if !correct {
		return Result{Correct: false, AwardedPoints: 0, Detail: wrongDetail}
	}

	return Result{Correct: true, AwardedPoints: maxPoints, Detail: correctDetail}
}

// equalJSON compares two decoded JSON values. Numbers are compared numerically
// so 45 and 45.0 match, and strings are compared exactly.
func equalJSON(a, b any) bool {
	aNum, aIsNum := toFloat(a)
	bNum, bIsNum := toFloat(b)
	if aIsNum && bIsNum {
		return aNum == bNum
	}

	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)
	if aIsStr && bIsStr {
		return aStr == bStr
	}

	// Anything else — booleans, arrays, nested objects — is compared
	// structurally rather than by its formatted text.
	return reflect.DeepEqual(a, b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case json.Number:
		parsed, err := n.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

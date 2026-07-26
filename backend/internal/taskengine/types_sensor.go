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
	"math"
	"strings"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// Telematics penalties. Each harsh event costs 5% of the efficiency index, so
// twenty of them zero the score.
const (
	hardStopPenalty  = 0.05
	sharpTurnPenalty = 0.05
)

// Branch choices a crew can make at a decision point.
const (
	branchSolve = "solve"
	branchSkip  = "skip"
)

type telematicsPayload struct {
	HardStops  int     `json:"hardStops"`
	SharpTurns int     `json:"sharpTurns"`
	Km         float64 `json:"km"`
}

// validateTelematics scores TELEMATICS on an efficiency index: a clean run
// keeps the full award, and each harsh stop or sharp turn shaves it down.
//
// The client reports counts derived from DeviceMotion; the server decides what
// they are worth, so a tampered count still cannot exceed the task's points.
func validateTelematics(_, payload json.RawMessage, maxPoints int) (Result, error) {
	var given telematicsPayload
	if err := decode(payload, &given, "driving data"); err != nil {
		return Result{}, err
	}
	if given.HardStops < 0 || given.SharpTurns < 0 {
		return Result{}, apperr.Validationf("driving event counts must not be negative")
	}

	index := 1 - hardStopPenalty*float64(given.HardStops) - sharpTurnPenalty*float64(given.SharpTurns)
	points := scale(maxPoints, index)

	return Result{
		Correct:       points > 0,
		AwardedPoints: points,
		Detail:        "Eco-driving index scored.",
	}, nil
}

type reachedPayload struct {
	Reached bool `json:"reached"`
}

// validateReached scores the checkpoint types — GEOFENCE_CROSS and PROXIMITY —
// which are simply hit or not.
//
// The flag is only ever set after the backend's own geofence evaluation
// unlocked the task, so a client cannot claim a checkpoint it never reached.
func validateReached(_, payload json.RawMessage, maxPoints int) (Result, error) {
	var given reachedPayload
	if err := decode(payload, &given, "checkpoint report"); err != nil {
		return Result{}, err
	}

	return awarded(given.Reached, maxPoints, "Checkpoint reached.", "Checkpoint not reached yet."), nil
}

// validateRestLock scores REST_LOCK, which awards nothing: the mandatory rest
// is a compliance stop, not a challenge. Completing it still marks the task
// done so the crew can move on.
func validateRestLock(_, payload json.RawMessage, _ int) (Result, error) {
	var given reachedPayload
	if err := decode(payload, &given, "rest report"); err != nil {
		return Result{}, err
	}

	if !given.Reached {
		return Result{Correct: false, AwardedPoints: 0, Detail: "The rest stop is not complete."}, nil
	}

	return Result{Correct: true, AwardedPoints: 0, Detail: "Rest complete. Drive safely."}, nil
}

type blindTimerConfig struct {
	TargetSec float64 `json:"targetSec"`
}

type blindTimerPayload struct {
	ElapsedSec float64 `json:"elapsedSec"`
}

// validateBlindTimer scores BLIND_TIMER by how close the crew's untimed guess
// came to the target: exact earns everything, twice the target earns nothing.
func validateBlindTimer(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg blindTimerConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given blindTimerPayload
	if err := decode(payload, &given, "timing"); err != nil {
		return Result{}, err
	}
	if cfg.TargetSec <= 0 {
		return Result{}, apperr.Validationf("this task has no target time configured")
	}
	if given.ElapsedSec < 0 {
		return Result{}, apperr.Validationf("the elapsed time must not be negative")
	}

	accuracy := 1 - math.Abs(given.ElapsedSec-cfg.TargetSec)/cfg.TargetSec
	points := scale(maxPoints, accuracy)

	return Result{
		Correct:       points > 0,
		AwardedPoints: points,
		Detail:        "Timing scored against the target.",
	}, nil
}

type branchConfig struct {
	SolvePoints int `json:"solvePoints"`
	SkipPoints  int `json:"skipPoints"`
}

type branchPayload struct {
	Branch string `json:"branch"`
}

// validateBranch scores BRANCH, the one task where a crew can lose points:
// solving the detour pays, skipping it costs.
func validateBranch(config, payload json.RawMessage, _ int) (Result, error) {
	var cfg branchConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given branchPayload
	if err := decode(payload, &given, "choice"); err != nil {
		return Result{}, err
	}

	switch strings.ToLower(strings.TrimSpace(given.Branch)) {
	case branchSolve:
		return Result{Correct: true, AwardedPoints: cfg.SolvePoints, Detail: "Detour solved."}, nil
	case branchSkip:
		return Result{Correct: true, AwardedPoints: cfg.SkipPoints, Detail: "Detour skipped."}, nil
	default:
		return Result{}, apperr.Validationf("choose either %q or %q", branchSolve, branchSkip)
	}
}

type timedTriviaConfig struct {
	Answer   any     `json:"answer"`
	LimitSec float64 `json:"limitSec"`
}

type timedTriviaPayload struct {
	Answer     any     `json:"answer"`
	ElapsedSec float64 `json:"elapsedSec"`
}

// validateTimedTrivia scores TIMED_TRIVIA: the answer must be right *and*
// inside the time limit. A right answer that arrives late earns nothing, which
// is the whole point of the task.
func validateTimedTrivia(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg timedTriviaConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given timedTriviaPayload
	if err := decode(payload, &given, "answer"); err != nil {
		return Result{}, err
	}
	if cfg.LimitSec <= 0 {
		return Result{}, apperr.Validationf("this task has no time limit configured")
	}

	if given.ElapsedSec > cfg.LimitSec {
		return Result{Correct: false, AwardedPoints: 0, Detail: "Out of time."}, nil
	}

	correct := equalJSON(cfg.Answer, given.Answer)
	if expected, ok := cfg.Answer.(string); ok {
		if actual, ok := given.Answer.(string); ok {
			correct = strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(actual))
		}
	}

	return awarded(correct, maxPoints, "Correct, and in time.", detailIncorrect), nil
}

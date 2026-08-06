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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/taskengine"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

// Submission statuses. A row exists only once a crew has attempted the task.
const (
	// SubmissionCompleted means the task was attempted and scored.
	SubmissionCompleted = "completed"
	// SubmissionSkipped records a task the crew chose to pass on.
	SubmissionSkipped = "skipped"
)

// ErrTaskNotOnThisRally means the task belongs to a different event, so this
// crew has no business submitting it.
var ErrTaskNotOnThisRally = fmt.Errorf("%w: task", apperr.ErrNotFound)

// TaskWinner is who claimed a task, for telling a latecomer they were beaten.
type TaskWinner struct {
	CrewMemberID   string
	CrewMemberName string
	AwardedPoints  int
}

// ErrTaskClaimedBy reports that a teammate already answered this task.
//
// Named rather than anonymous because on four phones racing the same question,
// "someone got there first" is confusing and "Ayesha got there first" is the
// whole point of playing as a car.
func ErrTaskClaimedBy(winner TaskWinner) error {
	who := winner.CrewMemberName
	if who == "" {
		who = "a teammate"
	}

	return apperr.Conflictf("%s already answered this one for the car", who)
}

// SubmittableTask is the definition the engine needs to score an attempt.
type SubmittableTask struct {
	ID      string
	EventID string
	Code    string
	Type    tasks.TaskType
	Points  int
	Config  json.RawMessage
}

// Submission is one crew member's attempt at one task, on behalf of the car.
type Submission struct {
	ID           string
	SessionID    string
	TaskID       string
	CrewMemberID string
	WaypointID   *string
	Status       string
	Payload      json.RawMessage
	// AwardedPoints is what the engine decided, never what the phone claimed.
	AwardedPoints int
	SubmittedAt   time.Time
}

// SubmitTask scores an attempt and folds it into the car's total.
//
// Everything that decides the score happens here: the crew sends only what they
// did, and the engine — not the phone — says what it was worth.
//
// The first submission wins the task for the whole car. Four phones can be
// answering the same question at the same moment, so the winner is settled by a
// unique index rather than a read-then-write, and a latecomer is told who beat
// them instead of overwriting a score that was already earned. That reverses the
// single-phone behaviour where a resubmission corrected the total: with a race
// in play, letting the second answer replace the first would be a scoring bug.
func (s *Service) SubmitTask(
	ctx context.Context, sessionID, crewMemberID, taskID string, payload json.RawMessage,
) (taskengine.Result, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return taskengine.Result{}, err
	}
	if session.Status == StatusFinished {
		return taskengine.Result{}, ErrSessionFinished
	}

	task, err := s.repo.SubmittableTaskOf(ctx, taskID)
	if err != nil {
		return taskengine.Result{}, err
	}
	// A task from another event would be scored against the wrong rally.
	if task.EventID != session.EventID {
		return taskengine.Result{}, ErrTaskNotOnThisRally
	}

	if err := requireUnlocked(session, task); err != nil {
		return taskengine.Result{}, err
	}

	result, err := taskengine.Validate(task.Type, task.Config, payload, task.Points)
	if err != nil {
		return taskengine.Result{}, err
	}

	submission := Submission{
		ID:            store.NewID(),
		SessionID:     sessionID,
		TaskID:        taskID,
		CrewMemberID:  crewMemberID,
		WaypointID:    session.CurrentWaypointID,
		Status:        SubmissionCompleted,
		Payload:       payload,
		AwardedPoints: result.AwardedPoints,
		SubmittedAt:   time.Now().UTC(),
	}

	total, err := s.repo.SaveSubmission(ctx, submission)
	if err != nil {
		// A lost race is a normal outcome, not a failure to save: pass it
		// through unwrapped so it stays a 409 naming the winner.
		if errors.Is(err, apperr.ErrConflict) {
			return taskengine.Result{}, err
		}

		return taskengine.Result{}, fmt.Errorf("save submission for session %s: %w", sessionID, err)
	}

	s.publishScore(ctx, session, task, result, total, submission.CrewMemberID)

	return result, nil
}

// sensorTaskTypes read the car's own motion or position, so they are only
// meaningful where the car actually is.
var sensorTaskTypes = []tasks.TaskType{
	tasks.TypeTelematics,
	tasks.TypeGeofenceCross,
	tasks.TypeProximity,
}

// requireUnlocked gates the sensor-backed types on where the car is.
//
// These were once restricted to a designated phone, which bought nothing: their
// payloads are computed by the client, so any phone in the car could fabricate
// one, and tying them to a device only meant losing the points when that phone's
// battery died. Server state is the real check — the task has to be reachable
// from where the car currently is.
func requireUnlocked(session Session, task SubmittableTask) error {
	if !slices.Contains(sensorTaskTypes, task.Type) {
		return nil
	}
	if session.CurrentWaypointID == nil {
		return apperr.Conflictf("drive into the checkpoint before answering this one")
	}

	return nil
}

// publishScore tells the organizer's monitor and leaderboard what changed. A
// broadcast failure must not undo a score the crew has already earned, so
// nothing here is fatal.
func (s *Service) publishScore(
	ctx context.Context, session Session, task SubmittableTask,
	result taskengine.Result, total int, crewMemberID string,
) {
	code, err := s.repo.VehicleCodeOf(ctx, session.VehicleID)
	if err != nil {
		return
	}

	eventTopic := EventTopic(session.EventID)
	// delta is now a true delta: a submission row is written once and never
	// updated, so the awarded points are exactly the change to the total.
	s.broadcast(eventTopic, map[string]any{
		"type":        "score_delta",
		"vehicleCode": code,
		"delta":       result.AwardedPoints,
		"total":       total,
	})

	completed := map[string]any{
		"type":         "task_completed",
		"vehicleCode":  code,
		"taskCode":     task.Code,
		"taskId":       task.ID,
		"completedBy":  crewMemberID,
		"totalScore":   total,
		"awardedPoint": result.AwardedPoints,
	}
	if result.Correct {
		s.broadcast(eventTopic, completed)
	}

	// The car's other phones are showing this same question. They are told
	// regardless of whether the answer was right, because either way the task is
	// settled and leaving it open on three screens invites a wasted second try.
	s.broadcast(SessionTopic(session.ID), completed)
}

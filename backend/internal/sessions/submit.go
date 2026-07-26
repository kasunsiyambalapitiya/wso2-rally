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
	"fmt"
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

// SubmittableTask is the definition the engine needs to score an attempt.
type SubmittableTask struct {
	ID      string
	EventID string
	Code    string
	Type    tasks.TaskType
	Points  int
	Config  json.RawMessage
}

// Submission is one crew's attempt at one task.
type Submission struct {
	ID            string
	SessionID     string
	TaskID        string
	WaypointID    *string
	Status        string
	Payload       json.RawMessage
	AwardedPoints int
	SubmittedAt   time.Time
}

// SubmitTask scores an attempt and folds it into the team's total.
//
// Everything that decides the score happens here: the crew sends only what
// they did, and the engine — not the phone — says what it was worth. The score
// is recomputed from the stored submissions rather than incremented, so a
// resubmission corrects the total instead of double-counting it.
func (s *Service) SubmitTask(
	ctx context.Context, sessionID, taskID string, payload json.RawMessage,
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

	result, err := taskengine.Validate(task.Type, task.Config, payload, task.Points)
	if err != nil {
		return taskengine.Result{}, err
	}

	submission := Submission{
		ID:            store.NewID(),
		SessionID:     sessionID,
		TaskID:        taskID,
		WaypointID:    session.CurrentWaypointID,
		Status:        SubmissionCompleted,
		Payload:       payload,
		AwardedPoints: result.AwardedPoints,
		SubmittedAt:   time.Now().UTC(),
	}

	total, err := s.repo.SaveSubmission(ctx, submission)
	if err != nil {
		return taskengine.Result{}, fmt.Errorf("save submission for session %s: %w", sessionID, err)
	}

	s.publishScore(ctx, session, task, result, total)

	return result, nil
}

// publishScore tells the organizer's monitor and leaderboard what changed. A
// broadcast failure must not undo a score the crew has already earned, so
// nothing here is fatal.
func (s *Service) publishScore(
	ctx context.Context, session Session, task SubmittableTask, result taskengine.Result, total int,
) {
	code, err := s.repo.VehicleCodeOf(ctx, session.VehicleID)
	if err != nil {
		return
	}

	topic := EventTopic(session.EventID)
	s.broadcast(topic, map[string]any{
		"type":        "score_delta",
		"vehicleCode": code,
		"delta":       result.AwardedPoints,
		"total":       total,
	})
	if result.Correct {
		s.broadcast(topic, map[string]any{
			"type":        "task_completed",
			"vehicleCode": code,
			"taskCode":    task.Code,
		})
	}
}

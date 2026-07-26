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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
)

// emptyConfig is what a task stores when the organizer has not authored any
// parameters yet. The column is NOT NULL, and the taskengine expects an object.
var emptyConfig = json.RawMessage(`{}`)

// Repo is the persistence contract for task definitions.
type Repo interface {
	Create(ctx context.Context, task Task) error
	Get(ctx context.Context, id string) (Task, error)
	Update(ctx context.Context, task Task) error
	Search(ctx context.Context, eventID string, page httpx.Page) ([]Task, int, error)
}

// Service holds the task-authoring rules.
type Service struct {
	repo Repo
}

// NewService wires a Service to its repository.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// Create authors a new task after checking its type, trigger, sensor, and that
// its config is a JSON object the engine can read.
func (s *Service) Create(ctx context.Context, in CreateTaskInput) (Task, error) {
	task := Task{
		ID:      store.NewID(),
		EventID: in.EventID,
		Code:    strings.TrimSpace(in.Code),
		Title:   strings.TrimSpace(in.Title),
		Type:    in.Type,
		Trigger: in.Trigger,
		Points:  in.Points,
		Sensor:  in.Sensor,
		Config:  in.Config,
	}
	if task.Sensor == "" {
		task.Sensor = SensorNone
	}
	if len(bytes.TrimSpace(task.Config)) == 0 {
		task.Config = emptyConfig
	}

	if task.EventID == "" {
		return Task{}, apperr.Validationf("event id is required")
	}
	if err := validate(task); err != nil {
		return Task{}, err
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}

// Get returns one task definition, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Task, error) {
	return s.repo.Get(ctx, id)
}

// Update applies the non-nil fields of in and re-validates the whole task, so
// a retune can never leave a task the engine cannot score.
func (s *Service) Update(ctx context.Context, id string, in UpdateTaskInput) (Task, error) {
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		return Task{}, err
	}

	if in.Code != nil {
		task.Code = strings.TrimSpace(*in.Code)
	}
	if in.Title != nil {
		task.Title = strings.TrimSpace(*in.Title)
	}
	if in.Type != nil {
		task.Type = *in.Type
	}
	if in.Trigger != nil {
		task.Trigger = *in.Trigger
	}
	if in.Points != nil {
		task.Points = *in.Points
	}
	if in.Sensor != nil {
		task.Sensor = *in.Sensor
	}
	if len(bytes.TrimSpace(in.Config)) > 0 {
		task.Config = in.Config
	}

	if err := validate(task); err != nil {
		return Task{}, err
	}
	if err := s.repo.Update(ctx, task); err != nil {
		return Task{}, fmt.Errorf("update task %s: %w", id, err)
	}

	return task, nil
}

// Search returns a page of an event's tasks plus the unpaged total.
func (s *Service) Search(ctx context.Context, eventID string, page httpx.Page) ([]Task, int, error) {
	if eventID == "" {
		return nil, 0, apperr.Validationf("event id is required")
	}

	found, total, err := s.repo.Search(ctx, eventID, page)
	if err != nil {
		return nil, 0, fmt.Errorf("search tasks of event %s: %w", eventID, err)
	}

	return found, total, nil
}

func validate(task Task) error {
	if task.Code == "" {
		return apperr.Validationf("task code is required")
	}
	if task.Title == "" {
		return apperr.Validationf("task title is required")
	}
	if !task.Type.IsValid() {
		return apperr.Validationf("unknown task type %q", task.Type)
	}
	if !task.Trigger.IsValid() {
		return apperr.Validationf("unknown task trigger %q", task.Trigger)
	}
	if !task.Sensor.IsValid() {
		return apperr.Validationf("unknown task sensor %q", task.Sensor)
	}
	// Only a BRANCH may cost points: it is the one task where taking the
	// shortcut is a scored decision rather than a failure.
	if task.Points < 0 && task.Type != TypeBranch {
		return apperr.Validationf("task points must not be negative for a %s task", task.Type)
	}

	return validateConfig(task.Config)
}

// validateConfig requires a JSON object. The engine indexes config by key, so
// an array or a bare scalar would fail at run time, in the middle of a rally.
func validateConfig(config json.RawMessage) error {
	if !json.Valid(config) {
		return apperr.Validationf("task config must be valid JSON")
	}
	if trimmed := bytes.TrimSpace(config); len(trimmed) == 0 || trimmed[0] != '{' {
		return apperr.Validationf("task config must be a JSON object")
	}

	return nil
}

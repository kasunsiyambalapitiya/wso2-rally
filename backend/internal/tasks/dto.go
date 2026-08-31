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

import "encoding/json"

// TaskDTO is a task definition on the wire. Config passes through untouched:
// its shape is per-type and only the taskengine and the matching micro-app
// screen interpret it.
type TaskDTO struct {
	ID      string          `json:"id"`
	EventID string          `json:"eventId"`
	Code    string          `json:"code"`
	Title   string          `json:"title"`
	Type    string          `json:"type"`
	Trigger string          `json:"trigger"`
	Points  int             `json:"points"`
	Sensor  string          `json:"sensor"`
	Config  json.RawMessage `json:"config"`
}

// CreateTaskRequest is the POST /events/{eventId}/tasks body.
type CreateTaskRequest struct {
	Code    string          `json:"code"`
	Title   string          `json:"title"`
	Type    string          `json:"type"`
	Trigger string          `json:"trigger"`
	Points  int             `json:"points"`
	Sensor  string          `json:"sensor"`
	Config  json.RawMessage `json:"config"`
}

// UpdateTaskRequest is the PATCH /tasks/{taskId} body.
type UpdateTaskRequest struct {
	Code    *string         `json:"code"`
	Title   *string         `json:"title"`
	Type    *string         `json:"type"`
	Trigger *string         `json:"trigger"`
	Points  *int            `json:"points"`
	Sensor  *string         `json:"sensor"`
	Config  json.RawMessage `json:"config"`
}

// SearchTasksRequest is the POST /events/{eventId}/tasks/search body.
type SearchTasksRequest struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

func toDTO(task Task) TaskDTO {
	config := task.Config
	if len(config) == 0 {
		config = emptyConfig
	}

	return TaskDTO{
		ID:      task.ID,
		EventID: task.EventID,
		Code:    task.Code,
		Title:   task.Title,
		Type:    string(task.Type),
		Trigger: string(task.Trigger),
		Points:  task.Points,
		Sensor:  string(task.Sensor),
		Config:  config,
	}
}

func toDTOs(list []Task) []TaskDTO {
	out := make([]TaskDTO, 0, len(list))
	for _, task := range list {
		out = append(out, toDTO(task))
	}

	return out
}

func (r CreateTaskRequest) toCreateInput(eventID string) CreateTaskInput {
	return CreateTaskInput{
		EventID: eventID,
		Code:    r.Code,
		Title:   r.Title,
		Type:    TaskType(r.Type),
		Trigger: Trigger(r.Trigger),
		Points:  r.Points,
		Sensor:  Sensor(r.Sensor),
		Config:  r.Config,
	}
}

func (r UpdateTaskRequest) toUpdateInput() UpdateTaskInput {
	in := UpdateTaskInput{
		Code:   r.Code,
		Title:  r.Title,
		Points: r.Points,
		Config: r.Config,
	}
	if r.Type != nil {
		taskType := TaskType(*r.Type)
		in.Type = &taskType
	}
	if r.Trigger != nil {
		trigger := Trigger(*r.Trigger)
		in.Trigger = &trigger
	}
	if r.Sensor != nil {
		sensor := Sensor(*r.Sensor)
		in.Sensor = &sensor
	}

	return in
}

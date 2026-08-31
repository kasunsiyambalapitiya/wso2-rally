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
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

const eventID = "0123456789abcdef0123456789abcdef"

type fakeRepo struct {
	tasks     map[string]Task
	codeTaken bool
}

func newFakeRepo() *fakeRepo { return &fakeRepo{tasks: map[string]Task{}} }

func (f *fakeRepo) Create(_ context.Context, task Task) error {
	if f.codeTaken {
		return ErrDuplicateCode
	}
	f.tasks[task.ID] = task
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (Task, error) {
	task, ok := f.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	return task, nil
}

func (f *fakeRepo) Update(_ context.Context, task Task) error {
	if _, ok := f.tasks[task.ID]; !ok {
		return ErrNotFound
	}
	f.tasks[task.ID] = task
	return nil
}

func (f *fakeRepo) Search(_ context.Context, eventID string, page httpx.Page) ([]Task, int, error) {
	var matched []Task
	for _, task := range f.tasks {
		if task.EventID == eventID {
			matched = append(matched, task)
		}
	}
	slices.SortFunc(matched, func(a, b Task) int { return len(a.Code) - len(b.Code) })
	total := len(matched)
	if page.Offset >= total {
		return nil, total, nil
	}

	return matched[page.Offset:min(page.Offset+page.Limit, total)], total, nil
}

func validInput() CreateTaskInput {
	return CreateTaskInput{
		EventID: eventID,
		Code:    "T1",
		Title:   "Translation Cipher",
		Type:    TypeInputSelect,
		Trigger: TriggerGeofence,
		Points:  50,
		Sensor:  SensorNone,
		Config:  json.RawMessage(`{"answer":"API Integration"}`),
	}
}

func TestTaskType_CoversEveryTaskInTheSpec(t *testing.T) {
	// The rally runs fifteen tasks over thirteen distinct types.
	require.Len(t, AllTypes(), 13)

	for _, taskType := range AllTypes() {
		require.True(t, taskType.IsValid(), "%s should be valid", taskType)
	}
	require.False(t, TaskType("NOPE").IsValid())
	require.False(t, TaskType("").IsValid())
}

func TestService_Create_AssignsIDAndStoresConfig(t *testing.T) {
	svc := NewService(newFakeRepo())

	got, err := svc.Create(context.Background(), validInput())

	require.NoError(t, err)
	require.Len(t, got.ID, 32)
	require.JSONEq(t, `{"answer":"API Integration"}`, string(got.Config))
}

func TestService_Create_RejectsUnknownType(t *testing.T) {
	in := validInput()
	in.Type = "NOPE"

	_, err := NewService(newFakeRepo()).Create(context.Background(), in)

	require.ErrorIs(t, err, apperr.ErrValidation)
	require.Contains(t, err.Error(), "NOPE")
}

func TestService_Create_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CreateTaskInput)
		wantMsg string
	}{
		{"missing event", func(in *CreateTaskInput) { in.EventID = "" }, "event id"},
		{"blank code", func(in *CreateTaskInput) { in.Code = " " }, "code"},
		{"blank title", func(in *CreateTaskInput) { in.Title = "" }, "title"},
		{"unknown trigger", func(in *CreateTaskInput) { in.Trigger = "whenever" }, "trigger"},
		{"unknown sensor", func(in *CreateTaskInput) { in.Sensor = "lidar" }, "sensor"},
		{"negative points", func(in *CreateTaskInput) { in.Points = -1 }, "points"},
		{"malformed config", func(in *CreateTaskInput) { in.Config = json.RawMessage(`{oops`) }, "config"},
		{"config is not an object", func(in *CreateTaskInput) { in.Config = json.RawMessage(`[1,2]`) }, "config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)

			_, err := NewService(newFakeRepo()).Create(context.Background(), in)

			require.ErrorIs(t, err, apperr.ErrValidation)
			require.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// BRANCH tasks may award negative points for taking the shortcut, so the
// non-negative rule must not apply to them.
func TestService_Create_AllowsNegativePointsForBranch(t *testing.T) {
	in := validInput()
	in.Type = TypeBranch
	in.Trigger = TriggerChoice
	in.Points = -40

	got, err := NewService(newFakeRepo()).Create(context.Background(), in)

	require.NoError(t, err)
	require.Equal(t, -40, got.Points)
}

func TestService_Create_DefaultsEmptyConfigToAnObject(t *testing.T) {
	in := validInput()
	in.Config = nil

	got, err := NewService(newFakeRepo()).Create(context.Background(), in)

	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(got.Config))
}

func TestService_Create_DefaultsEmptySensorToNone(t *testing.T) {
	in := validInput()
	in.Sensor = ""

	got, err := NewService(newFakeRepo()).Create(context.Background(), in)

	require.NoError(t, err)
	require.Equal(t, SensorNone, got.Sensor)
}

func TestService_Create_DuplicateCodeIsConflict(t *testing.T) {
	repo := newFakeRepo()
	repo.codeTaken = true

	_, err := NewService(repo).Create(context.Background(), validInput())

	require.ErrorIs(t, err, apperr.ErrConflict)
}

func TestService_Get_UnknownIsNotFound(t *testing.T) {
	_, err := NewService(newFakeRepo()).Get(context.Background(), "missing")

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_Update_AppliesOnlyProvidedFields(t *testing.T) {
	svc := NewService(newFakeRepo())
	created, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	newPoints := 75

	updated, err := svc.Update(context.Background(), created.ID, UpdateTaskInput{Points: &newPoints})

	require.NoError(t, err)
	require.Equal(t, 75, updated.Points)
	require.Equal(t, created.Title, updated.Title)
	require.JSONEq(t, string(created.Config), string(updated.Config))
}

func TestService_Update_RejectsUnknownType(t *testing.T) {
	svc := NewService(newFakeRepo())
	created, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	badType := TaskType("NOPE")

	_, err = svc.Update(context.Background(), created.ID, UpdateTaskInput{Type: &badType})

	require.ErrorIs(t, err, apperr.ErrValidation)
}

func TestService_Update_ReplacesConfig(t *testing.T) {
	svc := NewService(newFakeRepo())
	created, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	newConfig := json.RawMessage(`{"answer":"Service Mesh"}`)

	updated, err := svc.Update(context.Background(), created.ID, UpdateTaskInput{Config: newConfig})

	require.NoError(t, err)
	require.JSONEq(t, `{"answer":"Service Mesh"}`, string(updated.Config))
}

func TestService_Search_FiltersByEvent(t *testing.T) {
	svc := NewService(newFakeRepo())
	ctx := context.Background()
	_, err := svc.Create(ctx, validInput())
	require.NoError(t, err)
	other := validInput()
	other.EventID = "ffffffffffffffffffffffffffffffff"
	other.Code = "T2"
	_, err = svc.Create(ctx, other)
	require.NoError(t, err)

	found, total, err := svc.Search(ctx, eventID, httpx.Page{Offset: 0, Limit: 20})

	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, found, 1)
}

func TestService_Search_RequiresEventID(t *testing.T) {
	_, _, err := NewService(newFakeRepo()).Search(context.Background(), "", httpx.Page{Limit: 20})

	require.ErrorIs(t, err, apperr.ErrValidation)
}

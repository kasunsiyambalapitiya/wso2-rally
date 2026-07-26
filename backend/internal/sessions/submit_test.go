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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

func TestService_SubmitTask_ScoresAndTotalsUp(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)

	result, err := svc.SubmitTask(context.Background(), session.ID, "task-1",
		json.RawMessage(`{"answer":"API Integration"}`))

	require.NoError(t, err)
	require.True(t, result.Correct)
	require.Equal(t, 50, result.AwardedPoints)
	require.Equal(t, 50, repo.totals[session.ID])
}

func TestService_SubmitTask_WrongAnswerScoresZero(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)

	result, err := svc.SubmitTask(context.Background(), session.ID, "task-1",
		json.RawMessage(`{"answer":"Service Mesh"}`))

	require.NoError(t, err)
	require.False(t, result.Correct)
	require.Zero(t, result.AwardedPoints)
}

// Resubmitting corrects the total rather than adding to it.
func TestService_SubmitTask_ResubmissionReplacesTheScore(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)
	ctx := context.Background()
	_, err := svc.SubmitTask(ctx, session.ID, "task-1", json.RawMessage(`{"answer":"API Integration"}`))
	require.NoError(t, err)

	_, err = svc.SubmitTask(ctx, session.ID, "task-1", json.RawMessage(`{"answer":"Wrong"}`))

	require.NoError(t, err)
	require.Zero(t, repo.totals[session.ID], "the second attempt replaces the first")
}

func TestService_SubmitTask_BroadcastsScoreDelta(t *testing.T) {
	svc, _, _, sent := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.SubmitTask(context.Background(), session.ID, "task-1",
		json.RawMessage(`{"answer":"API Integration"}`))

	require.NoError(t, err)
	require.Contains(t, messageTypes(*sent), "score_delta")
	require.Contains(t, messageTypes(*sent), "task_completed")
}

func TestService_SubmitTask_AfterFinishIsRejected(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)
	ctx := context.Background()
	_, err := svc.Finish(ctx, session.ID)
	require.NoError(t, err)

	_, err = svc.SubmitTask(ctx, session.ID, "task-1", json.RawMessage(`{"answer":"API Integration"}`))

	require.ErrorIs(t, err, ErrSessionFinished)
}

// A task from another rally must not be scoreable, however it was discovered.
func TestService_SubmitTask_RejectsTaskFromAnotherEvent(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)
	repo.submittable["task-foreign"] = SubmittableTask{
		ID: "task-foreign", EventID: "another-event", Code: "T9",
		Type: tasks.TypeInputSelect, Points: 50, Config: json.RawMessage(`{"answer":"x"}`),
	}

	_, err := svc.SubmitTask(context.Background(), session.ID, "task-foreign", json.RawMessage(`{"answer":"x"}`))

	require.ErrorIs(t, err, ErrTaskNotOnThisRally)
}

func TestService_SubmitTask_UnknownTask(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.SubmitTask(context.Background(), session.ID, "missing", json.RawMessage(`{}`))

	require.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestService_SubmitTask_MalformedPayloadIsValidation(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.SubmitTask(context.Background(), session.ID, "task-1", json.RawMessage(`{"answer":`))

	require.ErrorIs(t, err, apperr.ErrValidation)
}

func messageTypes(records []broadcastRecord) []string {
	types := make([]string, 0, len(records))
	for _, record := range records {
		if fields, ok := record.message.(map[string]any); ok {
			if messageType, ok := fields["type"].(string); ok {
				types = append(types, messageType)
			}
		}
	}

	return types
}

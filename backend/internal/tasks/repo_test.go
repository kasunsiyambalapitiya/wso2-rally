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
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/storetest"
)

func seedEvent(t *testing.T, db *sql.DB) string {
	t.Helper()

	id := store.NewID()
	_, err := db.Exec(
		"INSERT INTO event (id, name, event_date, start_time, created_by) VALUES (?, 'Rally', '2027-02-13', '09:00', 'u')",
		id)
	require.NoError(t, err)

	return id
}

// The `trigger` column is a MySQL reserved word; this exercises the backticked
// SQL end to end.
func TestRepo_RoundTripsEveryTaskType(t *testing.T) {
	db := storetest.DB(t)
	repo := NewRepo(db)
	svc := NewService(repo)
	ctx := context.Background()
	eventID := seedEvent(t, db)

	for i, taskType := range AllTypes() {
		in := CreateTaskInput{
			EventID: eventID,
			Code:    "T" + string(rune('a'+i)),
			Title:   string(taskType),
			Type:    taskType,
			Trigger: TriggerGeofence,
			Points:  50,
			Sensor:  SensorGeolocation,
			Config:  json.RawMessage(`{"answer":"x"}`),
		}
		created, err := svc.Create(ctx, in)
		require.NoError(t, err)

		got, err := repo.Get(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, taskType, got.Type)
		require.Equal(t, TriggerGeofence, got.Trigger)
		require.Equal(t, SensorGeolocation, got.Sensor)
		require.JSONEq(t, `{"answer":"x"}`, string(got.Config))
	}
}

func TestRepo_DuplicateCodeInSameEvent(t *testing.T) {
	db := storetest.DB(t)
	svc := NewService(NewRepo(db))
	ctx := context.Background()
	in := validInput()
	in.EventID = seedEvent(t, db)
	_, err := svc.Create(ctx, in)
	require.NoError(t, err)

	_, err = svc.Create(ctx, in)

	require.ErrorIs(t, err, ErrDuplicateCode)
}

func TestRepo_SameCodeInDifferentEventsIsAllowed(t *testing.T) {
	db := storetest.DB(t)
	svc := NewService(NewRepo(db))
	ctx := context.Background()

	for range 2 {
		in := validInput()
		in.EventID = seedEvent(t, db)
		_, err := svc.Create(ctx, in)
		require.NoError(t, err)
	}
}

func TestRepo_Get_UnknownIsNotFound(t *testing.T) {
	repo := NewRepo(storetest.DB(t))

	_, err := repo.Get(context.Background(), store.NewID())

	require.ErrorIs(t, err, ErrNotFound)
}

func TestRepo_Update_PersistsConfigAndPoints(t *testing.T) {
	db := storetest.DB(t)
	repo := NewRepo(db)
	svc := NewService(repo)
	ctx := context.Background()
	in := validInput()
	in.EventID = seedEvent(t, db)
	created, err := svc.Create(ctx, in)
	require.NoError(t, err)
	newPoints := 90

	_, err = svc.Update(ctx, created.ID, UpdateTaskInput{
		Points: &newPoints,
		Config: json.RawMessage(`{"answer":"Service Mesh","tolerance":2}`),
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, 90, got.Points)
	require.JSONEq(t, `{"answer":"Service Mesh","tolerance":2}`, string(got.Config))
}

func TestRepo_Search_OrdersCodesNumerically(t *testing.T) {
	db := storetest.DB(t)
	repo := NewRepo(db)
	svc := NewService(repo)
	ctx := context.Background()
	eventID := seedEvent(t, db)
	for _, code := range []string{"T10", "T2", "T1"} {
		in := validInput()
		in.EventID = eventID
		in.Code = code
		_, err := svc.Create(ctx, in)
		require.NoError(t, err)
	}

	found, total, err := repo.Search(ctx, eventID, httpx.Page{Offset: 0, Limit: 20})

	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Equal(t, []string{"T1", "T2", "T10"}, []string{found[0].Code, found[1].Code, found[2].Code})
}

func TestRepo_Search_EmptyEvent(t *testing.T) {
	db := storetest.DB(t)

	found, total, err := NewRepo(db).Search(context.Background(), seedEvent(t, db), httpx.Page{Offset: 0, Limit: 20})

	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, found)
}

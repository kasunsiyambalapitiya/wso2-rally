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

package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/storetest"
)

func TestRepo_CreateGetRoundTrip(t *testing.T) {
	repo := NewRepo(storetest.DB(t))
	ctx := context.Background()
	want, err := NewService(repo).Create(ctx, validInput())
	require.NoError(t, err)

	got, err := repo.Get(ctx, want.ID)

	require.NoError(t, err)
	require.Equal(t, want.Name, got.Name)
	require.Equal(t, want.StartTime, got.StartTime)
	require.Equal(t, StatusSetup, got.Status)
	require.Equal(t, want.Cipher, got.Cipher)
	require.InDelta(t, *want.Start.Lat, *got.Start.Lat, 1e-9)
	require.Equal(t, want.Start.RadiusM, got.Start.RadiusM)
	require.Equal(t, want.EventDate.Format(dateLayout), got.EventDate.Format(dateLayout))
}

func TestRepo_Get_UnknownIsNotFound(t *testing.T) {
	repo := NewRepo(storetest.DB(t))

	_, err := repo.Get(context.Background(), "0123456789abcdef0123456789abcdef")

	require.ErrorIs(t, err, ErrNotFound)
}

func TestRepo_Update_PersistsAndReportsMissingRows(t *testing.T) {
	repo := NewRepo(storetest.DB(t))
	ctx := context.Background()
	created, err := NewService(repo).Create(ctx, validInput())
	require.NoError(t, err)

	created.Status = StatusActive
	created.Name = "Renamed"
	require.NoError(t, repo.Update(ctx, created))

	got, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, StatusActive, got.Status)
	require.Equal(t, "Renamed", got.Name)

	require.ErrorIs(t, repo.Update(ctx, Event{ID: "0123456789abcdef0123456789abcdef"}), ErrNotFound)
}

// An unplaced geofence must round-trip as NULL, not as a zero coordinate off
// the coast of Africa.
func TestRepo_NullBoundaryRoundTrips(t *testing.T) {
	repo := NewRepo(storetest.DB(t))
	ctx := context.Background()
	in := validInput()
	in.End = Boundary{RadiusM: 30}
	created, err := NewService(repo).Create(ctx, in)
	require.NoError(t, err)

	got, err := repo.Get(ctx, created.ID)

	require.NoError(t, err)
	require.Nil(t, got.End.Lat)
	require.Nil(t, got.End.Lng)
	require.Empty(t, got.End.Label)
	require.False(t, got.End.IsPlaced())
}

func TestRepo_Search_FiltersAndPaginates(t *testing.T) {
	repo := NewRepo(storetest.DB(t))
	ctx := context.Background()
	svc := NewService(repo)
	for i := range 3 {
		in := validInput()
		in.Name = string(rune('A'+i)) + " Rally"
		created, err := svc.Create(ctx, in)
		require.NoError(t, err)
		if i == 0 {
			created.Status = StatusActive
			require.NoError(t, repo.Update(ctx, created))
		}
	}

	setup, total, err := repo.Search(ctx, httpx.Page{Offset: 0, Limit: 10}, SearchFilter{Status: StatusSetup})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, setup, 2)

	firstPage, total, err := repo.Search(ctx, httpx.Page{Offset: 0, Limit: 1}, SearchFilter{})
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, firstPage, 1)

	beyondEnd, total, err := repo.Search(ctx, httpx.Page{Offset: 99, Limit: 10}, SearchFilter{})
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Empty(t, beyondEnd)
}

func TestRepo_Search_EmptyTable(t *testing.T) {
	repo := NewRepo(storetest.DB(t))

	found, total, err := repo.Search(context.Background(), httpx.Page{Offset: 0, Limit: 10}, SearchFilter{})

	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, found)
}

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

package debrief

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

const eventID = "0123456789abcdef0123456789abcdef"

type fakeRepo struct {
	stored []Video
}

func (f *fakeRepo) Create(_ context.Context, v Video) error {
	f.stored = append(f.stored, v)
	return nil
}

func (f *fakeRepo) Search(
	_ context.Context, eventID string, filter SearchFilter, page httpx.Page,
) ([]Video, int, error) {
	var matched []Video
	for _, v := range f.stored {
		if v.EventID != eventID {
			continue
		}
		if filter.Day > 0 && v.Day != filter.Day {
			continue
		}
		matched = append(matched, v)
	}
	total := len(matched)
	if page.Offset >= total {
		return nil, total, nil
	}

	return matched[page.Offset:min(page.Offset+page.Limit, total)], total, nil
}

func TestService_Add_AssignsIDAndDefaultsToDayOne(t *testing.T) {
	svc := NewService(&fakeRepo{})

	got, err := svc.Add(context.Background(), AddVideoInput{
		EventID: eventID, ObjectKey: "s3://rally/day1/opening.mp4",
	})

	require.NoError(t, err)
	require.Len(t, got.ID, 32)
	require.Equal(t, 1, got.Day)
	require.False(t, got.UploadedAt.IsZero())
}

func TestService_Add_Validation(t *testing.T) {
	tests := []struct {
		name    string
		in      AddVideoInput
		wantMsg string
	}{
		{"missing event", AddVideoInput{ObjectKey: "s3://x"}, "event id"},
		{"blank object key", AddVideoInput{EventID: eventID, ObjectKey: "   "}, "video URL"},
		{"day beyond the rally", AddVideoInput{EventID: eventID, ObjectKey: "s3://x", Day: 99}, "day must be"},
		{"negative day", AddVideoInput{EventID: eventID, ObjectKey: "s3://x", Day: -1}, "day must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(&fakeRepo{}).Add(context.Background(), tt.in)

			require.ErrorIs(t, err, apperr.ErrValidation)
			require.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestService_Add_KeepsTheVehicleWhenGiven(t *testing.T) {
	vehicleID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	got, err := NewService(&fakeRepo{}).Add(context.Background(), AddVideoInput{
		EventID: eventID, ObjectKey: "s3://x", Day: 2, VehicleID: &vehicleID,
	})

	require.NoError(t, err)
	require.NotNil(t, got.VehicleID)
	require.Equal(t, vehicleID, *got.VehicleID)
}

func TestService_Search_FiltersByDay(t *testing.T) {
	svc := NewService(&fakeRepo{})
	ctx := context.Background()
	for _, day := range []int{1, 1, 2} {
		_, err := svc.Add(ctx, AddVideoInput{EventID: eventID, ObjectKey: "s3://x", Day: day})
		require.NoError(t, err)
	}

	dayOne, total, err := svc.Search(ctx, eventID, SearchFilter{Day: 1}, httpx.Page{Offset: 0, Limit: 20})

	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, dayOne, 2)
}

func TestService_Search_NoFilterReturnsEveryDay(t *testing.T) {
	svc := NewService(&fakeRepo{})
	ctx := context.Background()
	for _, day := range []int{1, 2} {
		_, err := svc.Add(ctx, AddVideoInput{EventID: eventID, ObjectKey: "s3://x", Day: day})
		require.NoError(t, err)
	}

	_, total, err := svc.Search(ctx, eventID, SearchFilter{}, httpx.Page{Offset: 0, Limit: 20})

	require.NoError(t, err)
	require.Equal(t, 2, total)
}

func TestService_Search_Validation(t *testing.T) {
	svc := NewService(&fakeRepo{})

	_, _, err := svc.Search(context.Background(), "", SearchFilter{}, httpx.Page{Limit: 20})
	require.ErrorIs(t, err, apperr.ErrValidation)

	_, _, err = svc.Search(context.Background(), eventID, SearchFilter{Day: -1}, httpx.Page{Limit: 20})
	require.ErrorIs(t, err, apperr.ErrValidation)
}

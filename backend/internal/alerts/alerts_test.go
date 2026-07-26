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

package alerts

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/vehicles"
)

const (
	vehicleID = "0123456789abcdef0123456789abcdef"
	eventID   = "ffffffffffffffffffffffffffffffff"
)

type fakeRepo struct {
	stored map[string]Alert
}

func newFakeRepo() *fakeRepo { return &fakeRepo{stored: map[string]Alert{}} }

func (f *fakeRepo) Create(_ context.Context, a Alert) error {
	f.stored[a.ID] = a
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (Alert, error) {
	a, ok := f.stored[id]
	if !ok {
		return Alert{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeRepo) Resolve(_ context.Context, id string, at time.Time) error {
	a, ok := f.stored[id]
	if !ok {
		return ErrNotFound
	}
	a.ResolvedAt = &at
	f.stored[id] = a
	return nil
}

func (f *fakeRepo) Search(_ context.Context, _ string, filter SearchFilter, page httpx.Page) ([]Alert, int, error) {
	var matched []Alert
	for _, a := range f.stored {
		if filter.OpenOnly && !a.IsOpen() {
			continue
		}
		matched = append(matched, a)
	}
	total := len(matched)
	if page.Offset >= total {
		return nil, total, nil
	}
	return matched[page.Offset:min(page.Offset+page.Limit, total)], total, nil
}

func (f *fakeRepo) OpenTypesOf(_ context.Context, vehicleID string) ([]Type, error) {
	var types []Type
	for _, a := range f.stored {
		if a.VehicleID == vehicleID && a.IsOpen() {
			types = append(types, a.Type)
		}
	}
	return types, nil
}

func (f *fakeRepo) EventIDOf(context.Context, string) (string, error) { return eventID, nil }

// fakeVehicles records the status transitions the alert service drives.
type fakeVehicles struct {
	status  vehicles.Status
	history []vehicles.Status
	err     error
}

func (f *fakeVehicles) SetStatus(_ context.Context, _ string, status vehicles.Status) error {
	if f.err != nil {
		return f.err
	}
	f.status = status
	f.history = append(f.history, status)
	return nil
}

func newService(t *testing.T) (*Service, *fakeRepo, *fakeVehicles, *[]any) {
	t.Helper()

	repo, vehiclesStub := newFakeRepo(), &fakeVehicles{status: vehicles.StatusOK}
	var broadcasts []any
	svc := NewService(repo, vehiclesStub, func(_ string, message any) {
		broadcasts = append(broadcasts, message)
	})

	return svc, repo, vehiclesStub, &broadcasts
}

func raiseInput(alertType Type) RaiseAlertInput {
	return RaiseAlertInput{
		VehicleID: vehicleID,
		Type:      alertType,
		Note:      "Front left tyre",
		Source:    SourceOrganizer,
		RaisedBy:  "organizer@wso2.com",
	}
}

func TestService_Raise_AssignsIDAndTimestamp(t *testing.T) {
	svc, _, _, _ := newService(t)

	got, err := svc.Raise(context.Background(), raiseInput(TypeBreakdown))

	require.NoError(t, err)
	require.Len(t, got.ID, 32)
	require.False(t, got.RaisedAt.IsZero())
	require.True(t, got.IsOpen())
}

func TestService_Raise_SetsVehicleStatus(t *testing.T) {
	tests := []struct {
		alertType Type
		want      vehicles.Status
	}{
		{TypeBreakdown, vehicles.StatusBreakdown},
		{TypeDeviceIssue, vehicles.StatusDeviceIssue},
	}
	for _, tt := range tests {
		t.Run(string(tt.alertType), func(t *testing.T) {
			svc, _, vehiclesStub, _ := newService(t)

			_, err := svc.Raise(context.Background(), raiseInput(tt.alertType))

			require.NoError(t, err)
			require.Equal(t, tt.want, vehiclesStub.status)
		})
	}
}

// "Other" is informational: the car keeps running.
func TestService_Raise_OtherLeavesVehicleStatusAlone(t *testing.T) {
	svc, _, vehiclesStub, _ := newService(t)

	_, err := svc.Raise(context.Background(), raiseInput(TypeOther))

	require.NoError(t, err)
	require.Empty(t, vehiclesStub.history)
}

func TestService_Raise_Broadcasts(t *testing.T) {
	svc, _, _, broadcasts := newService(t)

	_, err := svc.Raise(context.Background(), raiseInput(TypeBreakdown))

	require.NoError(t, err)
	require.Len(t, *broadcasts, 1)
}

func TestService_Raise_Validation(t *testing.T) {
	badLat := 95.0
	tests := []struct {
		name   string
		mutate func(*RaiseAlertInput)
	}{
		{"missing vehicle", func(in *RaiseAlertInput) { in.VehicleID = "" }},
		{"unknown type", func(in *RaiseAlertInput) { in.Type = "on fire" }},
		{"unknown source", func(in *RaiseAlertInput) { in.Source = "bystander" }},
		{"half a coordinate", func(in *RaiseAlertInput) { in.Lat = &badLat }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := newService(t)
			in := raiseInput(TypeBreakdown)
			tt.mutate(&in)

			_, err := svc.Raise(context.Background(), in)

			require.ErrorIs(t, err, apperr.ErrValidation)
		})
	}
}

func TestService_Raise_DefaultsSourceToOrganizer(t *testing.T) {
	svc, _, _, _ := newService(t)
	in := raiseInput(TypeBreakdown)
	in.Source = ""

	got, err := svc.Raise(context.Background(), in)

	require.NoError(t, err)
	require.Equal(t, SourceOrganizer, got.Source)
}

func TestService_Resolve_SetsResolvedAtAndRestoresStatus(t *testing.T) {
	svc, _, vehiclesStub, _ := newService(t)
	ctx := context.Background()
	raised, err := svc.Raise(ctx, raiseInput(TypeBreakdown))
	require.NoError(t, err)

	resolved, err := svc.Resolve(ctx, raised.ID)

	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
	require.False(t, resolved.IsOpen())
	require.Equal(t, vehicles.StatusOK, vehiclesStub.status)
}

// Clearing one problem must not declare the vehicle healthy while another is
// still open.
func TestService_Resolve_KeepsStatusWhileAnotherAlertIsOpen(t *testing.T) {
	svc, _, vehiclesStub, _ := newService(t)
	ctx := context.Background()
	deviceIssue, err := svc.Raise(ctx, raiseInput(TypeDeviceIssue))
	require.NoError(t, err)
	_, err = svc.Raise(ctx, raiseInput(TypeBreakdown))
	require.NoError(t, err)

	_, err = svc.Resolve(ctx, deviceIssue.ID)

	require.NoError(t, err)
	require.Equal(t, vehicles.StatusBreakdown, vehiclesStub.status)
}

func TestService_Resolve_IsIdempotent(t *testing.T) {
	svc, _, _, _ := newService(t)
	ctx := context.Background()
	raised, err := svc.Raise(ctx, raiseInput(TypeBreakdown))
	require.NoError(t, err)
	first, err := svc.Resolve(ctx, raised.ID)
	require.NoError(t, err)

	second, err := svc.Resolve(ctx, raised.ID)

	require.NoError(t, err)
	require.Equal(t, first.ResolvedAt.Unix(), second.ResolvedAt.Unix())
}

func TestService_Resolve_UnknownIsNotFound(t *testing.T) {
	svc, _, _, _ := newService(t)

	_, err := svc.Resolve(context.Background(), "missing")

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_Search_OpenOnly(t *testing.T) {
	svc, _, _, _ := newService(t)
	ctx := context.Background()
	resolved, err := svc.Raise(ctx, raiseInput(TypeBreakdown))
	require.NoError(t, err)
	_, err = svc.Resolve(ctx, resolved.ID)
	require.NoError(t, err)
	_, err = svc.Raise(ctx, raiseInput(TypeOther))
	require.NoError(t, err)

	open, total, err := svc.Search(ctx, eventID, SearchFilter{OpenOnly: true}, httpx.Page{Offset: 0, Limit: 20})

	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, open, 1)
	require.Equal(t, TypeOther, open[0].Type)
}

func TestService_Search_RequiresEventID(t *testing.T) {
	svc, _, _, _ := newService(t)

	_, _, err := svc.Search(context.Background(), "", SearchFilter{}, httpx.Page{Limit: 20})

	require.ErrorIs(t, err, apperr.ErrValidation)
}

func TestNewService_NilBroadcasterIsSafe(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeVehicles{}, nil)

	_, err := svc.Raise(context.Background(), raiseInput(TypeBreakdown))

	require.NoError(t, err)
}

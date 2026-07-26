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

package vehicles

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

const eventID = "0123456789abcdef0123456789abcdef"

type fakeRepo struct {
	stored    map[string]Vehicle
	routes    map[string]string // name -> id
	codeTaken bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		stored: map[string]Vehicle{},
		routes: map[string]string{"Inland": "route-inland", "Wetlands": "route-wetlands"},
	}
}

func (f *fakeRepo) Create(ctx context.Context, v Vehicle) error {
	return f.CreateMany(ctx, []Vehicle{v})
}

func (f *fakeRepo) CreateMany(_ context.Context, list []Vehicle) error {
	if f.codeTaken {
		return ErrDuplicateCode
	}
	for _, v := range list {
		f.stored[v.ID] = v
	}
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (Vehicle, error) {
	v, ok := f.stored[id]
	if !ok {
		return Vehicle{}, ErrNotFound
	}
	return v, nil
}

func (f *fakeRepo) Update(_ context.Context, v Vehicle) error {
	if _, ok := f.stored[v.ID]; !ok {
		return ErrNotFound
	}
	f.stored[v.ID] = v
	return nil
}

func (f *fakeRepo) Search(_ context.Context, eventID string, page httpx.Page) ([]Vehicle, int, error) {
	matched := f.byEvent(eventID)
	total := len(matched)
	if page.Offset >= total {
		return nil, total, nil
	}
	return matched[page.Offset:min(page.Offset+page.Limit, total)], total, nil
}

func (f *fakeRepo) ListByEvent(_ context.Context, eventID string) ([]Vehicle, error) {
	return f.byEvent(eventID), nil
}

func (f *fakeRepo) byEvent(eventID string) []Vehicle {
	var matched []Vehicle
	for _, v := range f.stored {
		if v.EventID == eventID {
			matched = append(matched, v)
		}
	}
	slices.SortFunc(matched, func(a, b Vehicle) int { return strings.Compare(a.Code, b.Code) })
	return matched
}

func (f *fakeRepo) SetStatus(_ context.Context, vehicleID string, status Status) error {
	v, ok := f.stored[vehicleID]
	if !ok {
		return ErrNotFound
	}
	v.Status = status
	f.stored[vehicleID] = v
	return nil
}

func (f *fakeRepo) RouteNamesByID(_ context.Context, _ string) (map[string]string, error) {
	byID := map[string]string{}
	for name, id := range f.routes {
		byID[id] = name
	}
	return byID, nil
}

func (f *fakeRepo) RouteIDsByName(_ context.Context, _ string) (map[string]string, error) {
	return f.routes, nil
}

func validInput() CreateVehicleInput {
	return CreateVehicleInput{
		EventID:       eventID,
		Code:          "PKT-001",
		TeamName:      "Packet Pioneers",
		VehicleType:   "SUV",
		ContactNumber: "+94771234567",
		RouteID:       "route-inland",
		Crew: []CrewMemberInput{
			{Name: "Nimal", Role: RoleNavigator, OriginCountry: "LK"},
			{Name: "Sunil"},
		},
	}
}

func TestService_Create_AssignsIDsAndDefaults(t *testing.T) {
	got, err := NewService(newFakeRepo()).Create(context.Background(), validInput())

	require.NoError(t, err)
	require.Len(t, got.ID, 32)
	require.Equal(t, StatusOK, got.Status)
	require.Len(t, got.Crew, 2)
	require.Equal(t, RoleNavigator, got.Crew[0].Role)
	require.Equal(t, RoleNode, got.Crew[1].Role, "an unset crew role defaults to node")
	require.Equal(t, got.ID, got.Crew[0].VehicleID)
	require.Len(t, got.Crew[0].ID, 32)
}

func TestService_Create_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CreateVehicleInput)
		wantMsg string
	}{
		{"missing event", func(in *CreateVehicleInput) { in.EventID = "" }, "event id"},
		{"blank code", func(in *CreateVehicleInput) { in.Code = "  " }, "code"},
		{"blank team", func(in *CreateVehicleInput) { in.TeamName = "" }, "team name"},
		{"blank crew name", func(in *CreateVehicleInput) { in.Crew[1].Name = " " }, "crew member name"},
		{"unknown crew role", func(in *CreateVehicleInput) { in.Crew[0].Role = "driver" }, "crew role"},
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

func TestService_Create_DuplicateCodeIsConflict(t *testing.T) {
	repo := newFakeRepo()
	repo.codeTaken = true

	_, err := NewService(repo).Create(context.Background(), validInput())

	require.ErrorIs(t, err, apperr.ErrConflict)
}

func TestService_Update_ReplacesCrewWholesale(t *testing.T) {
	svc := NewService(newFakeRepo())
	created, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	newCrew := []CrewMemberInput{{Name: "Kamala", Role: RoleNavigator}}

	updated, err := svc.Update(context.Background(), created.ID, UpdateVehicleInput{Crew: &newCrew})

	require.NoError(t, err)
	require.Len(t, updated.Crew, 1)
	require.Equal(t, "Kamala", updated.Crew[0].Name)
	require.Equal(t, created.Code, updated.Code)
}

func TestService_Update_UnknownIsNotFound(t *testing.T) {
	_, err := NewService(newFakeRepo()).Update(context.Background(), "missing", UpdateVehicleInput{})

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_SetStatus(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)

	require.NoError(t, svc.SetStatus(context.Background(), created.ID, StatusBreakdown))

	got, err := repo.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, StatusBreakdown, got.Status)
}

func TestService_SetStatus_RejectsUnknownStatus(t *testing.T) {
	err := NewService(newFakeRepo()).SetStatus(context.Background(), "any", "on fire")

	require.ErrorIs(t, err, apperr.ErrValidation)
}

const importCSV = `code,team_name,vehicle_type,contact_number,route_name,crew_names
PKT-001,Packet Pioneers,SUV,+94771234567,Inland,Nimal|Sunil
PKT-002,Byte Brigade,Van,+94777654321,Wetlands,Kamala
`

func TestService_ImportCSV_CreatesVehiclesAndCrew(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	imported, err := svc.ImportCSV(ctx, eventID, strings.NewReader(importCSV))

	require.NoError(t, err)
	require.Equal(t, 2, imported)

	stored, err := repo.ListByEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	require.Equal(t, "PKT-001", stored[0].Code)
	require.Equal(t, "route-inland", stored[0].RouteID, "route names resolve to ids")
	require.Len(t, stored[0].Crew, 2)
	require.Equal(t, RoleNavigator, stored[0].Crew[0].Role, "the first crew member holds the phone")
	require.Equal(t, RoleNode, stored[0].Crew[1].Role)
	require.Equal(t, "route-wetlands", stored[1].RouteID)
}

func TestService_ImportCSV_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"empty file", "", "empty"},
		{"header only", "code,team_name,vehicle_type,contact_number,route_name,crew_names\n", "no vehicles"},
		{
			"wrong columns",
			"vehicle,team,type,phone,route,crew\nPKT-001,T,SUV,1,Inland,A\n",
			"column 1",
		},
		{
			"missing code",
			"code,team_name,vehicle_type,contact_number,route_name,crew_names\n,Team,SUV,1,Inland,A\n",
			"no vehicle code",
		},
		{
			"missing team name",
			"code,team_name,vehicle_type,contact_number,route_name,crew_names\nPKT-001,,SUV,1,Inland,A\n",
			"no team name",
		},
		{
			"unknown route",
			"code,team_name,vehicle_type,contact_number,route_name,crew_names\nPKT-001,Team,SUV,1,Highlands,A\n",
			"does not exist",
		},
		{
			"duplicate code within the file",
			"code,team_name,vehicle_type,contact_number,route_name,crew_names\n" +
				"PKT-001,Team,SUV,1,Inland,A\nPKT-001,Other,Van,2,Inland,B\n",
			"more than once",
		},
		{
			"ragged row",
			"code,team_name,vehicle_type,contact_number,route_name,crew_names\nPKT-001,Team,SUV\n",
			"line 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(newFakeRepo()).ImportCSV(context.Background(), eventID, strings.NewReader(tt.body))

			require.ErrorIs(t, err, apperr.ErrValidation)
			require.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// A rejected file must leave the event untouched, not half-provisioned.
func TestService_ImportCSV_IsAllOrNothing(t *testing.T) {
	repo := newFakeRepo()
	body := "code,team_name,vehicle_type,contact_number,route_name,crew_names\n" +
		"PKT-001,Team One,SUV,1,Inland,A\nPKT-002,Team Two,Van,2,Highlands,B\n"

	_, err := NewService(repo).ImportCSV(context.Background(), eventID, strings.NewReader(body))

	require.Error(t, err)
	require.Empty(t, repo.stored)
}

func TestService_ImportCSV_AcceptsMissingRouteAndCrew(t *testing.T) {
	repo := newFakeRepo()
	body := "code,team_name,vehicle_type,contact_number,route_name,crew_names\nPKT-009,Solo,Sedan,,,\n"

	imported, err := NewService(repo).ImportCSV(context.Background(), eventID, strings.NewReader(body))

	require.NoError(t, err)
	require.Equal(t, 1, imported)
	stored, err := repo.ListByEvent(context.Background(), eventID)
	require.NoError(t, err)
	require.Empty(t, stored[0].RouteID)
	require.Empty(t, stored[0].Crew)
}

func TestService_ImportCSV_ToleratesExcelBOM(t *testing.T) {
	repo := newFakeRepo()

	imported, err := NewService(repo).ImportCSV(context.Background(), eventID, strings.NewReader(utf8BOM+importCSV))

	require.NoError(t, err)
	require.Equal(t, 2, imported)
}

func TestService_ExportCSV_RoundTripsAnImport(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	_, err := svc.ImportCSV(ctx, eventID, strings.NewReader(importCSV))
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, svc.ExportCSV(ctx, eventID, &out))

	require.Equal(t, importCSV, out.String())
}

func TestService_ExportCSV_EmptyEventStillWritesHeader(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, NewService(newFakeRepo()).ExportCSV(context.Background(), eventID, &out))

	require.Equal(t, "code,team_name,vehicle_type,contact_number,route_name,crew_names\n", out.String())
}

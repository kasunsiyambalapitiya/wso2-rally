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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/alerts"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

const (
	testVehicleID = "0123456789abcdef0123456789abcdef"
	testEventID   = "ffffffffffffffffffffffffffffffff"
	testRouteID   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	crewA         = "crew-a"
	crewB         = "crew-b"
)

// fakeRepo is an in-memory Repo. It enforces the one-live-session rule the
// real schema enforces with a unique index.
type fakeRepo struct {
	sessions   map[string]Session
	vouchers   map[string]Voucher
	event      EventInfo
	waypoints  []WaypointGeo
	routeID    string
	crew       []string
	taskStates []TaskState
	noVehicle  bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		sessions: map[string]Session{},
		vouchers: map[string]Voucher{},
		event: EventInfo{
			Status:    "active",
			Cipher:    "API Integration",
			StartTime: "09:00",
			Start:     GeoCircle{Lat: 6.8901, Lng: 79.9200, RadiusM: 40, Placed: true},
			Finish:    GeoCircle{Lat: 6.8480, Lng: 79.9280, RadiusM: 30, Placed: true},
		},
		routeID: testRouteID,
		crew:    []string{crewA, crewB},
		waypoints: []WaypointGeo{
			waypointAt("wp-1", 0, atKandy, WaypointTask{ID: "task-1", Type: tasks.TypeInputSelect}),
			waypointAt("wp-2", 1, LatLng{Lat: 6.8700, Lng: 79.9240},
				WaypointTask{ID: "task-2", Type: tasks.TypeRestLock}),
		},
	}
}

func (f *fakeRepo) BindTargetOf(_ context.Context, vehicleID string) (BindTarget, error) {
	if f.noVehicle {
		return BindTarget{}, ErrVehicleNotFound
	}
	return BindTarget{
		EventID: testEventID, RouteID: f.routeID,
		Code: "PKT-001", TeamName: "Packet Pioneers", CrewMemberID: f.crew,
	}, nil
}

func (f *fakeRepo) CreateSession(_ context.Context, s Session) error {
	for _, existing := range f.sessions {
		if existing.VehicleID == s.VehicleID && existing.Status.IsLive() {
			return ErrAlreadyBound
		}
	}
	f.sessions[s.ID] = s
	return nil
}

func (f *fakeRepo) GetSession(_ context.Context, id string) (Session, error) {
	s, ok := f.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return s, nil
}

func (f *fakeRepo) UpdateSession(_ context.Context, s Session) error {
	if _, ok := f.sessions[s.ID]; !ok {
		return ErrNotFound
	}
	f.sessions[s.ID] = s
	return nil
}

func (f *fakeRepo) EventInfoOf(context.Context, string) (EventInfo, error) { return f.event, nil }

func (f *fakeRepo) WaypointsOf(context.Context, string) ([]WaypointGeo, error) {
	return f.waypoints, nil
}

func (f *fakeRepo) RouteIDOfVehicle(context.Context, string) (string, error) { return f.routeID, nil }

func (f *fakeRepo) TaskStatesOf(context.Context, string, string) ([]TaskState, error) {
	return f.taskStates, nil
}

func (f *fakeRepo) CreateVoucher(_ context.Context, v Voucher) error {
	f.vouchers[v.SessionID] = v
	return nil
}

func (f *fakeRepo) VoucherOf(_ context.Context, sessionID string) (Voucher, error) {
	v, ok := f.vouchers[sessionID]
	if !ok {
		return Voucher{}, ErrNoVoucher
	}
	return v, nil
}

func (f *fakeRepo) CrewSizeOf(context.Context, string) (int, error) { return len(f.crew), nil }

func (f *fakeRepo) VehicleCodeOf(context.Context, string) (string, error) { return "PKT-001", nil }

// stubMinter issues a predictable token so tests can assert it reached the
// caller without decoding a JWT.
type stubMinter struct{ err error }

func (s stubMinter) Mint(sessionID, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return "token-for-" + sessionID, nil
}

// recordingAlerts captures crew reports instead of persisting them.
type recordingAlerts struct {
	raised []alerts.RaiseAlertInput
	err    error
}

func (r *recordingAlerts) Raise(_ context.Context, in alerts.RaiseAlertInput) (alerts.Alert, error) {
	if r.err != nil {
		return alerts.Alert{}, r.err
	}
	r.raised = append(r.raised, in)
	return alerts.Alert{ID: "alert-1", VehicleID: in.VehicleID, Type: in.Type, Source: in.Source}, nil
}

type broadcastRecord struct {
	topic   string
	message any
}

func newService(t *testing.T) (*Service, *fakeRepo, *recordingAlerts, *[]broadcastRecord) {
	t.Helper()

	repo, alertRaiser := newFakeRepo(), &recordingAlerts{}
	var sent []broadcastRecord
	svc := NewService(repo, stubMinter{}, alertRaiser, func(topic string, message any) {
		sent = append(sent, broadcastRecord{topic: topic, message: message})
	})

	return svc, repo, alertRaiser, &sent
}

func bindOnce(t *testing.T, svc *Service) Session {
	t.Helper()

	session, token, err := svc.Bind(context.Background(), BindInput{
		VehicleID: testVehicleID, CrewMemberIDs: []string{crewA},
	})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	return session
}

func TestService_Bind_IssuesTokenAndBoundSession(t *testing.T) {
	svc, _, _, _ := newService(t)

	session, token, err := svc.Bind(context.Background(), BindInput{
		VehicleID: testVehicleID, CrewMemberIDs: []string{crewA, crewB},
	})

	require.NoError(t, err)
	require.Len(t, session.ID, 32)
	require.Equal(t, StatusBound, session.Status)
	require.Equal(t, testEventID, session.EventID)
	require.NotNil(t, session.BoundAt)
	require.Equal(t, "token-for-"+session.ID, token)
}

// The one-active-phone rule: a second device must be turned away.
func TestService_Bind_SecondPhoneIsRejected(t *testing.T) {
	svc, _, _, _ := newService(t)
	bindOnce(t, svc)

	_, _, err := svc.Bind(context.Background(), BindInput{
		VehicleID: testVehicleID, CrewMemberIDs: []string{crewA},
	})

	require.ErrorIs(t, err, ErrAlreadyBound)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

func TestService_Bind_UnknownVehicle(t *testing.T) {
	svc, repo, _, _ := newService(t)
	repo.noVehicle = true

	_, _, err := svc.Bind(context.Background(), BindInput{
		VehicleID: testVehicleID, CrewMemberIDs: []string{crewA},
	})

	require.ErrorIs(t, err, ErrVehicleNotFound)
}

func TestService_Bind_RejectsUnpublishedEvent(t *testing.T) {
	svc, repo, _, _ := newService(t)
	repo.event.Status = "setup"

	_, _, err := svc.Bind(context.Background(), BindInput{
		VehicleID: testVehicleID, CrewMemberIDs: []string{crewA},
	})

	require.ErrorIs(t, err, ErrEventNotActive)
}

func TestService_Bind_CrewValidation(t *testing.T) {
	tests := []struct {
		name string
		crew []string
	}{
		{"no crew selected", nil},
		{"crew member from another vehicle", []string{"someone-else"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, _ := newService(t)

			_, _, err := svc.Bind(context.Background(), BindInput{VehicleID: testVehicleID, CrewMemberIDs: tt.crew})

			require.ErrorIs(t, err, apperr.ErrValidation)
		})
	}
}

func TestService_Bind_RequiresVehicleID(t *testing.T) {
	svc, _, _, _ := newService(t)

	_, _, err := svc.Bind(context.Background(), BindInput{CrewMemberIDs: []string{crewA}})

	require.ErrorIs(t, err, apperr.ErrValidation)
}

func TestService_State_RevealsCipherOnlyWhenActive(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)

	active, err := svc.State(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, "API Integration", active.Cipher)

	repo.event.Status = "setup"
	setup, err := svc.State(context.Background(), session.ID)
	require.NoError(t, err)
	require.Empty(t, setup.Cipher, "the cipher stays off the wire until the event is live")
}

func TestService_State_ReportsTheNextWaypoint(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)

	state, err := svc.State(context.Background(), session.ID)

	require.NoError(t, err)
	require.Equal(t, "wp-1", state.NextWaypointID)
	require.Equal(t, "PKT-001", state.VehicleCode)
	require.Len(t, state.Waypoints, 2)
}

func TestService_State_UnknownSession(t *testing.T) {
	svc, _, _, _ := newService(t)

	_, err := svc.State(context.Background(), "missing")

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_Ping_ActivatesAndUnlocks(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)

	result, err := svc.Ping(context.Background(), session.ID, atKandy, 5)

	require.NoError(t, err)
	require.Equal(t, []string{"task-1"}, result.UnlockedTaskIDs)
	require.Equal(t, "wp-1", result.CurrentWaypointID)

	stored := repo.sessions[session.ID]
	require.Equal(t, StatusActive, stored.Status, "the first ping starts the run")
	require.NotNil(t, stored.StartedAt)
	require.NotNil(t, stored.LastLat)
	require.InDelta(t, atKandy.Lat, *stored.LastLat, 1e-9)
}

func TestService_Ping_BroadcastsPositionToOrganizers(t *testing.T) {
	svc, _, _, sent := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.Ping(context.Background(), session.ID, atKandy, 5)

	require.NoError(t, err)
	require.Contains(t, topicsOf(*sent), EventTopic(testEventID))
}

func TestService_Ping_BroadcastsRestLockToTheCrew(t *testing.T) {
	svc, _, _, sent := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.Ping(context.Background(), session.ID, LatLng{Lat: 6.8700, Lng: 79.9240}, 5)

	require.NoError(t, err)
	require.Contains(t, topicsOf(*sent), SessionTopic(session.ID))
}

func TestService_Ping_ArrivalFinishesAndIssuesVoucher(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)

	result, err := svc.Ping(context.Background(), session.ID, LatLng{Lat: 6.8480, Lng: 79.9280}, 5)

	require.NoError(t, err)
	require.True(t, result.Arrived)

	stored := repo.sessions[session.ID]
	require.Equal(t, StatusFinished, stored.Status)
	require.NotNil(t, stored.FinishedAt)

	voucher, err := svc.Vouchers(context.Background(), session.ID)
	require.NoError(t, err)
	require.NotEmpty(t, voucher.EntryCode)
	require.Equal(t, len(repo.crew), voucher.LunchPasses)
}

// Once a crew has finished, a stray ping must not reopen their run.
func TestService_Ping_AfterFinishIsRejected(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)
	_, err := svc.Ping(context.Background(), session.ID, LatLng{Lat: 6.8480, Lng: 79.9280}, 5)
	require.NoError(t, err)

	_, err = svc.Ping(context.Background(), session.ID, atKandy, 5)

	require.ErrorIs(t, err, ErrSessionFinished)
}

func TestService_Ping_RejectsImpossibleCoordinates(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.Ping(context.Background(), session.ID, LatLng{Lat: 95, Lng: 0}, 5)

	require.ErrorIs(t, err, apperr.ErrValidation)
}

func TestService_Ping_OutsideEveryBoundaryStillRecordsPosition(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)

	result, err := svc.Ping(context.Background(), session.ID, nowhere, 5)

	require.NoError(t, err)
	require.Empty(t, result.UnlockedTaskIDs)
	require.NotNil(t, repo.sessions[session.ID].LastPingAt)
}

func TestService_Finish_LocksTheSession(t *testing.T) {
	svc, repo, _, _ := newService(t)
	session := bindOnce(t, svc)

	finished, err := svc.Finish(context.Background(), session.ID)

	require.NoError(t, err)
	require.Equal(t, StatusFinished, finished.Status)
	require.Equal(t, StatusFinished, repo.sessions[session.ID].Status)
}

func TestService_Finish_IsIdempotent(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)
	first, err := svc.Finish(context.Background(), session.ID)
	require.NoError(t, err)

	second, err := svc.Finish(context.Background(), session.ID)

	require.NoError(t, err)
	require.Equal(t, first.FinishedAt.Unix(), second.FinishedAt.Unix())
}

func TestService_Vouchers_BeforeFinishing(t *testing.T) {
	svc, _, _, _ := newService(t)
	session := bindOnce(t, svc)

	_, err := svc.Vouchers(context.Background(), session.ID)

	require.ErrorIs(t, err, ErrNoVoucher)
	require.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestService_RaiseCrewAlert_TagsTheSource(t *testing.T) {
	svc, _, alertRaiser, _ := newService(t)
	session := bindOnce(t, svc)
	lat, lng := 6.89, 79.92

	_, err := svc.RaiseCrewAlert(context.Background(), session.ID, CrewAlertInput{
		Type: "breakdown", Note: "Flat tyre", Lat: &lat, Lng: &lng,
	})

	require.NoError(t, err)
	require.Len(t, alertRaiser.raised, 1)
	require.Equal(t, alerts.SourceCrew, alertRaiser.raised[0].Source)
	require.Equal(t, testVehicleID, alertRaiser.raised[0].VehicleID)
	require.Equal(t, session.ID, alertRaiser.raised[0].RaisedBy)
}

func TestService_RaiseCrewAlert_UnknownSession(t *testing.T) {
	svc, _, _, _ := newService(t)

	_, err := svc.RaiseCrewAlert(context.Background(), "missing", CrewAlertInput{Type: "other"})

	require.ErrorIs(t, err, ErrNotFound)
}

func TestNextWaypointID(t *testing.T) {
	waypoints := []WaypointGeo{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	at := func(id string) *string { return &id }

	require.Equal(t, "a", nextWaypointID(waypoints, nil))
	require.Equal(t, "b", nextWaypointID(waypoints, at("a")))
	require.Empty(t, nextWaypointID(waypoints, at("c")), "there is nothing after the last waypoint")
	require.Empty(t, nextWaypointID(nil, nil))
}

func topicsOf(records []broadcastRecord) []string {
	topics := make([]string, 0, len(records))
	for _, record := range records {
		topics = append(topics, record.topic)
	}

	return topics
}

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
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/alerts"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
)

// lunchPassesPerCrew is one pass per person aboard.
const lunchPassesPerCrew = 1

// entryCodeLength is how much of a fresh id becomes the printed entry code.
const entryCodeLength = 6

// Repo is the persistence contract for the in-car runtime.
type Repo interface {
	// BindTargetOf returns what is needed to bind a vehicle, or
	// ErrVehicleNotFound.
	BindTargetOf(ctx context.Context, vehicleID string) (BindTarget, error)
	// CreateSession inserts a session, returning ErrAlreadyBound when the
	// vehicle already has a live one.
	CreateSession(ctx context.Context, s Session) error
	GetSession(ctx context.Context, id string) (Session, error)
	UpdateSession(ctx context.Context, s Session) error
	EventInfoOf(ctx context.Context, eventID string) (EventInfo, error)
	// WaypointsOf returns a route's waypoints reduced to geofence data.
	WaypointsOf(ctx context.Context, routeID string) ([]WaypointGeo, error)
	// RouteIDOfVehicle returns the course a vehicle is assigned to, or "".
	RouteIDOfVehicle(ctx context.Context, vehicleID string) (string, error)
	// TaskStatesOf lists a session's tasks with their submission status.
	TaskStatesOf(ctx context.Context, sessionID, routeID string) ([]TaskState, error)
	CreateVoucher(ctx context.Context, v Voucher) error
	VoucherOf(ctx context.Context, sessionID string) (Voucher, error)
	// CrewSizeOf counts the people aboard, which sets the lunch passes.
	CrewSizeOf(ctx context.Context, vehicleID string) (int, error)
	// VehicleCodeOf is used to label live-monitor broadcasts.
	VehicleCodeOf(ctx context.Context, vehicleID string) (string, error)
	// SubmittableTaskOf loads the definition needed to score an attempt.
	SubmittableTaskOf(ctx context.Context, taskID string) (SubmittableTask, error)
	// SaveSubmission stores an attempt and returns the session's recomputed
	// total, so a resubmission corrects the score instead of adding to it.
	SaveSubmission(ctx context.Context, sub Submission) (int, error)
}

// AlertRaiser is the slice of the alerts service this package needs, so a crew
// report lands in the same place an organizer's does.
type AlertRaiser interface {
	Raise(ctx context.Context, in alerts.RaiseAlertInput) (alerts.Alert, error)
}

// Broadcaster publishes to a topic. Two topics matter here: the event topic
// organizers watch, and the per-session topic the in-car phone subscribes to.
type Broadcaster func(topic string, message any)

// EventTopic is the channel organizers subscribe to for one event.
func EventTopic(eventID string) string { return "event:" + eventID }

// SessionTopic is the channel one in-car phone subscribes to.
func SessionTopic(sessionID string) string { return "session:" + sessionID }

// TokenMinter issues the credential a bound phone carries for the rest of the
// rally.
type TokenMinter interface {
	Mint(sessionID, vehicleID string) (string, error)
}

// HMACTokenMinter mints team tokens with the configured shared secret.
type HMACTokenMinter struct {
	Secret string
	TTL    time.Duration
}

// Mint implements TokenMinter.
func (m HMACTokenMinter) Mint(sessionID, vehicleID string) (string, error) {
	return authz.MintTeamToken(m.Secret, sessionID, vehicleID, m.TTL)
}

// Service holds the in-car runtime rules.
type Service struct {
	repo      Repo
	minter    TokenMinter
	alerts    AlertRaiser
	broadcast Broadcaster
}

// NewService wires a Service. A nil broadcaster becomes a no-op so the service
// is usable before the realtime hub exists.
func NewService(repo Repo, minter TokenMinter, alertRaiser AlertRaiser, broadcast Broadcaster) *Service {
	if broadcast == nil {
		broadcast = func(string, any) {}
	}

	return &Service{repo: repo, minter: minter, alerts: alertRaiser, broadcast: broadcast}
}

// SetBroadcaster attaches the realtime hub after construction.
func (s *Service) SetBroadcaster(broadcast Broadcaster) {
	if broadcast != nil {
		s.broadcast = broadcast
	}
}

// Bind pairs a phone with a vehicle and returns the team token it will carry.
//
// This is the zero-facilitator start: no one hands out credentials, the crew
// simply picks their vehicle. The one-active-phone rule is enforced by a
// unique index, so two devices racing to bind the same vehicle cannot both win.
func (s *Service) Bind(ctx context.Context, in BindInput) (Session, string, error) {
	if in.VehicleID == "" {
		return Session{}, "", apperr.Validationf("vehicle id is required")
	}

	target, err := s.repo.BindTargetOf(ctx, in.VehicleID)
	if err != nil {
		return Session{}, "", err
	}

	if err := validateCrewSelection(in.CrewMemberIDs, target.CrewMemberID); err != nil {
		return Session{}, "", err
	}

	event, err := s.repo.EventInfoOf(ctx, target.EventID)
	if err != nil {
		return Session{}, "", err
	}
	if !event.IsActive() {
		return Session{}, "", ErrEventNotActive
	}

	now := time.Now().UTC()
	session := Session{
		ID:        store.NewID(),
		EventID:   target.EventID,
		VehicleID: in.VehicleID,
		Status:    StatusBound,
		BoundAt:   &now,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return Session{}, "", err
	}

	token, err := s.minter.Mint(session.ID, session.VehicleID)
	if err != nil {
		return Session{}, "", fmt.Errorf("mint team token for session %s: %w", session.ID, err)
	}

	return session, token, nil
}

// State returns everything the micro app needs to decide which screen to show.
func (s *Service) State(ctx context.Context, sessionID string) (SessionState, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return SessionState{}, err
	}

	event, err := s.repo.EventInfoOf(ctx, session.EventID)
	if err != nil {
		return SessionState{}, err
	}
	target, err := s.repo.BindTargetOf(ctx, session.VehicleID)
	if err != nil {
		return SessionState{}, err
	}

	waypoints, err := s.waypointsFor(ctx, session.VehicleID)
	if err != nil {
		return SessionState{}, err
	}

	state := SessionState{
		Session:      session,
		VehicleCode:  target.Code,
		TeamName:     target.TeamName,
		EventStatus:  event.Status,
		StartTime:    event.StartTime,
		StartCircle:  event.Start,
		FinishCircle: event.Finish,
		Waypoints:    waypoints,
	}
	// The cipher is part of the start signal; withholding it until the event
	// is active keeps it off the wire during setup.
	if event.IsActive() {
		state.Cipher = event.Cipher
	}
	state.NextWaypointID = nextWaypointID(waypoints, session.CurrentWaypointID)

	return state, nil
}

// Ping records a reported position and answers with what the crew may now do.
//
// The client never decides whether it is inside a boundary: it reports where
// it is, and this method runs the geofence maths server-side.
func (s *Service) Ping(ctx context.Context, sessionID string, position LatLng, accuracyM float64) (PingResult, error) {
	if err := validatePosition(position); err != nil {
		return PingResult{}, err
	}

	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return PingResult{}, err
	}
	if session.Status == StatusFinished {
		return PingResult{}, ErrSessionFinished
	}

	event, err := s.repo.EventInfoOf(ctx, session.EventID)
	if err != nil {
		return PingResult{}, err
	}
	waypoints, err := s.waypointsFor(ctx, session.VehicleID)
	if err != nil {
		return PingResult{}, err
	}

	result := EvaluatePing(position, waypoints, event.Finish)

	now := time.Now().UTC()
	session.LastLat, session.LastLng, session.LastPingAt = &position.Lat, &position.Lng, &now
	// The first ping past the start grid is what makes a bound crew active.
	if session.Status == StatusBound {
		session.Status = StatusActive
		session.StartedAt = &now
	}
	if result.CurrentWaypointID != "" {
		waypointID := result.CurrentWaypointID
		session.CurrentWaypointID = &waypointID
	}

	if result.Arrived {
		if err := s.finish(ctx, &session, now); err != nil {
			return PingResult{}, err
		}
	} else if err := s.repo.UpdateSession(ctx, session); err != nil {
		return PingResult{}, fmt.Errorf("update session %s: %w", sessionID, err)
	}

	s.publishPosition(ctx, session, position)
	s.broadcastSessionEvents(session.ID, result)

	return result, nil
}

// ListTasks returns the crew's task list with each task's submission status.
func (s *Service) ListTasks(ctx context.Context, sessionID string) ([]TaskState, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	routeID, err := s.repo.RouteIDOfVehicle(ctx, session.VehicleID)
	if err != nil {
		return nil, err
	}

	states, err := s.repo.TaskStatesOf(ctx, sessionID, routeID)
	if err != nil {
		return nil, fmt.Errorf("list tasks of session %s: %w", sessionID, err)
	}

	return states, nil
}

// RaiseCrewAlert files a problem reported from the car, tagged as crew-sourced
// so organizers can tell it apart from one they filed themselves.
func (s *Service) RaiseCrewAlert(ctx context.Context, sessionID string, in CrewAlertInput) (alerts.Alert, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return alerts.Alert{}, err
	}

	raised, err := s.alerts.Raise(ctx, alerts.RaiseAlertInput{
		VehicleID: session.VehicleID,
		Type:      alerts.Type(in.Type),
		Note:      in.Note,
		Source:    alerts.SourceCrew,
		RaisedBy:  sessionID,
		Lat:       in.Lat,
		Lng:       in.Lng,
	})
	if err != nil {
		return alerts.Alert{}, err
	}

	return raised, nil
}

// Finish ends a run explicitly. Arrival at the finish geofence does the same
// thing automatically, so this is the manual fallback.
func (s *Service) Finish(ctx context.Context, sessionID string) (Session, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if session.Status == StatusFinished {
		return session, nil // Finishing twice is a no-op, not an error.
	}

	if err := s.finish(ctx, &session, time.Now().UTC()); err != nil {
		return Session{}, err
	}

	return session, nil
}

// Vouchers returns what the crew collects at Pearl Bay.
func (s *Service) Vouchers(ctx context.Context, sessionID string) (Voucher, error) {
	return s.repo.VoucherOf(ctx, sessionID)
}

// finish locks the score, issues the voucher, and tells both topics.
func (s *Service) finish(ctx context.Context, session *Session, at time.Time) error {
	session.Status = StatusFinished
	session.FinishedAt = &at

	if err := s.repo.UpdateSession(ctx, *session); err != nil {
		return fmt.Errorf("finish session %s: %w", session.ID, err)
	}

	if err := s.issueVoucher(ctx, *session); err != nil {
		return err
	}

	s.broadcast(SessionTopic(session.ID), map[string]any{"type": string(EventArrival)})

	return nil
}

// issueVoucher creates the crew's finish-line voucher, unless one already
// exists from an earlier finish.
func (s *Service) issueVoucher(ctx context.Context, session Session) error {
	if _, err := s.repo.VoucherOf(ctx, session.ID); err == nil {
		return nil
	}

	crewSize, err := s.repo.CrewSizeOf(ctx, session.VehicleID)
	if err != nil {
		return fmt.Errorf("count crew of vehicle %s: %w", session.VehicleID, err)
	}
	code := store.NewID()

	voucher := Voucher{
		ID:          code,
		SessionID:   session.ID,
		EntryCode:   strings.ToUpper(code[:entryCodeLength]),
		LockerID:    strings.ToUpper(session.VehicleID[:entryCodeLength]),
		LunchPasses: crewSize * lunchPassesPerCrew,
	}
	if err := s.repo.CreateVoucher(ctx, voucher); err != nil {
		return fmt.Errorf("issue voucher for session %s: %w", session.ID, err)
	}

	return nil
}

// waypointsFor loads the geofence data of the course a vehicle is running. A
// vehicle with no route assigned simply has no waypoints.
func (s *Service) waypointsFor(ctx context.Context, vehicleID string) ([]WaypointGeo, error) {
	routeID, err := s.repo.RouteIDOfVehicle(ctx, vehicleID)
	if err != nil {
		return nil, err
	}
	if routeID == "" {
		return nil, nil
	}

	waypoints, err := s.repo.WaypointsOf(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("load waypoints of route %s: %w", routeID, err)
	}

	return waypoints, nil
}

// publishPosition pushes the vehicle's position to the organizer's monitor.
// A failure here costs a map marker, not the crew's ping, so it is not fatal.
func (s *Service) publishPosition(ctx context.Context, session Session, position LatLng) {
	code, err := s.repo.VehicleCodeOf(ctx, session.VehicleID)
	if err != nil {
		return
	}

	s.broadcast(EventTopic(session.EventID), map[string]any{
		"type":        "vehicle_position",
		"vehicleCode": code,
		"lat":         position.Lat,
		"lng":         position.Lng,
	})
}

// broadcastSessionEvents mirrors the rest-lock and trivia events onto the
// session topic, so the phone switches screens even if its own HTTP response
// is delayed.
func (s *Service) broadcastSessionEvents(sessionID string, result PingResult) {
	for _, event := range result.Events {
		switch event.Type {
		case EventRestLock, EventTrivia:
			s.broadcast(SessionTopic(sessionID), map[string]any{
				"type":   string(event.Type),
				"taskId": event.TaskID,
			})
		}
	}
}

// nextWaypointID is the first waypoint after the one the crew is at, or the
// very first when they have not reached any.
func nextWaypointID(waypoints []WaypointGeo, currentID *string) string {
	if len(waypoints) == 0 {
		return ""
	}
	if currentID == nil {
		return waypoints[0].ID
	}

	for i, waypoint := range waypoints {
		if waypoint.ID == *currentID && i+1 < len(waypoints) {
			return waypoints[i+1].ID
		}
	}

	return ""
}

func validateCrewSelection(selected, roster []string) error {
	if len(selected) == 0 {
		return apperr.Validationf("select at least one crew member before starting")
	}

	for _, id := range selected {
		if !slices.Contains(roster, id) {
			return apperr.Validationf("crew member %s is not part of this vehicle", id)
		}
	}

	return nil
}

func validatePosition(p LatLng) error {
	if p.Lat < -90 || p.Lat > 90 {
		return apperr.Validationf("latitude must be between -90 and 90")
	}
	if p.Lng < -180 || p.Lng > 180 {
		return apperr.Validationf("longitude must be between -180 and 180")
	}

	return nil
}

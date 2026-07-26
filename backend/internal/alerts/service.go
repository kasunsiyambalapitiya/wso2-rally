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
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/vehicles"
)

// Repo is the persistence contract for vehicle alerts.
type Repo interface {
	Create(ctx context.Context, a Alert) error
	Get(ctx context.Context, id string) (Alert, error)
	Resolve(ctx context.Context, id string, at time.Time) error
	Search(ctx context.Context, eventID string, filter SearchFilter, page httpx.Page) ([]Alert, int, error)
	// OpenTypesOf lists the types of a vehicle's still-unresolved alerts.
	OpenTypesOf(ctx context.Context, vehicleID string) ([]Type, error)
	// EventIDOf resolves the event a vehicle belongs to, for broadcasting.
	EventIDOf(ctx context.Context, vehicleID string) (string, error)
}

// VehicleStatusSetter is the slice of the vehicles service this package needs.
// Depending on the behaviour rather than the whole service keeps the coupling
// one-directional and the tests trivial.
type VehicleStatusSetter interface {
	SetStatus(ctx context.Context, vehicleID string, status vehicles.Status) error
}

// Broadcaster publishes an alert to the organizer's live monitor. It is a
// function so this package need not depend on the realtime hub; the wiring
// layer supplies one, and a nil broadcaster becomes a no-op.
type Broadcaster func(eventID string, message any)

// Service holds the alert rules.
type Service struct {
	repo      Repo
	vehicles  VehicleStatusSetter
	broadcast Broadcaster
}

// NewService wires a Service. A nil broadcaster is replaced by a no-op, so
// callers that do not care about live updates need not supply one.
func NewService(repo Repo, vehicleStatus VehicleStatusSetter, broadcast Broadcaster) *Service {
	if broadcast == nil {
		broadcast = func(string, any) {}
	}

	return &Service{repo: repo, vehicles: vehicleStatus, broadcast: broadcast}
}

// Raise files a vehicle problem, moves the vehicle's status to match, and
// pushes the alert to the organizer's live monitor.
func (s *Service) Raise(ctx context.Context, in RaiseAlertInput) (Alert, error) {
	if in.VehicleID == "" {
		return Alert{}, apperr.Validationf("vehicle id is required")
	}
	if !in.Type.IsValid() {
		return Alert{}, apperr.Validationf("unknown alert type %q", in.Type)
	}
	if in.Source == "" {
		in.Source = SourceOrganizer
	}
	if !in.Source.IsValid() {
		return Alert{}, apperr.Validationf("unknown alert source %q", in.Source)
	}
	if err := validateCoordinates(in.Lat, in.Lng); err != nil {
		return Alert{}, err
	}

	alert := Alert{
		ID:        store.NewID(),
		VehicleID: in.VehicleID,
		Type:      in.Type,
		Note:      strings.TrimSpace(in.Note),
		Source:    in.Source,
		RaisedBy:  strings.TrimSpace(in.RaisedBy),
		Lat:       in.Lat,
		Lng:       in.Lng,
		RaisedAt:  time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, alert); err != nil {
		return Alert{}, fmt.Errorf("create alert: %w", err)
	}

	// A breakdown or device issue is also a change of vehicle state; "other"
	// is informational and leaves the vehicle running.
	if status, changes := vehicleStatusFor(alert.Type); changes {
		if err := s.vehicles.SetStatus(ctx, alert.VehicleID, status); err != nil {
			return Alert{}, fmt.Errorf("set vehicle status from alert: %w", err)
		}
	}

	s.publish(ctx, alert)

	return alert, nil
}

// Resolve closes an alert. When it was the vehicle's last blocking problem the
// vehicle returns to ok, so a fixed car stops showing as broken down.
func (s *Service) Resolve(ctx context.Context, id string) (Alert, error) {
	alert, err := s.repo.Get(ctx, id)
	if err != nil {
		return Alert{}, err
	}
	if !alert.IsOpen() {
		return alert, nil // Resolving twice is a no-op, not an error.
	}

	resolvedAt := time.Now().UTC()
	if err := s.repo.Resolve(ctx, id, resolvedAt); err != nil {
		return Alert{}, fmt.Errorf("resolve alert %s: %w", id, err)
	}
	alert.ResolvedAt = &resolvedAt

	if err := s.restoreVehicleStatus(ctx, alert.VehicleID); err != nil {
		return Alert{}, err
	}

	s.publish(ctx, alert)

	return alert, nil
}

// Search returns a page of an event's alerts plus the unpaged total.
func (s *Service) Search(ctx context.Context, eventID string, filter SearchFilter, page httpx.Page) ([]Alert, int, error) {
	if eventID == "" {
		return nil, 0, apperr.Validationf("event id is required")
	}

	found, total, err := s.repo.Search(ctx, eventID, filter, page)
	if err != nil {
		return nil, 0, fmt.Errorf("search alerts of event %s: %w", eventID, err)
	}

	return found, total, nil
}

// restoreVehicleStatus recomputes a vehicle's status from whatever alerts are
// still open against it.
func (s *Service) restoreVehicleStatus(ctx context.Context, vehicleID string) error {
	open, err := s.repo.OpenTypesOf(ctx, vehicleID)
	if err != nil {
		return fmt.Errorf("list open alerts of vehicle %s: %w", vehicleID, err)
	}

	status := vehicles.StatusOK
	for _, alertType := range open {
		// A breakdown outranks a device issue: it is the more serious state.
		if candidate, changes := vehicleStatusFor(alertType); changes {
			status = candidate
			if candidate == vehicles.StatusBreakdown {
				break
			}
		}
	}

	if err := s.vehicles.SetStatus(ctx, vehicleID, status); err != nil {
		return fmt.Errorf("restore status of vehicle %s: %w", vehicleID, err)
	}

	return nil
}

// publish pushes the alert to the event topic. A missing event id only costs a
// live update, so it is not worth failing the write the organizer just made.
func (s *Service) publish(ctx context.Context, alert Alert) {
	eventID, err := s.repo.EventIDOf(ctx, alert.VehicleID)
	if err != nil || eventID == "" {
		return
	}
	s.broadcast(eventID, alert)
}

// vehicleStatusFor maps an alert type onto the vehicle status it implies. The
// boolean is false for types that leave the vehicle's state alone.
func vehicleStatusFor(alertType Type) (vehicles.Status, bool) {
	switch alertType {
	case TypeBreakdown:
		return vehicles.StatusBreakdown, true
	case TypeDeviceIssue:
		return vehicles.StatusDeviceIssue, true
	default:
		return vehicles.StatusOK, false
	}
}

func validateCoordinates(lat, lng *float64) error {
	if (lat == nil) != (lng == nil) {
		return apperr.Validationf("a location needs both a latitude and a longitude")
	}
	if lat == nil {
		return nil
	}
	if *lat < -90 || *lat > 90 {
		return apperr.Validationf("latitude must be between -90 and 90")
	}
	if *lng < -180 || *lng > 180 {
		return apperr.Validationf("longitude must be between -180 and 180")
	}

	return nil
}

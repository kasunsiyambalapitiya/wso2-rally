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
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/tasks"
)

const sessionColumns = `
	id, event_id, vehicle_id, status, current_waypoint_id, total_score,
	bound_at, started_at, finished_at, last_lat, last_lng, last_ping_at`

type sqlRepo struct {
	db *sql.DB
}

// NewRepo returns a Repo backed by the given database.
func NewRepo(db *sql.DB) Repo {
	return &sqlRepo{db: db}
}

func (r *sqlRepo) BindTargetOf(ctx context.Context, vehicleID string) (BindTarget, error) {
	const query = "SELECT event_id, COALESCE(route_id, ''), code, team_name FROM vehicle WHERE id = ?"

	var target BindTarget
	err := r.db.QueryRowContext(ctx, query, vehicleID).
		Scan(&target.EventID, &target.RouteID, &target.Code, &target.TeamName)
	if errors.Is(err, sql.ErrNoRows) {
		return BindTarget{}, ErrVehicleNotFound
	}
	if err != nil {
		return BindTarget{}, fmt.Errorf("select vehicle %s: %w", vehicleID, err)
	}

	crew, err := r.crewIDsOf(ctx, vehicleID)
	if err != nil {
		return BindTarget{}, err
	}
	target.CrewMemberID = crew

	return target, nil
}

func (r *sqlRepo) crewIDsOf(ctx context.Context, vehicleID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id FROM crew_member WHERE vehicle_id = ?", vehicleID)
	if err != nil {
		return nil, fmt.Errorf("select crew of vehicle %s: %w", vehicleID, err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan crew id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate crew: %w", err)
	}

	return ids, nil
}

// CreateSession relies on the uq_live_session_per_vehicle index to enforce the
// one-active-phone rule, so two phones racing to bind cannot both succeed.
func (r *sqlRepo) CreateSession(ctx context.Context, s Session) error {
	const query = `
		INSERT INTO team_session (id, event_id, vehicle_id, status, bound_at, total_score)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query, s.ID, s.EventID, s.VehicleID, string(s.Status), s.BoundAt, s.TotalScore)
	if store.IsDuplicateKey(err) {
		return ErrAlreadyBound
	}
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	return nil
}

func (r *sqlRepo) GetSession(ctx context.Context, id string) (Session, error) {
	query := "SELECT " + sessionColumns + " FROM team_session WHERE id = ?"

	session, err := scanSession(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("select session %s: %w", id, err)
	}

	return session, nil
}

func (r *sqlRepo) UpdateSession(ctx context.Context, s Session) error {
	const query = `
		UPDATE team_session SET
			status = ?, current_waypoint_id = ?, total_score = ?,
			started_at = ?, finished_at = ?, last_lat = ?, last_lng = ?, last_ping_at = ?
		WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		string(s.Status), s.CurrentWaypointID, s.TotalScore,
		s.StartedAt, s.FinishedAt, s.LastLat, s.LastLng, s.LastPingAt, s.ID)
	if err != nil {
		return fmt.Errorf("update session %s: %w", s.ID, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update session %s: %w", s.ID, err)
	}
	if affected == 0 {
		if _, getErr := r.GetSession(ctx, s.ID); getErr != nil {
			return getErr
		}
	}

	return nil
}

func (r *sqlRepo) EventInfoOf(ctx context.Context, eventID string) (EventInfo, error) {
	const query = `
		SELECT status, COALESCE(cipher, ''), start_time,
		       start_lat, start_lng, start_radius_m,
		       end_lat, end_lng, end_radius_m
		FROM event WHERE id = ?`

	var (
		info                     EventInfo
		startLat, startLng       sql.NullFloat64
		endLat, endLng           sql.NullFloat64
		startRadiusM, endRadiusM int
	)
	err := r.db.QueryRowContext(ctx, query, eventID).Scan(
		&info.Status, &info.Cipher, &info.StartTime,
		&startLat, &startLng, &startRadiusM,
		&endLat, &endLng, &endRadiusM,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EventInfo{}, fmt.Errorf("%w: event %s", ErrNotFound, eventID)
	}
	if err != nil {
		return EventInfo{}, fmt.Errorf("select event %s: %w", eventID, err)
	}

	info.Start = circleFrom(startLat, startLng, startRadiusM)
	info.Finish = circleFrom(endLat, endLng, endRadiusM)

	return info, nil
}

// WaypointsOf loads a route's boundaries together with the tasks attached to
// each, which is everything EvaluatePing needs.
func (r *sqlRepo) WaypointsOf(ctx context.Context, routeID string) ([]WaypointGeo, error) {
	const query = `
		SELECT w.id, w.display_order, w.lat, w.lng, w.boundary_radius_m,
		       COALESCE(t.id, ''), COALESCE(t.type, '')
		FROM waypoint w
		LEFT JOIN waypoint_task wt ON wt.waypoint_id = w.id
		LEFT JOIN task t ON t.id = wt.task_id
		WHERE w.route_id = ?
		ORDER BY w.display_order, wt.display_order`

	rows, err := r.db.QueryContext(ctx, query, routeID)
	if err != nil {
		return nil, fmt.Errorf("select waypoints of route %s: %w", routeID, err)
	}
	defer func() { _ = rows.Close() }()

	var (
		waypoints []WaypointGeo
		byID      = map[string]int{}
	)
	for rows.Next() {
		var (
			id            string
			order         int
			lat, lng      float64
			radiusM       int
			taskID, tType string
		)
		if err := rows.Scan(&id, &order, &lat, &lng, &radiusM, &taskID, &tType); err != nil {
			return nil, fmt.Errorf("scan waypoint: %w", err)
		}

		idx, seen := byID[id]
		if !seen {
			idx = len(waypoints)
			byID[id] = idx
			waypoints = append(waypoints, WaypointGeo{
				ID:     id,
				Order:  order,
				Circle: GeoCircle{Lat: lat, Lng: lng, RadiusM: radiusM, Placed: true},
			})
		}
		// The left join yields one row per attached task, and a single row with
		// an empty task id when the waypoint has none.
		if taskID != "" {
			waypoints[idx].Tasks = append(waypoints[idx].Tasks,
				WaypointTask{ID: taskID, Type: tasks.TaskType(tType)})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate waypoints: %w", err)
	}

	return waypoints, nil
}

func (r *sqlRepo) RouteIDOfVehicle(ctx context.Context, vehicleID string) (string, error) {
	var routeID string
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(route_id, '') FROM vehicle WHERE id = ?", vehicleID).Scan(&routeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrVehicleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("select route of vehicle %s: %w", vehicleID, err)
	}

	return routeID, nil
}

// TaskStatesOf lists every task on the crew's route, left-joined onto their own
// submissions so an unattempted task still appears.
func (r *sqlRepo) TaskStatesOf(ctx context.Context, sessionID, routeID string) ([]TaskState, error) {
	const query = `
		SELECT t.id, w.id, t.code, t.title, t.type, t.points,
		       COALESCE(s.status, 'pending'), COALESCE(s.awarded_points, 0)
		FROM waypoint w
		JOIN waypoint_task wt ON wt.waypoint_id = w.id
		JOIN task t ON t.id = wt.task_id
		LEFT JOIN task_submission s ON s.task_id = t.id AND s.session_id = ?
		WHERE w.route_id = ?
		ORDER BY w.display_order, wt.display_order`

	rows, err := r.db.QueryContext(ctx, query, sessionID, routeID)
	if err != nil {
		return nil, fmt.Errorf("select task states: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var states []TaskState
	for rows.Next() {
		var state TaskState
		if err := rows.Scan(&state.TaskID, &state.WaypointID, &state.Code, &state.Title,
			&state.Type, &state.Points, &state.Status, &state.Awarded); err != nil {
			return nil, fmt.Errorf("scan task state: %w", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task states: %w", err)
	}

	return states, nil
}

func (r *sqlRepo) CreateVoucher(ctx context.Context, v Voucher) error {
	const query = `
		INSERT INTO voucher (id, session_id, entry_code, locker_id, lunch_passes)
		VALUES (?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query, v.ID, v.SessionID, v.EntryCode, v.LockerID, v.LunchPasses)
	if store.IsDuplicateKey(err) {
		return nil // The crew already has a voucher; issuing is idempotent.
	}
	if err != nil {
		return fmt.Errorf("insert voucher: %w", err)
	}

	return nil
}

func (r *sqlRepo) VoucherOf(ctx context.Context, sessionID string) (Voucher, error) {
	const query = `
		SELECT id, session_id, COALESCE(entry_code, ''), COALESCE(locker_id, ''), lunch_passes
		FROM voucher WHERE session_id = ?`

	var v Voucher
	err := r.db.QueryRowContext(ctx, query, sessionID).
		Scan(&v.ID, &v.SessionID, &v.EntryCode, &v.LockerID, &v.LunchPasses)
	if errors.Is(err, sql.ErrNoRows) {
		return Voucher{}, ErrNoVoucher
	}
	if err != nil {
		return Voucher{}, fmt.Errorf("select voucher of session %s: %w", sessionID, err)
	}

	return v, nil
}

func (r *sqlRepo) CrewSizeOf(ctx context.Context, vehicleID string) (int, error) {
	var size int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM crew_member WHERE vehicle_id = ?", vehicleID).Scan(&size); err != nil {
		return 0, fmt.Errorf("count crew of vehicle %s: %w", vehicleID, err)
	}

	return size, nil
}

func (r *sqlRepo) VehicleCodeOf(ctx context.Context, vehicleID string) (string, error) {
	var code string
	err := r.db.QueryRowContext(ctx, "SELECT code FROM vehicle WHERE id = ?", vehicleID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrVehicleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("select code of vehicle %s: %w", vehicleID, err)
	}

	return code, nil
}

func (r *sqlRepo) SubmittableTaskOf(ctx context.Context, taskID string) (SubmittableTask, error) {
	const query = "SELECT id, event_id, code, type, points, config FROM task WHERE id = ?"

	var (
		task     SubmittableTask
		taskType string
		config   []byte
	)
	err := r.db.QueryRowContext(ctx, query, taskID).
		Scan(&task.ID, &task.EventID, &task.Code, &taskType, &task.Points, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return SubmittableTask{}, ErrTaskNotOnThisRally
	}
	if err != nil {
		return SubmittableTask{}, fmt.Errorf("select task %s: %w", taskID, err)
	}

	task.Type = tasks.TaskType(taskType)
	task.Config = config

	return task, nil
}

// SaveSubmission upserts the attempt and recomputes the session total in one
// transaction, so the score and the submissions it is derived from can never
// disagree.
func (r *sqlRepo) SaveSubmission(ctx context.Context, sub Submission) (int, error) {
	const upsert = `
		INSERT INTO task_submission
			(id, session_id, task_id, waypoint_id, status, payload, awarded_points, submitted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			waypoint_id = VALUES(waypoint_id),
			status = VALUES(status),
			payload = VALUES(payload),
			awarded_points = VALUES(awarded_points),
			submitted_at = VALUES(submitted_at)`

	const recompute = `
		UPDATE team_session
		SET total_score = (SELECT COALESCE(SUM(awarded_points), 0) FROM task_submission WHERE session_id = ?)
		WHERE id = ?`

	var total int
	err := store.InTx(ctx, r.db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, upsert,
			sub.ID, sub.SessionID, sub.TaskID, sub.WaypointID, sub.Status,
			[]byte(sub.Payload), sub.AwardedPoints, sub.SubmittedAt)
		if err != nil {
			return fmt.Errorf("upsert submission: %w", err)
		}

		if _, err := tx.ExecContext(ctx, recompute, sub.SessionID, sub.SessionID); err != nil {
			return fmt.Errorf("recompute session score: %w", err)
		}

		return tx.QueryRowContext(ctx,
			"SELECT total_score FROM team_session WHERE id = ?", sub.SessionID).Scan(&total)
	})
	if err != nil {
		return 0, err
	}

	return total, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row rowScanner) (Session, error) {
	var (
		s                 Session
		status            string
		currentWaypointID sql.NullString
		boundAt           sql.NullTime
		startedAt         sql.NullTime
		finishedAt        sql.NullTime
		lastPingAt        sql.NullTime
	)
	err := row.Scan(&s.ID, &s.EventID, &s.VehicleID, &status, &currentWaypointID, &s.TotalScore,
		&boundAt, &startedAt, &finishedAt, &s.LastLat, &s.LastLng, &lastPingAt)
	if err != nil {
		return Session{}, err
	}

	s.Status = Status(status)
	if currentWaypointID.Valid {
		id := currentWaypointID.String
		s.CurrentWaypointID = &id
	}
	s.BoundAt = timePtr(boundAt)
	s.StartedAt = timePtr(startedAt)
	s.FinishedAt = timePtr(finishedAt)
	s.LastPingAt = timePtr(lastPingAt)

	return s, nil
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	value := t.Time

	return &value
}

// circleFrom builds a boundary, marking it unplaced when the organizer has not
// dropped the pin, so it can never match a position.
func circleFrom(lat, lng sql.NullFloat64, radiusM int) GeoCircle {
	if !lat.Valid || !lng.Valid {
		return GeoCircle{RadiusM: radiusM}
	}

	return GeoCircle{Lat: lat.Float64, Lng: lng.Float64, RadiusM: radiusM, Placed: true}
}

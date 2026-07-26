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
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

const alertColumns = "id, vehicle_id, type, note, source, raised_by, lat, lng, raised_at, resolved_at"

// aliasedAlertColumns is the same projection qualified for the join in Search.
const aliasedAlertColumns = "a.id, a.vehicle_id, a.type, a.note, a.source, " +
	"a.raised_by, a.lat, a.lng, a.raised_at, a.resolved_at"

type sqlRepo struct {
	db *sql.DB
}

// NewRepo returns a Repo backed by the given database.
func NewRepo(db *sql.DB) Repo {
	return &sqlRepo{db: db}
}

func (r *sqlRepo) Create(ctx context.Context, a Alert) error {
	const query = "INSERT INTO vehicle_alert (" + alertColumns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	_, err := r.db.ExecContext(ctx, query,
		a.ID, a.VehicleID, string(a.Type), nullString(a.Note), string(a.Source),
		nullString(a.RaisedBy), a.Lat, a.Lng, a.RaisedAt, a.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}

	return nil
}

func (r *sqlRepo) Get(ctx context.Context, id string) (Alert, error) {
	const query = "SELECT " + alertColumns + " FROM vehicle_alert WHERE id = ?"

	alert, err := scanAlert(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, ErrNotFound
	}
	if err != nil {
		return Alert{}, fmt.Errorf("select alert %s: %w", id, err)
	}

	return alert, nil
}

func (r *sqlRepo) Resolve(ctx context.Context, id string, at time.Time) error {
	const query = "UPDATE vehicle_alert SET resolved_at = ? WHERE id = ? AND resolved_at IS NULL"

	result, err := r.db.ExecContext(ctx, query, at, id)
	if err != nil {
		return fmt.Errorf("resolve alert %s: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve alert %s: %w", id, err)
	}
	// Zero rows means the alert is missing or was resolved concurrently; the
	// service already read it, so re-reading tells the two apart.
	if affected == 0 {
		if _, getErr := r.Get(ctx, id); getErr != nil {
			return getErr
		}
	}

	return nil
}

func (r *sqlRepo) Search(ctx context.Context, eventID string, filter SearchFilter, page httpx.Page) ([]Alert, int, error) {
	where := " JOIN vehicle v ON v.id = a.vehicle_id WHERE v.event_id = ?"
	args := []any{eventID}
	if filter.OpenOnly {
		where += " AND a.resolved_at IS NULL"
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM vehicle_alert a"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alerts: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	query := "SELECT " + aliasedAlertColumns + " FROM vehicle_alert a" + where +
		" ORDER BY a.raised_at DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, query, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("select alerts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make([]Alert, 0, page.Limit)
	for rows.Next() {
		alert, err := scanAlert(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan alert: %w", err)
		}
		found = append(found, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate alerts: %w", err)
	}

	return found, total, nil
}

func (r *sqlRepo) OpenTypesOf(ctx context.Context, vehicleID string) ([]Type, error) {
	const query = "SELECT type FROM vehicle_alert WHERE vehicle_id = ? AND resolved_at IS NULL"

	rows, err := r.db.QueryContext(ctx, query, vehicleID)
	if err != nil {
		return nil, fmt.Errorf("select open alerts of vehicle %s: %w", vehicleID, err)
	}
	defer func() { _ = rows.Close() }()

	var types []Type
	for rows.Next() {
		var alertType string
		if err := rows.Scan(&alertType); err != nil {
			return nil, fmt.Errorf("scan alert type: %w", err)
		}
		types = append(types, Type(alertType))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open alerts: %w", err)
	}

	return types, nil
}

func (r *sqlRepo) EventIDOf(ctx context.Context, vehicleID string) (string, error) {
	var eventID string
	err := r.db.QueryRowContext(ctx, "SELECT event_id FROM vehicle WHERE id = ?", vehicleID).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("select event of vehicle %s: %w", vehicleID, err)
	}

	return eventID, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAlert(row rowScanner) (Alert, error) {
	var (
		alert             Alert
		alertType, source string
		note, raisedBy    sql.NullString
		resolvedAt        sql.NullTime
	)
	err := row.Scan(&alert.ID, &alert.VehicleID, &alertType, &note, &source,
		&raisedBy, &alert.Lat, &alert.Lng, &alert.RaisedAt, &resolvedAt)
	if err != nil {
		return Alert{}, err
	}

	alert.Type = Type(alertType)
	alert.Source = Source(source)
	alert.Note = note.String
	alert.RaisedBy = raisedBy.String
	if resolvedAt.Valid {
		at := resolvedAt.Time
		alert.ResolvedAt = &at
	}

	return alert, nil
}

// nullString stores an empty optional string as SQL NULL rather than "".
func nullString(s string) any {
	if s == "" {
		return nil
	}

	return s
}

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

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewID_Format(t *testing.T) {
	require.Regexp(t, hexID, NewID())
}

func TestNewID_IsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		id := NewID()
		_, dup := seen[id]
		require.False(t, dup, "NewID returned a duplicate: %s", id)
		seen[id] = struct{}{}
	}
}

func TestIsDuplicateKey(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"mysql 1062", &mysql.MySQLError{Number: duplicateEntryErrNo}, true},
		{"wrapped mysql 1062", fmt.Errorf("insert: %w", &mysql.MySQLError{Number: duplicateEntryErrNo}), true},
		{"other mysql error", &mysql.MySQLError{Number: 1045}, false},
		{"unrelated error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsDuplicateKey(tt.err))
		})
	}
}

func TestOpen_RejectsInvalidDSN(t *testing.T) {
	_, err := Open("not-a-dsn")

	require.Error(t, err)
}

// --- DB-backed tests: skipped unless TEST_DB_DSN is exported. ---

func TestMigrate_AppliesFullSchema(t *testing.T) {
	db := testDB(t)

	require.NoError(t, Migrate(db))

	// Every table from the design spec must exist after a single Up.
	for _, table := range []string{
		"event", "route", "waypoint", "waypoint_task", "task", "vehicle",
		"crew_member", "team_session", "task_submission", "vehicle_alert",
		"voucher", "debrief_video",
	} {
		var n int
		err := db.QueryRow(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
			table,
		).Scan(&n)
		require.NoError(t, err)
		require.Equal(t, 1, n, "table %q missing from the schema", table)
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	db := testDB(t)

	require.NoError(t, Migrate(db))
	require.NoError(t, Migrate(db), "a second Up must be a no-op, not an error")
}

func TestSchema_AllowsOnlyOneLiveSessionPerVehicle(t *testing.T) {
	db := testDB(t)
	require.NoError(t, Migrate(db))
	ctx := context.Background()
	eventID, vehicleID := seedVehicle(t, db)

	_, err := db.ExecContext(ctx,
		"INSERT INTO team_session (id, event_id, vehicle_id, status) VALUES (?, ?, ?, 'bound')",
		NewID(), eventID, vehicleID)
	require.NoError(t, err)

	// A second live session for the same vehicle is the one-active-phone
	// violation and must be rejected by the unique index, even though the
	// status differs.
	_, err = db.ExecContext(ctx,
		"INSERT INTO team_session (id, event_id, vehicle_id, status) VALUES (?, ?, ?, 'active')",
		NewID(), eventID, vehicleID)
	require.Error(t, err)
	require.True(t, IsDuplicateKey(err), "expected a duplicate-key error, got %v", err)
}

func TestSchema_AllowsRepeatedFinishedSessions(t *testing.T) {
	db := testDB(t)
	require.NoError(t, Migrate(db))
	ctx := context.Background()
	eventID, vehicleID := seedVehicle(t, db)

	// Finished sessions are historical rows; a vehicle may accumulate many.
	for range 2 {
		_, err := db.ExecContext(ctx,
			"INSERT INTO team_session (id, event_id, vehicle_id, status) VALUES (?, ?, ?, 'finished')",
			NewID(), eventID, vehicleID)
		require.NoError(t, err)
	}
}

func TestInTx_RollsBackOnError(t *testing.T) {
	db := testDB(t)
	require.NoError(t, Migrate(db))
	ctx := context.Background()
	id := NewID()
	wantErr := errors.New("handler said no")

	err := InTx(ctx, db, func(tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx,
			"INSERT INTO event (id, name, event_date, start_time, created_by) VALUES (?, 'R', '2027-01-01', '09:00', 'u')", id)
		require.NoError(t, execErr)
		return wantErr
	})

	require.ErrorIs(t, err, wantErr)
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event WHERE id = ?", id).Scan(&n))
	require.Zero(t, n, "the failed transaction must leave no rows behind")
}

func TestInTx_CommitsOnSuccess(t *testing.T) {
	db := testDB(t)
	require.NoError(t, Migrate(db))
	ctx := context.Background()
	id := NewID()

	err := InTx(ctx, db, func(tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx,
			"INSERT INTO event (id, name, event_date, start_time, created_by) VALUES (?, 'R', '2027-01-01', '09:00', 'u')", id)
		return execErr
	})

	require.NoError(t, err)
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event WHERE id = ?", id).Scan(&n))
	require.Equal(t, 1, n)
}

func seedVehicle(t *testing.T, db *sql.DB) (eventID, vehicleID string) {
	t.Helper()
	eventID, vehicleID = NewID(), NewID()
	_, err := db.Exec(
		"INSERT INTO event (id, name, event_date, start_time, created_by) VALUES (?, 'R', '2027-01-01', '09:00', 'u')",
		eventID)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO vehicle (id, event_id, code, team_name) VALUES (?, ?, ?, 'Team')",
		vehicleID, eventID, "PKT-"+vehicleID[:6])
	require.NoError(t, err)

	return eventID, vehicleID
}

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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// testLockName must match storetest.LockName: both serialise DB-backed tests
// against the one shared database. This package cannot import storetest —
// storetest imports store — so the few lines are duplicated rather than
// exporting test scaffolding from the production package.
const testLockName = "wso2_rally_test"

const testLockTimeoutSeconds = 120

// testDB opens the throwaway MySQL named by TEST_DB_DSN and leaves it empty.
// Tests that need a database skip themselves when the variable is unset, so
// `go test ./...` stays green on a machine without docker.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set TEST_DB_DSN to run store tests against MySQL")
	}

	db, err := Open(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, Migrate(db))
	lockForTest(t, db)
	truncateAll(t, db)

	return db
}

// lockForTest holds the shared advisory lock for the duration of the test, so a
// parallel package's truncation cannot wipe rows this test just seeded.
func lockForTest(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)

	var acquired sql.NullInt64
	err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", testLockName, testLockTimeoutSeconds).Scan(&acquired)
	require.NoError(t, err)
	require.True(t, acquired.Valid && acquired.Int64 == 1, "timed out waiting for the %q test lock", testLockName)

	t.Cleanup(func() {
		_, err := conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", testLockName)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	})
}

// truncateAll empties every domain table.
//
// It pins one connection: FOREIGN_KEY_CHECKS is session-scoped, so suspending
// it on the pool would let the TRUNCATEs run on a connection that still has the
// checks enabled.
func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()

	_, err = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
	require.NoError(t, err)
	defer func() {
		_, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
		require.NoError(t, err)
	}()

	for _, table := range []string{
		"voucher", "task_submission", "team_session", "vehicle_alert",
		"debrief_video", "crew_member", "vehicle", "waypoint_task",
		"waypoint", "route", "task", "event",
	} {
		_, err := conn.ExecContext(ctx, "TRUNCATE TABLE "+table)
		require.NoError(t, err, "truncating %s", table)
	}
}

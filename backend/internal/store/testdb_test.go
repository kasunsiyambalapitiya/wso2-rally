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
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	truncateAll(t, db)

	return db
}

// truncateAll empties every domain table. Foreign-key checks are disabled for
// the duration so the tables can be cleared in any order.
func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	require.NoError(t, err)
	defer func() {
		_, err := db.Exec("SET FOREIGN_KEY_CHECKS = 1")
		require.NoError(t, err)
	}()

	for _, table := range []string{
		"voucher", "task_submission", "team_session", "vehicle_alert",
		"debrief_video", "crew_member", "vehicle", "waypoint_task",
		"waypoint", "route", "task", "event",
	} {
		_, err := db.Exec("TRUNCATE TABLE " + table)
		require.NoError(t, err, "truncating %s", table)
	}
}

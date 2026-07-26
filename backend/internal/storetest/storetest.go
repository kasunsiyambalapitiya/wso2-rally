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

// Package storetest gives the domain repository tests a migrated, empty MySQL
// to work against.
//
// Tests calling DB skip themselves when TEST_DB_DSN is unset, so `go test ./...`
// stays green on a machine without docker. Bring the database up with
// `make docker-db` and run `make test-integration` to exercise them.
package storetest

import (
	"database/sql"
	"os"
	"testing"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
)

// DSNEnvVar names the environment variable holding the throwaway database DSN.
const DSNEnvVar = "TEST_DB_DSN"

// tables lists every domain table in dependency order, children first, so they
// can be truncated safely.
var tables = []string{
	"voucher", "task_submission", "team_session", "vehicle_alert",
	"debrief_video", "crew_member", "vehicle", "waypoint_task",
	"waypoint", "route", "task", "event",
}

// DB returns a migrated, empty database, or skips the test when TEST_DB_DSN is
// unset. The connection is closed when the test finishes.
func DB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv(DSNEnvVar)
	if dsn == "" {
		t.Skipf("set %s to run this test against MySQL", DSNEnvVar)
	}

	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	Truncate(t, db)

	return db
}

// Truncate empties every domain table. Foreign-key checks are suspended so the
// order of deletion cannot matter.
func Truncate(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("disable foreign key checks: %v", err)
	}
	defer func() {
		if _, err := db.Exec("SET FOREIGN_KEY_CHECKS = 1"); err != nil {
			t.Errorf("re-enable foreign key checks: %v", err)
		}
	}()

	for _, table := range tables {
		if _, err := db.Exec("TRUNCATE TABLE " + table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

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
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
)

// DSNEnvVar names the environment variable holding the throwaway database DSN.
const DSNEnvVar = "TEST_DB_DSN"

// LockName is the MySQL advisory lock that serialises DB-backed tests.
//
// `go test ./...` runs package test binaries in parallel, and every one of them
// points at the same database. Without this lock, one package's Truncate wipes
// rows another package has just seeded — which surfaces as a foreign-key
// violation or a mysteriously empty result, in a different test each run.
//
// Per-database isolation would be faster, but the test user granted by
// docker-compose cannot CREATE DATABASE, whereas GET_LOCK needs no privileges.
const LockName = "wso2_rally_test"

// lockTimeout bounds the wait for the advisory lock. It has to cover the whole
// DB-backed run of every other package, so it is generous.
const lockTimeout = 120 * time.Second

// tables lists every domain table in dependency order, children first, so they
// can be truncated safely.
var tables = []string{
	"voucher", "task_submission", "team_session", "vehicle_alert",
	"debrief_video", "crew_member", "vehicle", "waypoint_task",
	"waypoint", "route", "task", "event",
}

// One pool per test binary, opened on first use. Sharing it keeps a package
// with thirty DB tests from opening thirty connection pools.
var (
	poolOnce sync.Once
	pool     *sql.DB
	poolErr  error
)

// DB returns a migrated, empty database, or skips the test when TEST_DB_DSN is
// unset.
//
// It holds an advisory lock for the duration of the test, so DB-backed tests
// run one at a time across every package. The lock is released, and the tables
// left empty, when the test finishes.
func DB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv(DSNEnvVar)
	if dsn == "" {
		t.Skipf("set %s to run this test against MySQL", DSNEnvVar)
	}

	poolOnce.Do(func() { pool, poolErr = openPool(dsn) })
	if poolErr != nil {
		t.Fatalf("open test database: %v", poolErr)
	}

	lock(t, pool)
	Truncate(t, pool)

	return pool
}

func openPool(dsn string) (*sql.DB, error) {
	db, err := store.Open(dsn)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate test database: %w", err)
	}

	return db, nil
}

// lock takes the advisory lock on a pinned connection — MySQL scopes GET_LOCK
// to a session, so it cannot be acquired from a pool at large.
func lock(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve a connection for the test lock: %v", err)
	}

	var acquired sql.NullInt64
	err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)",
		LockName, int(lockTimeout.Seconds())).Scan(&acquired)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("acquire the test lock: %v", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		_ = conn.Close()
		t.Fatalf("timed out after %s waiting for the %q test lock; another package may be stuck",
			lockTimeout, LockName)
	}

	t.Cleanup(func() {
		if _, err := conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", LockName); err != nil {
			t.Errorf("release the test lock: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Errorf("return the lock connection: %v", err)
		}
	})
}

// Truncate empties every domain table.
//
// The whole sequence runs on one pinned connection: FOREIGN_KEY_CHECKS is
// session-scoped, so suspending it on a pool would leave the TRUNCATEs free to
// land on a different connection that still has the checks enabled.
func Truncate(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve a connection to truncate: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("return the truncate connection: %v", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("disable foreign key checks: %v", err)
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
			t.Errorf("re-enable foreign key checks: %v", err)
		}
	}()

	for _, table := range tables {
		if _, err := conn.ExecContext(ctx, "TRUNCATE TABLE "+table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

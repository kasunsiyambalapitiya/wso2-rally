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

// Package store owns the MySQL connection, entity id generation, transaction
// helpers, and the embedded schema migrations. Domain packages depend on it
// for those primitives and keep their own SQL in their repo.go.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Connection-pool sizing. The rally peaks at ~150 phones plus a handful of
// organizer browsers, which these limits comfortably absorb.
const (
	maxOpenConns    = 25
	maxIdleConns    = 10
	connMaxLifetime = 5 * time.Minute
	pingTimeout     = 5 * time.Second
)

// idBytes is half of the 32-hex-character id width used by every entity.
const idBytes = 16

// duplicateEntryErrNo is MySQL's ER_DUP_ENTRY. It is how a unique-index
// violation — such as a second live session for one vehicle — surfaces.
const duplicateEntryErrNo = 1062

// Open connects to MySQL, configures the pool, and verifies the connection is
// usable before returning. A DSN that cannot serve a ping is a startup error,
// not something to discover on the first request.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		// Closing keeps a failed startup from leaking the pool.
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}

// NewID returns a fresh 32-character lowercase hex entity id.
//
// crypto/rand is the source: ids appear in URLs and team tokens, so they must
// not be guessable. A failure there means the process has no entropy and
// cannot safely continue.
func NewID() string {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("store: crypto/rand unavailable: %v", err))
	}

	return hex.EncodeToString(b)
}

// InTx runs fn inside a transaction, committing when it returns nil and
// rolling back otherwise. A panic inside fn rolls back and re-panics so the
// recover middleware still sees it.
func InTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err == nil {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rbErr))
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// IsDuplicateKey reports whether err is a MySQL unique-index violation,
// including when it is wrapped. Callers turn it into a domain conflict — for
// example sessions.ErrAlreadyBound — instead of a 500.
func IsDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntryErrNo
}

// IsDuplicateKeyOn reports whether err is a unique-index violation on one
// specific index, named without its table qualifier.
//
// IsDuplicateKey is too coarse where the collision decides a rule rather than
// merely reporting a conflict. Claiming a task races every phone in the car for
// uq_submission_session_task, and losing that race means "someone already
// answered"; a collision on any *other* index — a PRIMARY KEY clash from
// NewID(), or an index added later — means something quite different and must
// not be reported as a lost race.
//
// The index name is matched inside MySQL's message, which reads:
//
//	Duplicate entry 'x-y' for key 'task_submission.uq_submission_session_task'
//
// Matching a message is unlovely, but the protocol carries the index name
// nowhere else, and the alternative is inferring the cause from an error that
// cannot distinguish it.
func IsDuplicateKeyOn(err error, index string) bool {
	if !IsDuplicateKey(err) || index == "" {
		return false
	}

	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}

	// The quotes matter: they stop "uq_x" from matching "uq_x_and_y", and the
	// leading separator stops it from matching a table called uq_x.
	return strings.Contains(mysqlErr.Message, "."+index+"'") ||
		strings.Contains(mysqlErr.Message, "'"+index+"'")
}

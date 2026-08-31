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

package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
)

// taskColumns is the shared projection. `trigger` is a MySQL reserved word and
// must stay backticked everywhere it appears.
const taskColumns = "id, event_id, code, title, type, `trigger`, points, sensor, config"

type sqlRepo struct {
	db *sql.DB
}

// NewRepo returns a Repo backed by the given database.
func NewRepo(db *sql.DB) Repo {
	return &sqlRepo{db: db}
}

func (r *sqlRepo) Create(ctx context.Context, task Task) error {
	const query = "INSERT INTO task (" + taskColumns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"

	_, err := r.db.ExecContext(ctx, query,
		task.ID, task.EventID, task.Code, task.Title,
		string(task.Type), string(task.Trigger), task.Points, string(task.Sensor), []byte(task.Config),
	)
	if store.IsDuplicateKey(err) {
		return ErrDuplicateCode
	}
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	return nil
}

func (r *sqlRepo) Get(ctx context.Context, id string) (Task, error) {
	const query = "SELECT " + taskColumns + " FROM task WHERE id = ?"

	task, err := scanTask(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("select task %s: %w", id, err)
	}

	return task, nil
}

func (r *sqlRepo) Update(ctx context.Context, task Task) error {
	const query = "UPDATE task SET code = ?, title = ?, type = ?, `trigger` = ?, points = ?, sensor = ?, config = ? " +
		"WHERE id = ?"

	result, err := r.db.ExecContext(ctx, query,
		task.Code, task.Title, string(task.Type), string(task.Trigger),
		task.Points, string(task.Sensor), []byte(task.Config), task.ID,
	)
	if store.IsDuplicateKey(err) {
		return ErrDuplicateCode
	}
	if err != nil {
		return fmt.Errorf("update task %s: %w", task.ID, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update task %s: %w", task.ID, err)
	}
	// MySQL also reports zero rows for a no-op update, so confirm the task is
	// really gone before saying so.
	if affected == 0 {
		if _, getErr := r.Get(ctx, task.ID); getErr != nil {
			return getErr
		}
	}

	return nil
}

func (r *sqlRepo) Search(ctx context.Context, eventID string, page httpx.Page) ([]Task, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM task WHERE event_id = ?", eventID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	// Ordering by length then value keeps T2 ahead of T10 in the task library.
	const query = "SELECT " + taskColumns + " FROM task WHERE event_id = ? " +
		"ORDER BY CHAR_LENGTH(code), code LIMIT ? OFFSET ?"

	rows, err := r.db.QueryContext(ctx, query, eventID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("select tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make([]Task, 0, page.Limit)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan task: %w", err)
		}
		found = append(found, task)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tasks: %w", err)
	}

	return found, total, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (Task, error) {
	var (
		task                      Task
		taskType, trigger, sensor string
		config                    []byte
	)
	err := row.Scan(&task.ID, &task.EventID, &task.Code, &task.Title,
		&taskType, &trigger, &task.Points, &sensor, &config)
	if err != nil {
		return Task{}, err
	}

	task.Type = TaskType(taskType)
	task.Trigger = Trigger(trigger)
	task.Sensor = Sensor(sensor)
	task.Config = config

	return task, nil
}

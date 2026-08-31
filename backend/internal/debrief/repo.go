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

package debrief

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

const videoColumns = "id, event_id, vehicle_id, day, object_key, uploaded_at"

type sqlRepo struct {
	db *sql.DB
}

// NewRepo returns a Repo backed by the given database.
func NewRepo(db *sql.DB) Repo {
	return &sqlRepo{db: db}
}

func (r *sqlRepo) Create(ctx context.Context, v Video) error {
	const query = "INSERT INTO debrief_video (" + videoColumns + ") VALUES (?, ?, ?, ?, ?, ?)"

	_, err := r.db.ExecContext(ctx, query, v.ID, v.EventID, v.VehicleID, v.Day, v.ObjectKey, v.UploadedAt)
	if err != nil {
		return fmt.Errorf("insert debrief video: %w", err)
	}

	return nil
}

func (r *sqlRepo) Search(
	ctx context.Context, eventID string, filter SearchFilter, page httpx.Page,
) ([]Video, int, error) {
	where := " WHERE event_id = ?"
	args := []any{eventID}
	if filter.Day > 0 {
		where += " AND day = ?"
		args = append(args, filter.Day)
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM debrief_video"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count debrief videos: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	query := "SELECT " + videoColumns + " FROM debrief_video" + where +
		" ORDER BY day, uploaded_at DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, query, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("select debrief videos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make([]Video, 0, page.Limit)
	for rows.Next() {
		var (
			video     Video
			vehicleID sql.NullString
		)
		if err := rows.Scan(&video.ID, &video.EventID, &vehicleID,
			&video.Day, &video.ObjectKey, &video.UploadedAt); err != nil {
			return nil, 0, fmt.Errorf("scan debrief video: %w", err)
		}
		if vehicleID.Valid {
			id := vehicleID.String
			video.VehicleID = &id
		}
		found = append(found, video)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate debrief videos: %w", err)
	}

	return found, total, nil
}

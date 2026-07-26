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

// Package debrief tracks the video clips organizers attach for the evening
// debrief.
//
// The MVP stores an object key or URL only: uploading and transcoding happen
// outside this service.
package debrief

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/store"
)

// maxRallyDays bounds the day field. The rally runs over a long weekend, so a
// day outside this range is a typo.
const maxRallyDays = 7

// ErrNotFound means no debrief video exists with the requested id.
var ErrNotFound = fmt.Errorf("%w: debrief video", apperr.ErrNotFound)

// Video is one attached clip.
type Video struct {
	ID      string
	EventID string
	// VehicleID is nil for a clip about the whole field rather than one team.
	VehicleID *string
	// Day is which day of the rally the clip covers, starting at 1.
	Day        int
	ObjectKey  string
	UploadedAt time.Time
}

// AddVideoInput is a request to attach a clip.
type AddVideoInput struct {
	EventID   string
	VehicleID *string
	Day       int
	ObjectKey string
}

// SearchFilter narrows a search. Day zero matches every day.
type SearchFilter struct {
	Day int
}

// Repo is the persistence contract for debrief videos.
type Repo interface {
	Create(ctx context.Context, v Video) error
	Search(ctx context.Context, eventID string, filter SearchFilter, page httpx.Page) ([]Video, int, error)
}

// Service holds the debrief rules.
type Service struct {
	repo Repo
}

// NewService wires a Service to its repository.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// Add attaches a clip to an event.
func (s *Service) Add(ctx context.Context, in AddVideoInput) (Video, error) {
	video := Video{
		ID:         store.NewID(),
		EventID:    in.EventID,
		VehicleID:  in.VehicleID,
		Day:        in.Day,
		ObjectKey:  strings.TrimSpace(in.ObjectKey),
		UploadedAt: time.Now().UTC(),
	}
	if video.Day == 0 {
		video.Day = 1
	}

	if video.EventID == "" {
		return Video{}, apperr.Validationf("event id is required")
	}
	if video.ObjectKey == "" {
		return Video{}, apperr.Validationf("a video URL or object key is required")
	}
	if video.Day < 1 || video.Day > maxRallyDays {
		return Video{}, apperr.Validationf("day must be between 1 and %d, got %d", maxRallyDays, video.Day)
	}

	if err := s.repo.Create(ctx, video); err != nil {
		return Video{}, fmt.Errorf("create debrief video: %w", err)
	}

	return video, nil
}

// Search returns a page of an event's clips plus the unpaged total.
func (s *Service) Search(
	ctx context.Context, eventID string, filter SearchFilter, page httpx.Page,
) ([]Video, int, error) {
	if eventID == "" {
		return nil, 0, apperr.Validationf("event id is required")
	}
	if filter.Day < 0 {
		return nil, 0, apperr.Validationf("day must not be negative")
	}

	found, total, err := s.repo.Search(ctx, eventID, filter, page)
	if err != nil {
		return nil, 0, fmt.Errorf("search debrief videos of event %s: %w", eventID, err)
	}

	return found, total, nil
}

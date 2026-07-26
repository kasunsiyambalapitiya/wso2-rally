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
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

// VideoDTO is a debrief clip on the wire.
type VideoDTO struct {
	ID         string  `json:"id"`
	EventID    string  `json:"eventId"`
	VehicleID  *string `json:"vehicleId"`
	Day        int     `json:"day"`
	ObjectKey  string  `json:"objectKey"`
	UploadedAt string  `json:"uploadedAt"`
}

// AddVideoRequest is the POST /events/{eventId}/debrief-videos body.
type AddVideoRequest struct {
	VehicleID *string `json:"vehicleId"`
	Day       int     `json:"day"`
	ObjectKey string  `json:"objectKey"`
}

// SearchVideosRequest is the POST /events/{eventId}/debrief-videos/search body.
type SearchVideosRequest struct {
	Offset  int `json:"offset"`
	Limit   int `json:"limit"`
	Filters struct {
		Day int `json:"day"`
	} `json:"filters"`
}

// Handler exposes the debrief REST surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler wires a Handler to its service.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register adds the debrief endpoints to r.
func (h *Handler) Register(r chi.Router) {
	r.Post("/events/{eventId}/debrief-videos", h.add)
	r.Post("/events/{eventId}/debrief-videos/search", h.search)
}

func (h *Handler) add(w http.ResponseWriter, r *http.Request) {
	var req AddVideoRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	video, err := h.service.Add(r.Context(), AddVideoInput{
		EventID:   chi.URLParam(r, "eventId"),
		VehicleID: req.VehicleID,
		Day:       req.Day,
		ObjectKey: req.ObjectKey,
	})
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, toDTO(video))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	var req SearchVideosRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	found, total, err := h.service.Search(r.Context(),
		chi.URLParam(r, "eventId"),
		SearchFilter{Day: req.Filters.Day},
		httpx.NormalizePage(req.Offset, req.Limit),
	)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, httpx.NewSearchResult(toDTOs(found), total))
}

func toDTO(v Video) VideoDTO {
	return VideoDTO{
		ID:         v.ID,
		EventID:    v.EventID,
		VehicleID:  v.VehicleID,
		Day:        v.Day,
		ObjectKey:  v.ObjectKey,
		UploadedAt: v.UploadedAt.UTC().Format(time.RFC3339),
	}
}

func toDTOs(list []Video) []VideoDTO {
	out := make([]VideoDTO, 0, len(list))
	for _, v := range list {
		out = append(out, toDTO(v))
	}

	return out
}

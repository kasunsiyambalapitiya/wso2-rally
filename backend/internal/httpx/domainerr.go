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

package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// WriteDomainError turns a service error into the one response shape this API
// uses, so every handler maps errors identically.
//
// Categorised errors (apperr.*) become their status code, carrying the
// service's own client-safe wording where it has any. Anything else is an
// internal fault: logged in full with the request id, reported as a generic
// 500 that leaks nothing.
func WriteDomainError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		WriteError(w, http.StatusNotFound, messageOr(err, MsgNotFound))
	case errors.Is(err, apperr.ErrValidation):
		WriteError(w, http.StatusBadRequest, messageOr(err, MsgBadRequest))
	case errors.Is(err, apperr.ErrConflict):
		WriteError(w, http.StatusConflict, messageOr(err, MsgConflict))
	case errors.Is(err, apperr.ErrForbidden):
		WriteError(w, http.StatusForbidden, MsgForbidden)
	default:
		logger.Error("request failed",
			"error", err,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", RequestIDFrom(r.Context()),
		)
		WriteError(w, http.StatusInternalServerError, MsgInternal)
	}
}

// WriteBadRequest reports a malformed request body. The decoder's own message
// is safe to echo — it describes the caller's JSON, not our internals.
func WriteBadRequest(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmptyBody):
		WriteError(w, http.StatusBadRequest, "A JSON request body is required.")
	case errors.Is(err, ErrBodyTooLarge):
		WriteError(w, http.StatusRequestEntityTooLarge, "The request body is too large.")
	case errors.Is(err, ErrTrailingContent):
		WriteError(w, http.StatusBadRequest, "The request body must contain a single JSON object.")
	default:
		WriteError(w, http.StatusBadRequest, MsgBadRequest)
	}
}

func messageOr(err error, fallback string) string {
	if msg := apperr.Message(err); msg != "" {
		return capitalise(msg)
	}

	return fallback
}

// capitalise makes a service message read as a sentence in the response body.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	if c := s[0]; c >= 'a' && c <= 'z' {
		return string(c-'a'+'A') + s[1:]
	}

	return s
}

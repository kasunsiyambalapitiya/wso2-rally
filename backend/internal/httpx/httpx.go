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

// Package httpx holds the HTTP plumbing shared by every domain handler:
// JSON encoding, the single `{"message": ...}` error shape, request-body
// decoding, and request-id propagation.
//
// Error bodies never carry internal detail. Handlers log the underlying error
// with the actor id and return one of the Msg* constants to the client.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// MaxBodyBytes caps a decoded request body. Rally payloads are small; anything
// larger is a mistake or an attack.
const MaxBodyBytes int64 = 1 << 20 // 1 MiB

// Client-safe error messages. These are the only strings a non-2xx response
// ever contains.
const (
	MsgBadRequest   = "Request could not be processed."
	MsgUnauthorized = "Authentication required."
	MsgForbidden    = "You do not have access to this resource."
	MsgNotFound     = "Resource not found."
	MsgConflict     = "That action conflicts with the current state."
	MsgInternal     = "Something went wrong. Please try again."
)

// Errors returned by DecodeJSON. Handlers map all of them to 400.
var (
	ErrEmptyBody       = errors.New("request body is empty")
	ErrBodyTooLarge    = fmt.Errorf("request body exceeds %d bytes", MaxBodyBytes)
	ErrTrailingContent = errors.New("body must contain a single JSON object")
)

type ctxKey string

// RequestIDKey is the context key carrying the per-request correlation id.
const RequestIDKey ctxKey = "request-id"

// errorBody is the one and only error wire shape.
type errorBody struct {
	Message string `json:"message"`
}

// WriteJSON writes v as JSON with the given status code.
//
// An encoding failure cannot change the already-written status, so it is
// logged rather than returned; the client sees a truncated body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response body", "error", err, "status", status)
	}
}

// WriteError writes the standard `{"message": msg}` body with the given status.
// msg must be client-safe — pass one of the Msg* constants.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, errorBody{Message: msg})
}

// WriteNoContent ends a request that has nothing to return.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// DecodeJSON decodes exactly one JSON object from the request body into dst.
//
// It rejects unknown fields, empty bodies, oversized bodies, and any content
// trailing the first object, so a malformed request fails at the edge instead
// of silently populating a partial struct.
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return ErrEmptyBody
	}
	if r.ContentLength > MaxBodyBytes {
		return ErrBodyTooLarge
	}

	dec := json.NewDecoder(io.LimitReader(r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		switch {
		case errors.Is(err, io.EOF):
			return ErrEmptyBody
		case errors.Is(err, io.ErrUnexpectedEOF) && r.ContentLength >= MaxBodyBytes:
			return ErrBodyTooLarge
		default:
			return fmt.Errorf("decode json body: %w", err)
		}
	}
	if dec.More() {
		return ErrTrailingContent
	}

	return nil
}

// WithRequestID returns a context carrying the given correlation id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, RequestIDKey, id)
}

// RequestIDFrom returns the correlation id stored on ctx, or "" if absent.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(RequestIDKey).(string)
	return id
}

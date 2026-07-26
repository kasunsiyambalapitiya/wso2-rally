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

package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequestID_GeneratesAndEchoes(t *testing.T) {
	var seen string
	rr := httptest.NewRecorder()

	RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
	})).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Len(t, seen, 32)
	require.Equal(t, seen, rr.Header().Get(RequestIDHeader))
}

func TestRequestID_HonoursInboundHeader(t *testing.T) {
	var seen string
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "upstream-id")

	RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), req)

	require.Equal(t, "upstream-id", seen)
}

func TestSecurityHeaders(t *testing.T) {
	rr := httptest.NewRecorder()

	SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	require.NotEmpty(t, rr.Header().Get("Strict-Transport-Security"))
	require.NotEmpty(t, rr.Header().Get("Content-Security-Policy"))
}

func TestRecover_TurnsPanicInto500(t *testing.T) {
	rr := httptest.NewRecorder()

	Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, httpx.MsgInternal, messageOf(t, rr))
}

// A panic caused by a client hanging up is normal traffic, not a server fault,
// and must not be turned into a response on a dead connection.
func TestRecover_RepanicsOnAbortHandler(t *testing.T) {
	require.Panics(t, func() {
		Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	})
}

func TestLogger_PassesStatusThrough(t *testing.T) {
	rr := httptest.NewRecorder()

	Logger(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusTeapot, rr.Code)
}

// The WebSocket upgrade reaches for the underlying connection through
// http.ResponseController, which walks Unwrap(). If the logging wrapper hides
// it, /ws breaks.
func TestLogger_WrapperExposesUnderlyingWriter(t *testing.T) {
	var unwrapped bool

	Logger(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		type unwrapper interface{ Unwrap() http.ResponseWriter }
		u, ok := w.(unwrapper)
		require.True(t, ok, "the logging wrapper must implement Unwrap")
		_, unwrapped = u.Unwrap().(*httptest.ResponseRecorder)
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	require.True(t, unwrapped)
}

func TestCORS_DisabledWhenOriginEmpty(t *testing.T) {
	rr := httptest.NewRecorder()

	CORS("")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowsConfiguredOriginOnly(t *testing.T) {
	mw := CORS("http://localhost:3000")

	t.Run("matching origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rr := httptest.NewRecorder()

		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, req)

		require.Equal(t, "http://localhost:3000", rr.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("other origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Origin", "https://evil.example")
		rr := httptest.NewRecorder()

		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, req)

		require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("preflight short-circuits", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/x", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rr := httptest.NewRecorder()
		called := false

		mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		})).ServeHTTP(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		require.False(t, called)
	})
}

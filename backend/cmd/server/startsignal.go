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

package main

import (
	"context"
	"log/slog"
	"time"
)

// startSignalInterval is how often the scheduler checks for a start that has
// arrived. One second is what makes the release feel synchronised across a
// grid of phones; the check is one small query, so it costs nothing.
const startSignalInterval = time.Second

// runStartSignals calls fire on every tick until ctx is cancelled.
//
// A failed tick is logged and the loop carries on: a database blip at 08:59
// must not cancel the 09:00 start, and the service leaves an event it could not
// release unfired so the next tick retries it.
func runStartSignals(ctx context.Context, fire func(context.Context) error, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := fire(ctx); err != nil {
				logger.Error("start signal check failed", "error", err)
			}
		}
	}
}

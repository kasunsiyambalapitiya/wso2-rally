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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import "@config/portalConfig";

export interface ApiConfig {
  /** Backend REST root, never with a trailing slash. */
  backendBaseUrl: string;
}

/**
 * Reads the backend base URL from the runtime config.
 *
 * Throws rather than defaulting to a guess: a web app pointed at the wrong
 * backend fails in ways that look like backend bugs.
 *
 * @returns {ApiConfig} The resolved API configuration.
 */
export function getApiConfig(): ApiConfig {
  const backendBaseUrl = window.config?.RALLY_BACKEND_BASE_URL;
  if (!backendBaseUrl) {
    throw new Error(
      "Api Config Error: Missing required configuration: RALLY_BACKEND_BASE_URL",
    );
  }

  return { backendBaseUrl: backendBaseUrl.replace(/\/+$/, "") };
}

/**
 * Derives the `/ws` endpoint from the backend base URL, so a deployment only
 * ever configures one host.
 *
 * @returns {string} The WebSocket URL for live event and session topics.
 */
export function getWebSocketUrl(): string {
  const { backendBaseUrl } = getApiConfig();

  return `${backendBaseUrl.replace(/^http/, "ws")}/ws`;
}

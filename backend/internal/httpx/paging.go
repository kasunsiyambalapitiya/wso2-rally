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

// Pagination bounds shared by every POST /<resource>/search endpoint.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Page is a normalised offset/limit window. Build one with NormalizePage so a
// hostile or careless client cannot ask for the whole table.
type Page struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// NormalizePage clamps a requested window into the supported range: a negative
// offset becomes zero, a missing limit becomes DefaultLimit, and anything above
// MaxLimit is capped.
func NormalizePage(offset, limit int) Page {
	if offset < 0 {
		offset = 0
	}
	switch {
	case limit <= 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}

	return Page{Offset: offset, Limit: limit}
}

// SearchResult is the envelope every search endpoint returns: the page of
// items plus the unpaged total, which the web app uses to render pagination.
type SearchResult[T any] struct {
	Items      []T `json:"items"`
	TotalCount int `json:"totalCount"`
}

// NewSearchResult wraps items, normalising a nil slice to an empty JSON array
// so clients never have to handle null.
func NewSearchResult[T any](items []T, total int) SearchResult[T] {
	if items == nil {
		items = []T{}
	}

	return SearchResult[T]{Items: items, TotalCount: total}
}

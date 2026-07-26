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

package taskengine

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// Crew-facing result wording, kept in one place so every task reads the same.
const (
	detailCorrect   = "Correct."
	detailIncorrect = "Not quite — no points for this one."
)

// answerConfig covers the types authored as "here is the right answer".
type answerConfig struct {
	Answer any `json:"answer"`
	// Tolerance is the allowed absolute difference for a numeric answer, for
	// tasks like the odometer reading where an exact match is unreasonable.
	Tolerance *float64 `json:"tolerance"`
}

type answerPayload struct {
	Answer any `json:"answer"`
}

// validateExactAnswer scores INPUT_SELECT: one choice, matched exactly.
// String answers are compared case-insensitively after trimming, so a crew is
// not punished for typing "api integration".
func validateExactAnswer(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg answerConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given answerPayload
	if err := decode(payload, &given, "answer"); err != nil {
		return Result{}, err
	}

	expected, expectedIsString := cfg.Answer.(string)
	actual, actualIsString := given.Answer.(string)
	if expectedIsString && actualIsString {
		match := strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(actual))
		return awarded(match, maxPoints, detailCorrect, detailIncorrect), nil
	}

	return awarded(equalJSON(cfg.Answer, given.Answer), maxPoints, detailCorrect, detailIncorrect), nil
}

// validateNumericAnswer scores INPUT_NUMBER: a number, optionally within a
// tolerance. It backs the signpost arithmetic, the milestone digits, and the
// odometer calibration.
func validateNumericAnswer(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg answerConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given answerPayload
	if err := decode(payload, &given, "answer"); err != nil {
		return Result{}, err
	}

	expected, ok := toFloat(cfg.Answer)
	if !ok {
		return Result{}, apperr.Validationf("this task's answer is not a number")
	}
	actual, ok := toFloat(given.Answer)
	if !ok {
		return Result{}, apperr.Validationf("the answer must be a number")
	}

	tolerance := 0.0
	if cfg.Tolerance != nil {
		tolerance = math.Abs(*cfg.Tolerance)
	}

	return awarded(math.Abs(expected-actual) <= tolerance, maxPoints, detailCorrect, detailIncorrect), nil
}

type multiSelectConfig struct {
	Answers []string `json:"answers"`
}

type multiSelectPayload struct {
	Answers []string `json:"answers"`
}

// validateMultiSelect scores MULTI_SELECT on set equality: order does not
// matter, but every expected choice must be present and nothing extra.
func validateMultiSelect(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg multiSelectConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given multiSelectPayload
	if err := decode(payload, &given, "answer"); err != nil {
		return Result{}, err
	}

	return awarded(sameSet(cfg.Answers, given.Answers), maxPoints, detailCorrect, detailIncorrect), nil
}

type barcodeConfig struct {
	Payload string `json:"payload"`
}

type barcodePayload struct {
	Payload string `json:"payload"`
}

// validateBarcode scores SCAN_BARCODE by exact payload match. The code is
// machine-read, so it is compared verbatim apart from surrounding whitespace —
// but manual entry is always allowed, so case is forgiven.
func validateBarcode(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg barcodeConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given barcodePayload
	if err := decode(payload, &given, "scan"); err != nil {
		return Result{}, err
	}

	match := strings.EqualFold(strings.TrimSpace(cfg.Payload), strings.TrimSpace(given.Payload))

	return awarded(match, maxPoints, "Checkpoint code accepted.", "That code does not match this checkpoint."), nil
}

type sequenceConfig struct {
	Solution []string `json:"solution"`
}

type sequencePayload struct {
	Answer []string `json:"answer"`
}

// validateGridFill scores GRID_FILL per correct cell, so a crossword that is
// three-quarters right still earns three-quarters of the points.
func validateGridFill(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg sequenceConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given sequencePayload
	if err := decode(payload, &given, "answer"); err != nil {
		return Result{}, err
	}
	if len(cfg.Solution) == 0 {
		return Result{}, apperr.Validationf("this task has no solution configured")
	}

	correct := 0
	for i, cell := range cfg.Solution {
		if i < len(given.Answer) && strings.EqualFold(strings.TrimSpace(cell), strings.TrimSpace(given.Answer[i])) {
			correct++
		}
	}

	points := scale(maxPoints, float64(correct)/float64(len(cfg.Solution)))
	if correct == len(cfg.Solution) {
		return Result{Correct: true, AwardedPoints: points, Detail: "Grid complete."}, nil
	}

	return Result{
		Correct:       false,
		AwardedPoints: points,
		Detail:        pluralCells(correct, len(cfg.Solution)),
	}, nil
}

// validateGateMatch scores GATE_MATCH all-or-nothing: the connectors are a
// sequence, and a sequence in the wrong order is simply wrong.
func validateGateMatch(config, payload json.RawMessage, maxPoints int) (Result, error) {
	var cfg sequenceConfig
	if err := decode(config, &cfg, "configuration"); err != nil {
		return Result{}, err
	}
	var given sequencePayload
	if err := decode(payload, &given, "answer"); err != nil {
		return Result{}, err
	}
	if len(cfg.Solution) == 0 {
		return Result{}, apperr.Validationf("this task has no solution configured")
	}

	match := len(cfg.Solution) == len(given.Answer)
	if match {
		for i, want := range cfg.Solution {
			if !strings.EqualFold(strings.TrimSpace(want), strings.TrimSpace(given.Answer[i])) {
				match = false
				break
			}
		}
	}

	return awarded(match, maxPoints, "Sequence matched.", "That is not the right order."), nil
}

// sameSet reports whether two lists hold the same values, ignoring order,
// repetition, case, and surrounding whitespace.
func sameSet(want, got []string) bool {
	wantSet := normalisedSet(want)
	gotSet := normalisedSet(got)
	if len(wantSet) != len(gotSet) {
		return false
	}

	for value := range wantSet {
		if _, ok := gotSet[value]; !ok {
			return false
		}
	}

	return true
}

func normalisedSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}

	return set
}

func pluralCells(correct, total int) string {
	noun := "cells"
	if correct == 1 {
		noun = "cell"
	}

	return fmt.Sprintf("%d %s correct out of %d.", correct, noun, total)
}

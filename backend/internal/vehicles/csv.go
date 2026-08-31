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

package vehicles

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/apperr"
)

// csvHeader is the exact column order of the import and export files.
var csvHeader = []string{"code", "team_name", "vehicle_type", "contact_number", "route_name", "crew_names"}

// crewSeparator splits the crew_names column. A pipe keeps the file readable
// in a spreadsheet, where a comma would need quoting.
const crewSeparator = "|"

// crewPhoneSeparator splits one crew entry into name and phone number. A colon
// cannot appear in either, unlike a comma (which the CSV owns) or a space
// (which names contain).
const crewPhoneSeparator = ":"

// crewEntryShape is how the crew column is described back to an organizer whose
// file was rejected.
const crewEntryShape = `"Name:0771234567", separated by "|"`

// utf8BOM is the byte-order mark Excel prepends when saving a UTF-8 CSV.
const utf8BOM = "\ufeff"

// csvRow is one parsed line, before route names are resolved to ids.
type csvRow struct {
	Code          string
	TeamName      string
	VehicleType   string
	ContactNumber string
	RouteName     string
	Crew          []CrewMemberInput
}

// parseCSV reads the provisioning file. The header row is required and its
// columns must be in the documented order, so a mis-shaped file fails loudly
// instead of importing a hundred vehicles into the wrong fields.
func parseCSV(r io.Reader) ([]csvRow, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = len(csvHeader)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, apperr.Validationf("the CSV file is empty")
	}
	if err != nil {
		return nil, apperr.Validationf("the CSV header could not be read: %v", err)
	}
	if err := checkHeader(header); err != nil {
		return nil, err
	}

	var rows []csvRow
	for line := 2; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, apperr.Validationf("line %d could not be read: %v", line, err)
		}

		row := csvRow{
			Code:          strings.TrimSpace(record[0]),
			TeamName:      strings.TrimSpace(record[1]),
			VehicleType:   strings.TrimSpace(record[2]),
			ContactNumber: strings.TrimSpace(record[3]),
			RouteName:     strings.TrimSpace(record[4]),
		}
		// Identity first: a row with no code is a more fundamental problem than
		// a malformed crew entry, and reporting the crew instead would send an
		// organizer looking in the wrong column.
		if row.Code == "" {
			return nil, apperr.Validationf("line %d has no vehicle code", line)
		}
		if row.TeamName == "" {
			return nil, apperr.Validationf("line %d has no team name", line)
		}

		crew, err := splitCrew(record[5])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		row.Crew = crew
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, apperr.Validationf("the CSV file has a header but no vehicles")
	}

	return rows, nil
}

func checkHeader(header []string) error {
	for i, want := range csvHeader {
		// A UTF-8 BOM from Excel would otherwise corrupt the first column name.
		got := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(header[i], utf8BOM)))
		if got != want {
			return apperr.Validationf("CSV column %d must be %q, got %q", i+1, want, got)
		}
	}

	return nil
}

// splitCrew parses the crew_names column, whose entries are Name:phone.
//
// The phone is not optional. A member without one could never join their car, so
// accepting a bare name here would import a roster that looks provisioned and
// silently leaves someone unable to take part on rally morning. The line number
// is the caller's to add.
func splitCrew(field string) ([]CrewMemberInput, error) {
	trimmedField := strings.TrimSpace(field)
	if trimmedField == "" {
		// A car with no crew listed yet is allowed; one with a nameless or
		// numberless member is not.
		return nil, nil
	}

	var crew []CrewMemberInput
	for entry := range strings.SplitSeq(trimmedField, crewSeparator) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, phone, found := strings.Cut(entry, crewPhoneSeparator)
		if !found {
			return nil, apperr.Validationf(
				"crew entry %q needs a phone number, written %s", entry, crewEntryShape)
		}
		name, phone = strings.TrimSpace(name), strings.TrimSpace(phone)
		if name == "" {
			return nil, apperr.Validationf("crew entry %q has no name", entry)
		}
		if err := validatePhoneNumber(name, phone); err != nil {
			return nil, err
		}

		crew = append(crew, CrewMemberInput{Name: name, PhoneNumber: phone})
	}

	return crew, nil
}

// writeCSV renders vehicles in the same shape parseCSV accepts, so an export
// can be edited and imported straight back.
func writeCSV(w io.Writer, list []Vehicle, routeNames map[string]string) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(csvHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, v := range list {
		entries := make([]string, 0, len(v.Crew))
		for _, member := range v.Crew {
			entries = append(entries, member.Name+crewPhoneSeparator+member.PhoneNumber)
		}
		record := []string{
			v.Code, v.TeamName, v.VehicleType, v.ContactNumber,
			routeNames[v.RouteID], strings.Join(entries, crewSeparator),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write csv row for %s: %w", v.Code, err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}

	return nil
}

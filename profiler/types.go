package profiler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// SearchQuery holds the query parameters for searching for profiles.
type SearchQuery struct {
	Filter SearchFilter `json:"filter"`
	Sort   SearchSort   `json:"sort"`
	Limit  int          `json:"limit"`
}

// SearchFilter holds the filter parameters for searching for profiles.
type SearchFilter struct {
	From  JSONTime `json:"from"`
	To    JSONTime `json:"to"`
	Query string   `json:"query"`
}

// SearchSort holds the sort parameters for searching for profiles.
type SearchSort struct {
	Order string `json:"order"`
	Field string `json:"field"`
}

// timeFormat is the time format used by the Datadog API.
const timeFormat = "2006-01-02T15:04:05.999999999Z"

// JSONTime is a time.Time that marshals to and from JSON in the format used by
// the Datadog API.
type JSONTime struct {
	time.Time
}

// MarshalJSON marshals the time in the format used by the Datadog API.
func (t JSONTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON unmarshals the time from the format used by the Datadog API.
func (t *JSONTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(timeFormat, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// String returns the time in the format used by the Datadog API.
func (t JSONTime) String() string {
	return t.Time.UTC().Round(time.Second).Format(timeFormat)
}

// SearchProfile holds information about a profile search result. ProfileID and
// EventID are used to identify the SearchProfile for downloading. The other
// fields are just logged for debugging.
type SearchProfile struct {
	Service   string
	CPUCores  float64
	ProfileID string
	EventID   string
	Timestamp time.Time
	Duration  time.Duration
}

// ProfileDownload is the result of downloading a profile.
type ProfileDownload struct {
	data []byte
}

// ExtractCPUProfile extracts the CPU profile from the download.
func (d ProfileDownload) ExtractCPUProfile() ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(d.data), int64(len(d.data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "cpu.pprof" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}

	return nil, errors.New("no cpu.pprof found in download")
}

// ProfilesDownload is the result of downloading several profiles from the pgo
// endpoint.
type ProfilesDownload struct {
	data []byte
}

// wrapErr wraps the error with name if it is not nil.
func wrapErr(err *error, name string) {
	if *err != nil {
		*err = fmt.Errorf("%s: %w", name, *err)
	}
}

const version = "0.0.1"

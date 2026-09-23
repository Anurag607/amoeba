package runtimekit

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a human-readable time.Duration used consistently by YAML and JSON.
type Duration time.Duration

// UnmarshalText accepts values such as "5s" and "250ms".
func (d *Duration) UnmarshalText(text []byte) error {
	value, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	*d = Duration(value)
	return nil
}

// MarshalText renders a duration in Go's stable duration syntax.
func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// UnmarshalJSON requires a quoted human-readable duration.
func (d *Duration) UnmarshalJSON(body []byte) error {
	var text string
	if err := json.Unmarshal(body, &text); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	return d.UnmarshalText([]byte(text))
}

// MarshalJSON renders a quoted human-readable duration.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

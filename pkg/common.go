package envoy

import (
	"encoding/json"
	"time"
)

type Dimensioned struct {
	Units string `json:"units"`
	Value string `json:"value"`
}

type Size struct {
	Length int    `json:"length"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Units  string `json:"units"`
}

type Value struct {
	Currency string  `json:"currency"`
	Value    float64 `json:"value"`
}

type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type BoolString bool

func (b *BoolString) UnmarshalJSON(data []byte) error {
	var s string
	// Try to unmarshal as a string first
	if err := json.Unmarshal(data, &s); err == nil {
		*b = (s == "true")
		return nil
	}
	// If not a string, try parsing as a normal boolean
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err != nil {
		return err
	}
	*b = BoolString(boolVal)
	return nil
}

type LocalDateTime struct {
	time.Time
}

func (t *LocalDateTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tz, err := time.Parse(time.RFC3339, s)
	if err == nil {
		*t = LocalDateTime{tz}
		return nil
	}

	tt, err := time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		*t = LocalDateTime{tt}
		return nil
	}

	return err
}

type LocalDate struct {
	time.Time
}

func (t *LocalDate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tt, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}

	*t = LocalDate{tt}
	return nil
}

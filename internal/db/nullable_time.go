package db

import (
	"bytes"
	"database/sql"
	"fmt"
	"time"
)

// NullableTime wraps sql.NullTime with JSON marshaling that produces a
// JSON null or an RFC3339 timestamp string instead of the default Go
// `{"Time":..., "Valid":...}` shape. The frontend (and any other JSON
// consumer) sees a `string | null` value, which is what every existing
// TypeScript model already declares.
//
// Because NullableTime embeds sql.NullTime, the database/sql Scanner and
// driver.Valuer methods are promoted automatically, so query files do not
// need to change beyond the field type itself.
type NullableTime struct {
	sql.NullTime
}

// NewNullableTime returns a NullableTime populated with t.UTC() and Valid=true.
func NewNullableTime(t time.Time) NullableTime {
	return NullableTime{NullTime: sql.NullTime{Time: t.UTC(), Valid: true}}
}

// NullableTimeNow returns a NullableTime initialized to time.Now().UTC().
func NullableTimeNow() NullableTime {
	return NewNullableTime(time.Now())
}

// MarshalJSON emits "null" when Valid is false; otherwise the embedded
// time.Time's RFC3339 representation.
func (n NullableTime) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return n.Time.MarshalJSON()
}

// UnmarshalJSON accepts JSON null (Valid=false) or any value time.Time
// itself accepts (RFC3339 strings, Valid=true). Anything else returns an
// error so callers can surface bad input to the user.
func (n *NullableTime) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Valid = false
		n.Time = time.Time{}
		return nil
	}
	var t time.Time
	if err := t.UnmarshalJSON(data); err != nil {
		return fmt.Errorf("nullable time unmarshal: %w", err)
	}
	n.Time = t
	n.Valid = true
	return nil
}

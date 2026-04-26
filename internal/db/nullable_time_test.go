package db

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNullableTimeMarshalJSON(t *testing.T) {
	// Invalid → "null".
	var invalid NullableTime
	got, err := json.Marshal(invalid)
	if err != nil {
		t.Fatalf("marshal invalid: %v", err)
	}
	if string(got) != "null" {
		t.Fatalf("invalid marshal = %q, want %q", got, "null")
	}

	// Valid → RFC3339 ISO string.
	moment := time.Date(2026, 4, 26, 12, 30, 45, 0, time.UTC)
	valid := NullableTime{NullTime: sqlNullTime(moment, true)}
	got, err = json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid: %v", err)
	}
	if !strings.Contains(string(got), "2026-04-26T12:30:45") {
		t.Fatalf("valid marshal = %q, want it to contain the timestamp", got)
	}
	// Must NOT have the {"Time":..., "Valid":...} default shape.
	if bytes.Contains(got, []byte(`"Valid"`)) {
		t.Fatalf("valid marshal leaked sql.NullTime default shape: %q", got)
	}
}

func TestNullableTimeUnmarshalJSON(t *testing.T) {
	// "null" → Valid=false.
	var n NullableTime
	if err := json.Unmarshal([]byte("null"), &n); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if n.Valid {
		t.Fatalf("null unmarshal: Valid = true, want false")
	}

	// ISO string → Valid=true.
	var n2 NullableTime
	if err := json.Unmarshal([]byte(`"2026-04-26T12:30:45Z"`), &n2); err != nil {
		t.Fatalf("unmarshal valid: %v", err)
	}
	if !n2.Valid {
		t.Fatalf("valid unmarshal: Valid = false, want true")
	}
	if n2.Time.Year() != 2026 || n2.Time.Month() != 4 || n2.Time.Day() != 26 {
		t.Fatalf("valid unmarshal: time = %v, want 2026-04-26", n2.Time)
	}

	// Garbage → error.
	var n3 NullableTime
	if err := json.Unmarshal([]byte(`"not a timestamp"`), &n3); err == nil {
		t.Fatalf("unmarshal garbage: expected error, got nil")
	}
}

func TestNullableTimeRoundTripThroughJSON(t *testing.T) {
	original := NullableTime{NullTime: sqlNullTime(time.Date(2026, 4, 26, 12, 30, 45, 0, time.UTC), true)}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded NullableTime
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Valid {
		t.Fatalf("round-trip: Valid = false, want true")
	}
	if !decoded.Time.Equal(original.Time) {
		t.Fatalf("round-trip: time = %v, want %v", decoded.Time, original.Time)
	}
}

func TestNullableTimeDBRoundTrip(t *testing.T) {
	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 200, Username: "nt-roundtrip", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	moment := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	token := &APIToken{
		UserID:    user.ID,
		Name:      "round-trip",
		TokenHash: "round-trip-hash",
		ExpiresAt: NullableTime{NullTime: sqlNullTime(moment, true)},
	}
	if err := database.CreateAPIToken(token); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := database.GetAPITokenByHash("round-trip-hash")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Note: GetAPITokenByHash returns nil for already-expired tokens. moment
	// is in the past relative to "now"; pick a future moment instead.
	if got != nil {
		t.Fatalf("expected past-dated token to be filtered as expired; got %+v", got)
	}

	// Now insert one with a future expiry and confirm it survives the round trip.
	future := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	token2 := &APIToken{
		UserID:    user.ID,
		Name:      "future",
		TokenHash: "future-hash",
		ExpiresAt: NullableTime{NullTime: sqlNullTime(future, true)},
	}
	if err := database.CreateAPIToken(token2); err != nil {
		t.Fatalf("create future: %v", err)
	}
	got, err = database.GetAPITokenByHash("future-hash")
	if err != nil {
		t.Fatalf("get future: %v", err)
	}
	if got == nil {
		t.Fatalf("future token vanished")
	}
	if !got.ExpiresAt.Valid {
		t.Fatalf("ExpiresAt.Valid = false after DB round trip")
	}
	if !got.ExpiresAt.Time.Equal(future) {
		t.Fatalf("ExpiresAt.Time = %v after round trip, want %v", got.ExpiresAt.Time, future)
	}

	// JSON marshaling on the round-tripped token must use the new format.
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"expires_at":"`) {
		t.Fatalf("expires_at not serialized as ISO string: %s", encoded)
	}
	if bytes.Contains(encoded, []byte(`"Valid"`)) {
		t.Fatalf("encoded JSON leaked sql.NullTime default shape: %s", encoded)
	}
}

// sqlNullTime is a tiny test helper to keep the literal noise down. NOT
// exported — production code should prefer the project's own helpers
// (e.g., NewNullableTime) when constructing values.
func sqlNullTime(t time.Time, valid bool) sql.NullTime {
	return sql.NullTime{Time: t, Valid: valid}
}

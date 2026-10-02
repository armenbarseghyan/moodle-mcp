package moodle_test

import (
	"encoding/json"
	"testing"
	"time"

	"moodle-mcp/internal/moodle"
)

func TestBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    bool
		wantErr bool
	}{
		{"true", true, false}, {"false", false, false},
		{"1", true, false}, {"0", false, false},
		{`"1"`, true, false}, {`"0"`, false, false},
		{`"true"`, true, false}, {`"false"`, false, false},
		{`""`, false, false}, {"null", false, false},
		{"2", false, true}, {`"yes"`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			var v struct {
				B moodle.Bool `json:"b"`
			}
			err := json.Unmarshal([]byte(`{"b":`+tt.in+`}`), &v)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if !tt.wantErr && bool(v.B) != tt.want {
				t.Errorf("got %v, want %v", v.B, tt.want)
			}
		})
	}
}

func TestUnix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in       string
		want     time.Time
		wantErr  bool
		wantZero bool
	}{
		{"1791386100", time.Unix(1791386100, 0), false, false},
		{`"1791386100"`, time.Unix(1791386100, 0), false, false},
		{"0", time.Time{}, false, true},
		{"null", time.Time{}, false, true},
		{`""`, time.Time{}, false, true},
		{"-5", time.Time{}, false, true},
		{`"soon"`, time.Time{}, true, false},
		{"1.5", time.Time{}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			var v struct {
				T moodle.Unix `json:"t"`
			}
			err := json.Unmarshal([]byte(`{"t":`+tt.in+`}`), &v)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tt.wantErr {
				return
			}
			got := v.T.Time()
			if got.IsZero() != tt.wantZero || (!tt.wantZero && !got.Equal(tt.want)) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

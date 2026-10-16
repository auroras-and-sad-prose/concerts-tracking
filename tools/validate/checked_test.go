package main

import (
	"strings"
	"testing"
	"time"
)

var checkedNow = time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)

func checkedRoster() *Artists {
	return &Artists{Artists: []Artist{{Slug: "scheps"}, {Slug: "perlman"}}}
}

func TestValidChecked(t *testing.T) {
	c := Checked{Artists: map[string]string{
		"scheps":  "2026-10-16T05:12:00Z",
		"perlman": "2026-10-14T05:10:00Z",
	}}
	if p := ValidateChecked(c, checkedRoster(), checkedNow); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

// The state before any run has stamped anything.
func TestEmptyChecked(t *testing.T) {
	for _, c := range []Checked{{}, {Artists: map[string]string{}}} {
		if p := ValidateChecked(c, checkedRoster(), checkedNow); len(p) != 0 {
			t.Fatalf("expected no problems, got %v", p)
		}
	}
}

func TestCheckedProblems(t *testing.T) {
	cases := []struct {
		name  string
		slug  string
		value string
		want  string
	}{
		{"unknown artist", "nobody", "2026-10-16T05:12:00Z", "not in the roster"},
		{"date only", "scheps", "2026-10-16", "UTC timestamp"},
		{"offset instead of Z", "scheps", "2026-10-16T07:12:00+02:00", "UTC timestamp"},
		{"no seconds", "scheps", "2026-10-16T05:12Z", "UTC timestamp"},
		{"fractional seconds", "scheps", "2026-10-16T05:12:00.5Z", "UTC timestamp"},
		{"not zero padded", "scheps", "2026-10-6T05:12:00Z", "UTC timestamp"},
		{"impossible date", "scheps", "2026-02-30T05:12:00Z", "UTC timestamp"},
		{"empty", "scheps", "", "UTC timestamp"},
		{"in the future", "scheps", "2026-10-17T05:12:00Z", "in the future"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Checked{Artists: map[string]string{tc.slug: tc.value}}
			p := ValidateChecked(c, checkedRoster(), checkedNow)
			if len(p) != 1 || !strings.Contains(p[0], tc.want) {
				t.Fatalf("want one problem containing %q, got %v", tc.want, p)
			}
		})
	}
}

// A few minutes ahead is clock skew between machines, not a lie.
func TestCheckedClockSkewIsTolerated(t *testing.T) {
	c := Checked{Artists: map[string]string{"scheps": "2026-10-16T06:05:00Z"}}
	if p := ValidateChecked(c, checkedRoster(), checkedNow); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

// Without a roster only the timestamp shape is checked.
func TestCheckedWithoutRoster(t *testing.T) {
	c := Checked{Artists: map[string]string{"nobody": "2026-10-16T05:12:00Z"}}
	if p := ValidateChecked(c, nil, checkedNow); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

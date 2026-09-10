package controllers

import "testing"

func TestNormalizeBureauName(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "trims surrounding space", raw: "  ADRAR  ", want: "ADRAR"},
		{name: "collapses internal runs", raw: "OUM EL   BOUGHI", want: "OUM EL BOUGHI"},
		{name: "collapses tabs and newlines", raw: "CHLEF\t\nTENES", want: "CHLEF TENES"},
		{name: "leaves a clean name alone", raw: "Bureau Bab Ezzouar", want: "Bureau Bab Ezzouar"},
		{name: "whitespace only becomes empty", raw: "   ", want: ""},
		{name: "empty stays empty", raw: "", want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeBureauName(c.raw); got != c.want {
				t.Fatalf("normalizeBureauName(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

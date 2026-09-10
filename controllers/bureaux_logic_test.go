package controllers

import "testing"

func TestIsBureauEligibleCarrier(t *testing.T) {
	cases := []struct {
		name    string
		carrier string
		want    bool
	}{
		{name: "osen is eligible", carrier: "Osen Express", want: true},
		{name: "leopard is eligible", carrier: "Leopard Express", want: true},
		{name: "anderson is eligible", carrier: "Anderson", want: true},
		{name: "zr is rejected", carrier: "ZR Express", want: false},
		{name: "zr lowercase is rejected", carrier: "zr express", want: false},
		{name: "zr mixed case is rejected", carrier: "Zr ExPrEsS", want: false},
		{name: "zr with padding is rejected", carrier: "  ZR  ", want: false},
		{name: "empty name is rejected", carrier: "", want: false},
		{name: "whitespace only is rejected", carrier: "   ", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isBureauEligibleCarrier(c.carrier); got != c.want {
				t.Fatalf("isBureauEligibleCarrier(%q) = %v, want %v", c.carrier, got, c.want)
			}
		})
	}
}

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

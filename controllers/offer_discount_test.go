package controllers

import "testing"

func TestValidateOfferDiscount(t *testing.T) {
	cases := []struct {
		typ string
		v   float64
		ok  bool
	}{
		{"percent", 10, true}, {"percent", 0, false}, {"percent", 100, false}, {"percent", 150, false},
		{"fixed", 500, true}, {"fixed", 0, false}, {"fixed", -1, false},
		{"override_price", 1990, true}, {"override_price", 0, false},
	}
	for _, c := range cases {
		if got := validateOfferDiscount(c.typ, c.v) == nil; got != c.ok {
			t.Errorf("%s %v: ok=%v want %v", c.typ, c.v, got, c.ok)
		}
	}
}

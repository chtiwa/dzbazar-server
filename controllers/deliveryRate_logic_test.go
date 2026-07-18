package controllers

import "testing"

func TestDeliveryRate(t *testing.T) {
	cases := []struct {
		name      string
		shipped   int64
		delivered int64
		want      *float64
	}{
		{name: "no shipped orders yet", shipped: 0, delivered: 0, want: nil},
		{name: "all shipped delivered", shipped: 4, delivered: 4, want: ptr(100)},
		{name: "none delivered", shipped: 4, delivered: 0, want: ptr(0)},
		{name: "partial", shipped: 3, delivered: 1, want: ptr(33.333333333333336)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deliveryRate(c.shipped, c.delivered)
			if (got == nil) != (c.want == nil) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			if got != nil && *got != *c.want {
				t.Fatalf("got %v, want %v", *got, *c.want)
			}
		})
	}
}

func ptr(f float64) *float64 { return &f }

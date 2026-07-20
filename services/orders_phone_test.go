package services

import "testing"

func TestIsValidPhoneNumber(t *testing.T) {
	cases := []struct {
		name  string
		phone string
		want  bool
	}{
		{"valid 05 number", "0512345678", true},
		{"valid 06 number", "0612345678", true},
		{"valid 07 number", "0712345678", true},
		{"wrong prefix", "0812345678", false},
		{"too short", "051234567", false},
		{"too long", "05123456789", false},
		{"non-digit characters", "05abcd5678", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidPhoneNumber(tc.phone); got != tc.want {
				t.Errorf("IsValidPhoneNumber(%q) = %v, want %v", tc.phone, got, tc.want)
			}
		})
	}
}

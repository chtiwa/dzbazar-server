package services

import "testing"

func TestCheckCap(t *testing.T) {
	cases := []struct {
		name    string
		max     int
		count   int64
		wantErr bool
	}{
		{"unlimited", -1, 1_000_000, false},
		{"under cap", 30, 29, false},
		{"at cap", 30, 30, true},
		{"over cap", 30, 31, true},
		{"zero cap blocks immediately", 0, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCap(tc.max, tc.count)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkCap(%d, %d) error = %v, wantErr %v", tc.max, tc.count, err, tc.wantErr)
			}
		})
	}
}

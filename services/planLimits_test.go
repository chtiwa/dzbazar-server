package services

import "testing"

func TestPhoneKey(t *testing.T) {
	for in, want := range map[string]string{
		"0554123456":        "554123456",
		"+213 554 12 34 56": "554123456",
		"213554123456":      "554123456",
		"05-54-12-34-56":    "554123456",
		"":                  "",
		"123":               "",
	} {
		if got := phoneKey(in); got != want {
			t.Errorf("phoneKey(%q) = %q, want %q", in, got, want)
		}
	}
}

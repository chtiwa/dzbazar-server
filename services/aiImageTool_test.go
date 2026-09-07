package services

import "testing"

func TestParseModerationVerdict(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantAllow  bool
		wantReason string // only checked when wantAllow is false and non-empty
	}{
		{"allow", "ALLOW", true, ""},
		{"allow lowercase", "allow", true, ""},
		{"allow with whitespace", "  ALLOW\n", true, ""},
		{"reject with reason", "REJECT: sexual content", false, "sexual content"},
		{"reject alone", "REJECT", false, ""},
		{"empty fails closed", "", false, ""},
		{"garbled fails closed", "I'm sorry, I can't help with that.", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseModerationVerdict(tc.raw)
			if tc.wantAllow {
				if err != nil {
					t.Fatalf("parseModerationVerdict(%q) = %v, want nil", tc.raw, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseModerationVerdict(%q) = nil, want a PromptRejectedError", tc.raw)
			}
			rejected, ok := err.(PromptRejectedError)
			if !ok {
				t.Fatalf("parseModerationVerdict(%q) error type = %T, want PromptRejectedError", tc.raw, err)
			}
			if rejected.Reason == "" {
				t.Fatalf("parseModerationVerdict(%q) reason is empty, want non-empty (fail closed)", tc.raw)
			}
			if tc.wantReason != "" && rejected.Reason != tc.wantReason {
				t.Fatalf("parseModerationVerdict(%q) reason = %q, want %q", tc.raw, rejected.Reason, tc.wantReason)
			}
		})
	}
}

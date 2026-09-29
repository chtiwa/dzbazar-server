package controllers

import (
	"testing"
	"time"
)

func TestVerifyOTPDecision(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute)
	past := now.Add(-10 * time.Minute)

	cases := []struct {
		name         string
		isVerified   bool
		storedOTP    string
		submittedOTP string
		expiresAt    *time.Time
		want         otpDecision
	}{
		{
			name:         "already verified user with wrong OTP never gets OK",
			isVerified:   true,
			storedOTP:    "123456",
			submittedOTP: "wrong",
			expiresAt:    &future,
			want:         otpAlreadyVerified,
		},
		{
			name:         "already verified user with CORRECT OTP still never gets OK",
			isVerified:   true,
			storedOTP:    "123456",
			submittedOTP: "123456",
			expiresAt:    &future,
			want:         otpAlreadyVerified,
		},
		{
			name:         "empty stored OTP never matches",
			isVerified:   false,
			storedOTP:    "",
			submittedOTP: "",
			expiresAt:    &future,
			want:         otpMismatch,
		},
		{
			name:         "matching OTP within expiry is OK",
			isVerified:   false,
			storedOTP:    "123456",
			submittedOTP: "123456",
			expiresAt:    &future,
			want:         otpOK,
		},
		{
			name:         "mismatched OTP is rejected",
			isVerified:   false,
			storedOTP:    "123456",
			submittedOTP: "654321",
			expiresAt:    &future,
			want:         otpMismatch,
		},
		{
			name:         "expired OTP is rejected even if it matches",
			isVerified:   false,
			storedOTP:    "123456",
			submittedOTP: "123456",
			expiresAt:    &past,
			want:         otpExpired,
		},
		{
			name:         "nil expiry is treated as expired",
			isVerified:   false,
			storedOTP:    "123456",
			submittedOTP: "123456",
			expiresAt:    nil,
			want:         otpExpired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := verifyOTPDecision(tc.isVerified, tc.storedOTP, tc.submittedOTP, tc.expiresAt, now)
			if got != tc.want {
				t.Fatalf("verifyOTPDecision() = %v, want %v", got, tc.want)
			}
		})
	}
}

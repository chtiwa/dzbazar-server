package controllers

import "testing"

func TestMemberUpdateDecision(t *testing.T) {
	newRole := func(r string) *string { return &r }

	cases := []struct {
		name                    string
		callerRole              string
		targetCurrentRole       string
		isSelf                  bool
		newRole                 *string
		wantAllowIdentityFields bool
		wantForbidden           bool
	}{
		{
			name:                    "self edit allows identity fields",
			callerRole:              "moderator",
			targetCurrentRole:       "moderator",
			isSelf:                  true,
			wantAllowIdentityFields: true,
			wantForbidden:           false,
		},
		{
			name:                    "owner editing another moderator: no identity fields, not forbidden",
			callerRole:              "owner",
			targetCurrentRole:       "moderator",
			isSelf:                  false,
			wantAllowIdentityFields: false,
			wantForbidden:           false,
		},
		{
			name:              "moderator editing another moderator: no identity fields, not forbidden",
			callerRole:        "moderator",
			targetCurrentRole: "moderator",
			isSelf:            false,
			wantForbidden:     false,
		},
		{
			name:              "non-owner editing the owner is forbidden",
			callerRole:        "moderator",
			targetCurrentRole: "owner",
			isSelf:            false,
			wantForbidden:     true,
		},
		{
			name:              "non-owner promoting someone to owner is forbidden",
			callerRole:        "moderator",
			targetCurrentRole: "moderator",
			isSelf:            false,
			newRole:           newRole("owner"),
			wantForbidden:     true,
		},
		{
			name:                    "owner promoting someone to owner is allowed",
			callerRole:              "owner",
			targetCurrentRole:       "moderator",
			isSelf:                  false,
			newRole:                 newRole("owner"),
			wantAllowIdentityFields: false,
			wantForbidden:           false,
		},
		{
			name:              "non-owner demoting the owner is forbidden",
			callerRole:        "moderator",
			targetCurrentRole: "owner",
			isSelf:            false,
			newRole:           newRole("moderator"),
			wantForbidden:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowIdentityFields, forbidden, reason := memberUpdateDecision(tc.callerRole, tc.targetCurrentRole, tc.isSelf, tc.newRole)
			if forbidden != tc.wantForbidden {
				t.Fatalf("forbidden = %v, want %v (reason=%q)", forbidden, tc.wantForbidden, reason)
			}
			if !forbidden && allowIdentityFields != tc.wantAllowIdentityFields {
				t.Fatalf("allowIdentityFields = %v, want %v", allowIdentityFields, tc.wantAllowIdentityFields)
			}
			if forbidden && reason == "" {
				t.Fatalf("forbidden decision must carry a reason")
			}
		})
	}
}

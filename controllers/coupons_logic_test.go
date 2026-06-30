package controllers

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

func TestCouponDiscount(t *testing.T) {
	productA := uuid.New()
	productB := uuid.New()
	landingPageProduct := uuid.New()

	cases := []struct {
		name         string
		coupon       models.Coupon
		productIDs   []uuid.UUID
		subtotal     float64
		wantMatched  bool
		wantDiscount float64
	}{
		{
			name:         "empty scope applies shop-wide",
			coupon:       models.Coupon{Percent: 10, Active: true},
			productIDs:   []uuid.UUID{productB},
			subtotal:     1999,
			wantMatched:  true,
			wantDiscount: 200, // 1999 * 10% = 199.9, rounds to 200
		},
		{
			name: "matches via linked product",
			coupon: models.Coupon{
				Percent: 25, Active: true,
				Products: []models.Product{{BaseModel: models.BaseModel{ID: productA}}},
			},
			productIDs:   []uuid.UUID{productA},
			subtotal:     1000,
			wantMatched:  true,
			wantDiscount: 250,
		},
		{
			name: "matches via linked landing page's product",
			coupon: models.Coupon{
				Percent: 50, Active: true,
				LandingPages: []models.LandingPage{{ProductID: landingPageProduct}},
			},
			productIDs:   []uuid.UUID{landingPageProduct},
			subtotal:     1000,
			wantMatched:  true,
			wantDiscount: 500,
		},
		{
			name: "out of scope does not match",
			coupon: models.Coupon{
				Percent: 25, Active: true,
				Products: []models.Product{{BaseModel: models.BaseModel{ID: productA}}},
			},
			productIDs:   []uuid.UUID{productB},
			subtotal:     1000,
			wantMatched:  false,
			wantDiscount: 0,
		},
		{
			name:         "inactive coupon never matches",
			coupon:       models.Coupon{Percent: 10, Active: false},
			productIDs:   []uuid.UUID{productA},
			subtotal:     1000,
			wantMatched:  false,
			wantDiscount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			discount, matched := couponDiscount(tc.coupon, tc.productIDs, tc.subtotal)
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if discount != tc.wantDiscount {
				t.Fatalf("discount = %v, want %v", discount, tc.wantDiscount)
			}
		})
	}
}

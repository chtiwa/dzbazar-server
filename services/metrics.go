package services

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// OrderCreationDuration tracks CreateOrderByShopID latency — the hottest,
// most business-critical write path (see server/CLAUDE.md async post-order
// tasks). Labeled by outcome so a spike in errors is visible separately
// from a general slowdown.
var OrderCreationDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "order_creation_duration_seconds",
		Help:    "Duration of CreateOrderByShopID, by outcome.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"outcome"}, // "success" | "error"
)

// CarrierCallDuration tracks latency/error rate of outbound calls to the
// courier APIs (Osen, ZR, Leopard, Anderson) — the other named target in
// the observability todo. Call ObserveCarrierCall around each outbound
// request as carrier code adopts it.
var CarrierCallDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "carrier_call_duration_seconds",
		Help:    "Duration of outbound carrier API calls, by carrier and outcome.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"carrier", "outcome"},
)

func init() {
	prometheus.MustRegister(OrderCreationDuration, CarrierCallDuration)
}

// ObserveCarrierCall times fn and records it under CarrierCallDuration,
// labeled by carrier ("osen" | "zr" | "leopard" | "anderson") and outcome
// derived from whether fn returned an error.
func ObserveCarrierCall(carrier string, fn func() error) error {
	start := time.Now()
	err := fn()
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	CarrierCallDuration.WithLabelValues(carrier, outcome).Observe(time.Since(start).Seconds())
	return err
}

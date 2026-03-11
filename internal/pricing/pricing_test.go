package pricing

import (
	"testing"
	"time"
)

func TestGetEffectivePrice_NoDiscounts(t *testing.T) {
	s := NewServiceFromEnv()

	priceLite := s.GetEffectivePrice("lite", 1, MethodDirect)
	pricePremium := s.GetEffectivePrice("premium", 1, MethodDirect)

	if priceLite <= 0 || pricePremium <= 0 {
		t.Fatalf("expected baseline prices to be positive, got lite=%d premium=%d", priceLite, pricePremium)
	}
}

func TestGetEffectivePrice_GlobalDiscount(t *testing.T) {
	s := NewServiceFromEnv()
	fixedNow := time.Now()
	s.SetNowFunc(func() time.Time { return fixedNow })

	s.SetDiscount(ScopeGlobal, 20, time.Hour)

	priceLite := s.GetEffectivePrice("lite", 1, MethodDirect)
	pricePremium := s.GetEffectivePrice("premium", 1, MethodDirect)

	if priceLite <= 0 || pricePremium <= 0 {
		t.Fatalf("expected discounted prices to be positive, got lite=%d premium=%d", priceLite, pricePremium)
	}
}

func TestGetEffectivePrice_ScopePriorityAndHighestPercent(t *testing.T) {
	s := NewServiceFromEnv()
	fixedNow := time.Now()
	s.SetNowFunc(func() time.Time { return fixedNow })

	// Smaller global discount and larger lite-only discount; lite should see the larger one.
	s.SetDiscount(ScopeGlobal, 10, time.Hour)
	s.SetDiscount(ScopeLite, 30, time.Hour)

	litePrice := s.GetEffectivePrice("lite", 1, MethodDirect)
	premiumPrice := s.GetEffectivePrice("premium", 1, MethodDirect)

	if litePrice <= 0 || premiumPrice <= 0 {
		t.Fatalf("expected positive prices, got lite=%d premium=%d", litePrice, premiumPrice)
	}
	if litePrice >= premiumPrice {
		t.Fatalf("expected lite price (%d) to be lower than premium price (%d) with larger discount", litePrice, premiumPrice)
	}
}

func TestGetEffectivePrice_ExpiredDiscountIgnored(t *testing.T) {
	s := NewServiceFromEnv()
	start := time.Now()
	now := start
	s.SetNowFunc(func() time.Time { return now })

	s.SetDiscount(ScopeGlobal, 50, time.Hour)
	priceWithDiscount := s.GetEffectivePrice("lite", 1, MethodDirect)

	// Move time beyond expiry.
	now = start.Add(2 * time.Hour)
	priceAfterExpiry := s.GetEffectivePrice("lite", 1, MethodDirect)

	if priceAfterExpiry <= 0 {
		t.Fatalf("expected positive price after expiry, got %d", priceAfterExpiry)
	}
	if priceAfterExpiry == priceWithDiscount {
		t.Fatalf("expected price after expiry (%d) to differ from discounted price (%d)", priceAfterExpiry, priceWithDiscount)
	}
}


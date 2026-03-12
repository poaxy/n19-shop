package pricing

import (
	"os"
	"strconv"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/config"
)

// Scope represents which plans a discount applies to.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeLite    Scope = "lite"
	ScopePremium Scope = "premium"
)

// Method represents the payment method used for a purchase.
type Method string

const (
	// MethodDirect covers non-Stars, non-Stripe payments that use the RUB prices
	// from PRICE_* (e.g. CryptoPay, YooKassa, Tribute, generic direct payments).
	MethodDirect Method = "direct"
	// MethodStars covers Telegram Stars payments that use STARS_PRICE_*.
	MethodStars Method = "stars"
	// MethodStripe covers Stripe payments that use STRIPE_PRICE_*.
	MethodStripe Method = "stripe"
)

// Discount describes a single time-limited percentage discount.
type Discount struct {
	Scope     Scope
	Percent   int
	ExpiresAt time.Time
}

// Service provides effective prices taking runtime discounts into account.
type Service struct {
	mu        sync.RWMutex
	discounts map[Scope]Discount
	now       func() time.Time
}

// NewServiceFromEnv creates a new pricing service.
// Baseline prices are always read from config (which is initialised from env).
func NewServiceFromEnv() *Service {
	return &Service{
		discounts: make(map[Scope]Discount),
		now:       time.Now,
	}
}

// SetNowFunc allows tests to inject a custom clock.
func (s *Service) SetNowFunc(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now == nil {
		s.now = time.Now
		return
	}
	s.now = now
}

// SetDiscount configures or replaces a discount for the given scope.
// duration defines how long the discount remains active from now.
func (s *Service) SetDiscount(scope Scope, percent int, duration time.Duration) {
	if percent <= 0 || percent >= 100 {
		// Silently ignore invalid percentages to avoid accidental 0 or 100% discounts.
		return
	}
	if duration <= 0 {
		return
	}

	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.discounts == nil {
		s.discounts = make(map[Scope]Discount)
	}

	s.discounts[scope] = Discount{
		Scope:     scope,
		Percent:   percent,
		ExpiresAt: now.Add(duration),
	}
}

// FlushDiscounts removes all configured time-limited discounts.
func (s *Service) FlushDiscounts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discounts = make(map[Scope]Discount)
}

// ActiveDiscounts returns a snapshot of currently active discounts.
func (s *Service) ActiveDiscounts() []Discount {
	now := s.now()

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Discount
	for _, d := range s.discounts {
		if d.ExpiresAt.After(now) {
			out = append(out, d)
		}
	}
	return out
}

// GetEffectivePrice returns the baseline price for the given tier, duration (in months)
// and payment method, with the best applicable active discount applied.
//
// The returned value is:
//   - RUB units for MethodDirect (PRICE_* / overrides)
//   - Stars units for MethodStars (STARS_PRICE_*)
//   - USD cents for MethodStripe (STRIPE_PRICE_*)
func (s *Service) GetEffectivePrice(tier string, months int, method Method) int {
	baseline := baselinePrice(tier, months, method)
	if baseline <= 0 {
		return baseline
	}

	percent := s.effectiveDiscountPercent(tier)
	if percent <= 0 {
		return baseline
	}

	// Integer arithmetic with floor: baseline * (100 - percent) / 100.
	return (baseline * (100 - percent)) / 100
}

// baselinePrice reads the baseline price for the given tier, duration and method.
func baselinePrice(tier string, months int, method Method) int {
	switch method {
	case MethodStars:
		return starsPriceForTier(tier, months)
	case MethodStripe:
		// Stripe should mirror the same base pricing as direct payments.
		// We reuse the direct per-plan price here so configuring direct pricing
		// automatically affects Stripe as well (before discounts).
		return directPriceForTier(tier, months)
	default:
		return directPriceForTier(tier, months)
	}
}

// directPriceForTier returns the direct (RUB) price for the given tier and duration.
// By default it uses PRICE_* from config, but it can be overridden per tier via:
//
//   - LITE_PRICE_1, LITE_PRICE_3, LITE_PRICE_6, LITE_PRICE_12
//   - PREMIUM_PRICE_1, PREMIUM_PRICE_3, PREMIUM_PRICE_6, PREMIUM_PRICE_12
//
// If an override is missing or invalid, it falls back to the shared PRICE_* value.
func directPriceForTier(tier string, months int) int {
	base := config.Price(months)

	var prefix string
	switch tier {
	case "lite":
		prefix = "LITE_PRICE_"
	case "premium":
		prefix = "PREMIUM_PRICE_"
	default:
		return base
	}

	key := prefix + strconv.Itoa(months)
	v := os.Getenv(key)
	if v == "" {
		return base
	}

	override, err := strconv.Atoi(v)
	if err != nil {
		return base
	}
	return override
}

// starsPriceForTier returns the Telegram Stars price for the given tier and duration.
// By default it uses STARS_PRICE_* from config, but it can be overridden per tier via:
//
//   - LITE_STARS_PRICE_1, LITE_STARS_PRICE_3, LITE_STARS_PRICE_6, LITE_STARS_PRICE_12
//   - PREMIUM_STARS_PRICE_1, PREMIUM_STARS_PRICE_3, PREMIUM_STARS_PRICE_6, PREMIUM_STARS_PRICE_12
//
// If an override is missing or invalid, it falls back to the shared STARS_PRICE_* value.
func starsPriceForTier(tier string, months int) int {
	base := config.StarsPrice(months)

	var prefix string
	switch tier {
	case "lite":
		prefix = "LITE_STARS_PRICE_"
	case "premium":
		prefix = "PREMIUM_STARS_PRICE_"
	default:
		return base
	}

	key := prefix + strconv.Itoa(months)
	v := os.Getenv(key)
	if v == "" {
		return base
	}

	override, err := strconv.Atoi(v)
	if err != nil {
		return base
	}
	return override
}

// BaselinePriceForPreview is a helper used by admin UI to show the
// before/after prices in summaries, without applying any discounts.
func BaselinePriceForPreview(tier string, months int, method Method) int {
	return baselinePrice(tier, months, method)
}

// effectiveDiscountPercent returns the highest active discount percentage applicable
// to the given tier, considering both Global and tier-specific discounts.
func (s *Service) effectiveDiscountPercent(tier string) int {
	now := s.now()

	s.mu.RLock()
	defer s.mu.RUnlock()

	maxPercent := 0
	for _, d := range s.discounts {
		if !d.ExpiresAt.After(now) {
			continue
		}
		if d.Scope == ScopeGlobal || matchesTierScope(d.Scope, tier) {
			if d.Percent > maxPercent {
				maxPercent = d.Percent
			}
		}
	}
	return maxPercent
}

func matchesTierScope(scope Scope, tier string) bool {
	switch scope {
	case ScopeLite:
		return tier == "lite"
	case ScopePremium:
		return tier == "premium"
	default:
		return false
	}
}


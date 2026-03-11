package pricing

import (
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
//   - RUB units for MethodDirect (config.Price)
//   - Stars units for MethodStars (config.StarsPrice)
//   - USD cents for MethodStripe (config.StripePrice)
func (s *Service) GetEffectivePrice(tier string, months int, method Method) int {
	baseline := baselinePrice(months, method)
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

// baselinePrice reads the baseline price for the given duration and method from config.
func baselinePrice(months int, method Method) int {
	switch method {
	case MethodStars:
		return config.StarsPrice(months)
	case MethodStripe:
		return config.StripePrice(months)
	default:
		return config.Price(months)
	}
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


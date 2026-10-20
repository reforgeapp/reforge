package budget

import (
	"math"
	"math/big"
	"time"

	"github.com/reforgeapp/reforge/pkg/auth"
)

func values(a Amount) []int64 {
	return []int64{a.MicroUSD, a.Tokens, a.Milliseconds, a.Requests, a.Concurrency}
}
func capValues(c Caps) []*int64 {
	return []*int64{c.MicroUSD, c.Tokens, c.Milliseconds, c.Requests, c.Concurrency}
}
func validAmount(a Amount) bool {
	for _, n := range values(a) {
		if n < 0 {
			return false
		}
	}
	return true
}
func excess(a, b Amount) Amount {
	x, y := values(a), values(b)
	for i := range x {
		if x[i] > y[i] {
			x[i] -= y[i]
		} else {
			x[i] = 0
		}
	}
	return Amount{x[0], x[1], x[2], x[3], x[4]}
}
func validCaps(c Caps) bool {
	any := false
	for _, n := range capValues(c) {
		if n != nil {
			any = true
			if *n < 0 {
				return false
			}
		}
	}
	return any
}
func plus(a, b Amount) (Amount, error) {
	x, y := values(a), values(b)
	for i := range x {
		if y[i] < 0 || x[i] < 0 || x[i] > math.MaxInt64-y[i] {
			return Amount{}, ErrInvalid
		}
		x[i] += y[i]
	}
	return Amount{x[0], x[1], x[2], x[3], x[4]}, nil
}
func minus(a, b Amount) (Amount, error) {
	x, y := values(a), values(b)
	for i := range x {
		if y[i] < 0 || x[i] < y[i] {
			return Amount{}, ErrInvalid
		}
		x[i] -= y[i]
	}
	return Amount{x[0], x[1], x[2], x[3], x[4]}, nil
}
func within(a Amount, c Caps) bool {
	for i, n := range capValues(c) {
		if n != nil && values(a)[i] > *n {
			return false
		}
	}
	return true
}
func quotaCaps(c Caps) bool {
	return c.Tokens != nil && c.Milliseconds != nil && c.Requests != nil && c.Concurrency != nil
}
func periodStart(l Limit, now time.Time) (time.Time, error) {
	now = now.UTC()
	switch l.Period {
	case "daily":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	case "monthly":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	case "custom":
		if now.Before(l.Start) || !now.Before(l.End) {
			return time.Time{}, ErrCapacity
		}
		return l.Start, nil
	default:
		return time.Time{}, ErrInvalid
	}
}
func validRoute(r Route) bool {
	return auth.ValidID(r.ConnectionID) && r.Model != "" && len(r.Model) <= 200 && r.Name != "" && len(r.Name) <= 200 && (r.Mode == "priced" || r.Mode == "quota") && (r.Mode != "priced" || r.PricingVersion != "" && r.MaxRequests == 1) && len(r.PricingVersion) <= 200 && r.InputMicroUSDPerMillion >= 0 && r.OutputMicroUSDPerMillion >= 0 && r.RequestMicroUSD >= 0 && r.MaxInputTokens > 0 && r.MaxOutputTokens > 0 && r.MaxMilliseconds > 0 && r.MaxRequests > 0 && (r.Mode != "quota" || (r.InputMicroUSDPerMillion == 0 && r.OutputMicroUSDPerMillion == 0 && r.RequestMicroUSD == 0))
}
func estimate(r Route, q Quote) (Amount, error) {
	if !validRoute(r) || q.InputTokens < 0 || q.MaxOutputTokens <= 0 || q.MaxMilliseconds <= 0 || q.MaxRequests <= 0 || q.InputTokens > r.MaxInputTokens || q.MaxOutputTokens > r.MaxOutputTokens || q.MaxMilliseconds > r.MaxMilliseconds || q.MaxRequests > r.MaxRequests {
		return Amount{}, ErrInvalid
	}
	if r.Paused || r.Mode == "quota" && (!r.Qualified || r.QualificationRef == "") {
		return Amount{}, ErrRevoked
	}
	tokens, err := plus(Amount{Tokens: q.InputTokens}, Amount{Tokens: q.MaxOutputTokens})
	if err != nil {
		return Amount{}, err
	}
	amount := Amount{Tokens: tokens.Tokens, Milliseconds: q.MaxMilliseconds, Requests: q.MaxRequests, Concurrency: 1}
	if r.Mode == "priced" {
		n := new(big.Int).Mul(big.NewInt(q.InputTokens), big.NewInt(r.InputMicroUSDPerMillion))
		n.Add(n, new(big.Int).Mul(big.NewInt(q.MaxOutputTokens), big.NewInt(r.OutputMicroUSDPerMillion)))
		n.Add(n, big.NewInt(999999))
		n.Div(n, big.NewInt(1000000))
		n.Add(n, new(big.Int).Mul(big.NewInt(q.MaxRequests), big.NewInt(r.RequestMicroUSD)))
		if !n.IsInt64() {
			return Amount{}, ErrInvalid
		}
		amount.MicroUSD = n.Int64()
	}
	return amount, nil
}

func ObservedAmount(route Route, input, output, milliseconds int64) (Amount, error) {
	if input < 0 || output < 0 || milliseconds < 0 {
		return Amount{}, ErrInvalid
	}
	r := route
	r.MaxInputTokens = max(r.MaxInputTokens, input)
	r.MaxOutputTokens = max(r.MaxOutputTokens, output, 1)
	r.MaxMilliseconds = max(r.MaxMilliseconds, milliseconds, 1)
	amount, err := estimate(r, Quote{InputTokens: input, MaxOutputTokens: max(output, 1), MaxMilliseconds: max(milliseconds, 1), MaxRequests: 1})
	if output == 0 && err == nil {
		n := new(big.Int).Mul(big.NewInt(input), big.NewInt(r.InputMicroUSDPerMillion))
		n.Add(n, big.NewInt(999999))
		n.Div(n, big.NewInt(1000000))
		n.Add(n, big.NewInt(r.RequestMicroUSD))
		if !n.IsInt64() {
			return Amount{}, ErrInvalid
		}
		amount.MicroUSD = n.Int64()
		amount.Tokens = input
	}
	amount.Milliseconds = milliseconds
	amount.Concurrency = 0
	return amount, err
}

package domain

// PlanRank defines ordering for plan upgrades/downgrades.
// Higher number means higher tier.
func PlanRank(plan string) int {
	switch plan {
	case "free":
		return 1
	case "lite":
		return 2
	case "premium":
		return 3
	default:
		return 0
	}
}


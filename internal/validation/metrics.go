package validation

// Recall computes |expected ∩ actual| / |expected|.
// Returns 1.0 if expected is empty.
func Recall(expected, actual []string) float64 {
	if len(expected) == 0 {
		return 1.0
	}
	set := toSet(actual)
	hits := 0
	for _, e := range expected {
		if set[e] {
			hits++
		}
	}
	return float64(hits) / float64(len(expected))
}

// Precision computes |expected ∩ actual| / |actual|.
// Returns 1.0 if actual is empty.
func Precision(expected, actual []string) float64 {
	if len(actual) == 0 {
		return 1.0
	}
	set := toSet(expected)
	hits := 0
	for _, a := range actual {
		if set[a] {
			hits++
		}
	}
	return float64(hits) / float64(len(actual))
}

// Missing returns elements in expected that are not in actual.
func Missing(expected, actual []string) []string {
	set := toSet(actual)
	var out []string
	for _, e := range expected {
		if !set[e] {
			out = append(out, e)
		}
	}
	return out
}

// Extra returns elements in actual that are not in expected.
func Extra(expected, actual []string) []string {
	set := toSet(expected)
	var out []string
	for _, a := range actual {
		if !set[a] {
			out = append(out, a)
		}
	}
	return out
}

// ContainsAll returns true if actual contains every element in required.
func ContainsAll(required, actual []string) bool {
	set := toSet(actual)
	for _, r := range required {
		if !set[r] {
			return false
		}
	}
	return true
}

// ContainsAny returns true if actual contains at least one element from items.
func ContainsAny(items, actual []string) bool {
	set := toSet(actual)
	for _, i := range items {
		if set[i] {
			return true
		}
	}
	return false
}

func toSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}

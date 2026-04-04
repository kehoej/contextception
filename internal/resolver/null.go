package resolver

import "github.com/kehoej/contextception/internal/model"

// NullResolver marks all imports as unresolved. Used for languages that don't
// have a resolver implementation yet (e.g., TypeScript before the TS resolver is built).
type NullResolver struct{}

func (r *NullResolver) Resolve(_ string, fact model.ImportFact, _ string) (model.ResolveResult, error) {
	return model.ResolveResult{
		External: true,
		Reason:   "no_resolver",
	}, nil
}

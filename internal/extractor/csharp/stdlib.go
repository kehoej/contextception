package csharp

// stdlibPrefixes lists top-level C# standard library and framework namespace prefixes.
// Imports starting with these prefixes are classified as external/stdlib.
var stdlibPrefixes = []string{
	"System.",
	"Microsoft.",
	"Windows.",
}

// IsStdlib returns true if the import specifier refers to a C# stdlib/framework namespace.
func IsStdlib(spec string) bool {
	for _, prefix := range stdlibPrefixes {
		if len(spec) >= len(prefix) && spec[:len(prefix)] == prefix {
			return true
		}
	}
	// Exact matches (e.g., "System" without a dot suffix).
	return spec == "System" || spec == "Microsoft" || spec == "Windows"
}

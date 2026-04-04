package rust

// stdlibCrates lists Rust standard library crate prefixes.
var stdlibCrates = []string{
	"std::",
	"core::",
	"alloc::",
}

// IsStdlib returns true if the use path refers to a Rust stdlib crate.
func IsStdlib(spec string) bool {
	for _, prefix := range stdlibCrates {
		if len(spec) >= len(prefix) && spec[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

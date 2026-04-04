package java

// stdlibPrefixes lists top-level Java standard library packages.
// Imports starting with these prefixes are classified as external/stdlib.
var stdlibPrefixes = []string{
	"java.",
	"javax.",
	"sun.",
	"jdk.",
	"com.sun.",
	"org.w3c.",
	"org.xml.",
	"org.ietf.",
}

// IsStdlib returns true if the import specifier refers to a Java stdlib package.
func IsStdlib(spec string) bool {
	for _, prefix := range stdlibPrefixes {
		if len(spec) >= len(prefix) && spec[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

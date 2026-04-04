// Package version holds the build version, set via ldflags.
// It lives in its own package to avoid import cycles between cli and mcpserver.
package version

// Version is set at build time via ldflags, or defaults to "dev".
var Version = "dev"

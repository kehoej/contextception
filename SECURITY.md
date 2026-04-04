# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Contextception, please report it responsibly using [GitHub's private vulnerability reporting](https://github.com/kehoej/contextception/security/advisories/new).

**Do not open a public issue for security vulnerabilities.**

### What to include

- Description of the vulnerability
- Steps to reproduce
- Impact assessment (what an attacker could do)
- Suggested fix (if you have one)

### Response timeline

- **Acknowledgement:** Within 48 hours
- **Assessment:** Within 1 week
- **Fix:** Depends on severity; critical issues will be prioritized

## Scope

Contextception is a read-only static analysis tool. It never modifies source code, executes arbitrary commands, or makes network requests. The primary attack surface is:

- **Malicious repository content:** Crafted source files that could cause excessive memory usage or crashes during parsing
- **SQLite index:** The local `.contextception/` database could theoretically be tampered with, though it is regenerated on `reindex`
- **MCP server:** The stdio-based MCP server processes JSON-RPC requests; malformed requests should be handled gracefully

## Supported Versions

| Version | Supported |
|---------|-----------|
| 1.x     | Yes       |
| < 1.0   | No        |

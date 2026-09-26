module github.com/shaktsin/umcode

go 1.24.0

toolchain go1.24.7

require (
	github.com/coder/websocket v1.8.15
	github.com/goccy/go-yaml v1.19.2
	github.com/ncruces/go-sqlite3 v0.32.0
)

require (
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

// golang.org/x modules are fetched from their GitHub mirrors so the build does
// not depend on the golang.org vanity host. Safe to remove where golang.org is reachable.
replace (
	golang.org/x/crypto => github.com/golang/crypto v0.48.0
	golang.org/x/sync => github.com/golang/sync v0.19.0
	golang.org/x/sys => github.com/golang/sys v0.41.0
	golang.org/x/text => github.com/golang/text v0.34.0
)

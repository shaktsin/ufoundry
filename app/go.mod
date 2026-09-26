module github.com/shaktsin/umcode/app

go 1.25.0

require (
	github.com/shaktsin/umcode v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0-beta.23
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.46.0 // indirect
)

replace github.com/shaktsin/umcode => ../

replace golang.org/x/sys => github.com/golang/sys v0.46.0

replace golang.org/x/term => github.com/golang/term v0.44.0

replace golang.org/x/text => github.com/golang/text v0.31.0

replace golang.org/x/tools => github.com/golang/tools v0.47.0

replace golang.org/x/net => github.com/golang/net v0.48.0

replace golang.org/x/crypto => github.com/golang/crypto v0.45.0

replace golang.org/x/mod => github.com/golang/mod v0.30.0

replace golang.org/x/sync => github.com/golang/sync v0.19.0

replace golang.org/x/exp => github.com/golang/exp v0.0.0-20260115194200-52b59b81b0cf

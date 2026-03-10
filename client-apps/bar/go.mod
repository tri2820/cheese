module github.com/tri2820/cheese/client-apps/bar

go 1.23

require (
	github.com/tri2820/cheese/apps/common v1.0.0
	github.com/tri2820/cheese/client-toolkit v1.0.0
	github.com/tri2820/cheese/protocols v1.0.0
	golang.org/x/image v0.24.0
)

require (
	github.com/jfreymuth/pulse v0.1.1 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)

replace github.com/tri2820/cheese/apps/common => ../../apps/common

replace github.com/tri2820/cheese/client-toolkit => ../../client-toolkit

replace github.com/tri2820/cheese/protocols => ../../protocols

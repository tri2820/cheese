module github.com/tri2820/cheese/client-toolkit

go 1.23

require (
	github.com/tri2820/cheese/protocols v1.0.0
	golang.org/x/image v0.23.0
	golang.org/x/sys v0.29.0
)

require golang.org/x/text v0.21.0 // indirect

replace github.com/tri2820/cheese/protocols => ../protocols

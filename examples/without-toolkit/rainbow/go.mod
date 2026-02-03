module github.com/tri2820/cheese/examples/rainbow

go 1.23

require (
	github.com/tri2820/cheese/protocols v1.0.0
	golang.org/x/sys v0.29.0
)

replace github.com/tri2820/cheese/protocols => ../../../protocols

replace github.com/tri2820/cheese/client-toolkit => ../../../client-toolkit

module github.com/tri2820/cheese/examples/with-toolkit/statusbar

go 1.23

require (
	github.com/tri2820/cheese/client-toolkit v1.0.0
	github.com/tri2820/cheese/protocols v1.0.0
)

require golang.org/x/sys v0.29.0 // indirect

replace github.com/tri2820/cheese/protocols => ../../../protocols

replace github.com/tri2820/cheese/client-toolkit => ../../../client-toolkit

module github.com/tri2820/cheese/ui

go 1.23

require github.com/lithdew/casso v0.0.0-20200531104607-fe75aa82181f

require github.com/tri2820/cheese/signals v0.0.0-00010101000000-000000000000

require github.com/tri2820/cheese/client-toolkit v0.0.0-00010101000000-000000000000

require (
	github.com/tri2820/cheese/protocols v1.0.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace github.com/tri2820/cheese/signals => ../signals

replace github.com/tri2820/cheese/client-toolkit => ../client-toolkit

replace github.com/tri2820/cheese/protocols => ../protocols

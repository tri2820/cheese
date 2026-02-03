module vk-dmabuf-example

go 1.23

require (
	github.com/tri2820/cheese/client-toolkit v1.0.0
	github.com/vulkan-go/vulkan v0.0.0-20221209234627-c0a353ae26c8
)

require (
	github.com/tri2820/cheese/protocols v1.0.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace github.com/tri2820/cheese/protocols => ../../../protocols

replace github.com/tri2820/cheese/client-toolkit => ../../../client-toolkit

// Package client provides the core Wayland client protocol runtime
package client

// BaseProxy is the base type for all Wayland objects
type BaseProxy struct {
	id  uint32
	ctx *Context
}

// ID returns the object ID
func (p *BaseProxy) ID() uint32 {
	return p.id
}

// Context returns the associated context
func (p *BaseProxy) Context() *Context {
	return p.ctx
}

// SetID sets the object ID
func (p *BaseProxy) SetID(id uint32) {
	p.id = id
}

// SetContext sets the associated context
func (p *BaseProxy) SetContext(ctx *Context) {
	p.ctx = ctx
}

// Proxy is the interface implemented by all Wayland objects
type Proxy interface {
	ID() uint32
	Context() *Context
}

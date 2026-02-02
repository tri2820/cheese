package client

import (
	"fmt"
	"net"
	"sync"
)

// Context manages a Wayland connection
type Context struct {
	conn       *net.UnixConn
	mu         sync.Mutex
	nextID     uint32
	objects    map[uint32]Proxy
	dispatcher map[uint32]func([]byte)
}

// NewContext creates a new Wayland context
func NewContext(conn *net.UnixConn) *Context {
	return &Context{
		conn:       conn,
		nextID:     1, // ID 1 is reserved for wl_display
		objects:    make(map[uint32]Proxy),
		dispatcher: make(map[uint32]func([]byte)),
	}
}

// Register registers a proxy and assigns it an ID
func (c *Context) Register(p Proxy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Assign ID
	if bp, ok := p.(*BaseProxy); ok {
		bp.id = c.nextID
		bp.ctx = c
		c.nextID++
	}

	c.objects[p.ID()] = p
}

// Unregister removes a proxy
func (c *Context) Unregister(p Proxy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.objects, p.ID())
	delete(c.dispatcher, p.ID())
}

// GetProxy retrieves a proxy by ID
func (c *Context) GetProxy(id uint32) Proxy {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.objects[id]
}

// WriteMsg sends a message to the compositor
func (c *Context) WriteMsg(data []byte, fds []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, oobn, err := c.conn.WriteMsgUnix(data, fds, nil)
	if err != nil {
		return err
	}
	if n != len(data) || oobn != len(fds) {
		return fmt.Errorf("incomplete write: wrote %d/%d bytes, %d/%d oob", n, len(data), oobn, len(fds))
	}

	return nil
}

// Dispatch reads and processes one event from the compositor
func (c *Context) Dispatch() error {
	buf := make([]byte, 4096)
	oob := make([]byte, 256)

	n, oobn, _, _, err := c.conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return err
	}

	_ = oobn // TODO: Handle file descriptors in out-of-band data

	// Parse message (simplified)
	if n < 8 {
		return fmt.Errorf("message too short: %d bytes", n)
	}

	// objectID := binary.LittleEndian.Uint32(buf[0:4])
	// opcode := binary.LittleEndian.Uint32(buf[4:8]) & 0xFFFF

	// TODO: Dispatch to appropriate handler
	// For now, just consume the message

	return nil
}

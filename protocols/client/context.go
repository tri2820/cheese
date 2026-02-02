package client

import (
	"fmt"
	"net"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// Context manages a Wayland connection
type Context struct {
	conn       *net.UnixConn
	mu         sync.Mutex
	nextID     uint32
	objects    map[uint32]Proxy
	dispatcher map[uint32]func([]byte)
}

var oobSpace = unix.CmsgSpace(4)

// NewContext creates a new Wayland context
func NewContext(conn *net.UnixConn) *Context {
	return &Context{
		conn:       conn,
		nextID:     0, // Will start at 1 when first object (wl_display) is registered
		objects:    make(map[uint32]Proxy),
		dispatcher: make(map[uint32]func([]byte)),
	}
}

// Register registers a proxy and assigns it an ID
func (c *Context) Register(p Proxy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextID++

	// Set ID and context using the Proxy interface methods
	// Note: We need to check if p implements SetID/SetContext
	type proxySetter interface {
		SetID(uint32)
		SetContext(*Context)
	}

	if ps, ok := p.(proxySetter); ok {
		ps.SetID(c.nextID)
		ps.SetContext(c)
	}

	c.objects[c.nextID] = p
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
	if id == 0 {
		return nil
	}
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
	senderID, opcode, fd, data, err := c.ReadMsg()
	if err != nil {
		return fmt.Errorf("ctx.Dispatch: unable to read msg: %w", err)
	}

	sender, ok := c.objects[senderID]
	if !ok {
		return fmt.Errorf("ctx.Dispatch: unable to find sender (senderID=%d)", senderID)
	}

	dispatcher, ok := sender.(Dispatcher)
	if !ok {
		return fmt.Errorf("ctx.Dispatch: sender doesn't implement Dispatch method (senderID=%d)", senderID)
	}

	dispatcher.Dispatch(opcode, fd, data)
	return nil
}

// ReadMsg reads and parses a single Wayland protocol message
func (c *Context) ReadMsg() (senderID uint32, opcode uint32, fd int, data []byte, err error) {
	fd = -1

	oob := make([]byte, oobSpace)
	header := make([]byte, 8)

	n, oobn, _, _, err := c.conn.ReadMsgUnix(header, oob)
	if err != nil {
		return senderID, opcode, fd, data, err
	}
	if n != 8 {
		return senderID, opcode, fd, data, fmt.Errorf("ctx.ReadMsg: incorrect number of bytes read for header (n=%d)", n)
	}

	if oobn > 0 {
		fds, err := getFdsFromOob(oob, oobn, "header")
		if err != nil {
			return senderID, opcode, fd, data, fmt.Errorf("ctx.ReadMsg: %w", err)
		}

		if len(fds) > 0 {
			fd = fds[0]
		}
	}

	senderID = Uint32(header[:4])
	opcodeAndSize := Uint32(header[4:8])
	opcode = opcodeAndSize & 0xffff
	size := opcodeAndSize >> 16

	msgSize := int(size) - 8
	if msgSize == 0 {
		return senderID, opcode, fd, nil, nil
	}

	data = make([]byte, msgSize)

	if fd == -1 {
		// if something was read before, then zero it out
		if oobn > 0 {
			oob = make([]byte, oobSpace)
		}

		n, oobn, _, _, err = c.conn.ReadMsgUnix(data, oob)
	} else {
		n, err = c.conn.Read(data)
	}
	if err != nil {
		return senderID, opcode, fd, data, fmt.Errorf("ctx.ReadMsg: %w", err)
	}
	if n != msgSize {
		return senderID, opcode, fd, data, fmt.Errorf("ctx.ReadMsg: incorrect number of bytes read for data (n=%d, msgSize=%d)", n, msgSize)
	}

	if fd == -1 && oobn > 0 {
		fds, err := getFdsFromOob(oob, oobn, "data")
		if err != nil {
			return senderID, opcode, fd, data, fmt.Errorf("ctx.ReadMsg: %w", err)
		}

		if len(fds) > 0 {
			fd = fds[0]
		}
	}

	return senderID, opcode, fd, data, nil
}

// getFdsFromOob extracts file descriptors from out-of-band data
func getFdsFromOob(oob []byte, oobn int, source string) ([]int, error) {
	if oobn > len(oob) {
		return nil, fmt.Errorf("getFdsFromOob: incorrect number of bytes read from %s for oob (oobn=%d)", source, oobn)
	}
	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, fmt.Errorf("getFdsFromOob: unable to parse control message from %s: %w", source, err)
	}

	var fdsRet []int
	for _, scm := range scms {
		fds, err := unix.ParseUnixRights(&scm)
		if err != nil {
			return nil, fmt.Errorf("getFdsFromOob: unable to parse unix rights from %s: %w", source, err)
		}

		fdsRet = append(fdsRet, fds...)
	}

	return fdsRet, nil
}

// Connect establishes a connection to the Wayland compositor
func Connect(addr string) (*WlDisplay, error) {
	if addr == "" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			return nil, fmt.Errorf("env XDG_RUNTIME_DIR not set")
		}
		addr = os.Getenv("WAYLAND_DISPLAY")
		if addr == "" {
			addr = "wayland-0"
		}
		addr = runtimeDir + "/" + addr
	}

	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: addr, Net: "unix"})
	if err != nil {
		return nil, err
	}

	ctx := NewContext(conn)

	// Create and register the display object (will get ID 1)
	return NewWlDisplay(ctx), nil
}

// Close closes the connection to the compositor
func (c *Context) Close() error {
	return c.conn.Close()
}

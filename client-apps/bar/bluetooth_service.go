package main

import (
	"log"
	"os/exec"
	"sort"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	bluezService            = "org.bluez"
	bluezRootPath           = dbus.ObjectPath("/")
	bluezPropsIface         = "org.freedesktop.DBus.Properties"
	bluezObjectManagerIface = "org.freedesktop.DBus.ObjectManager"
	bluezAdapterIface       = "org.bluez.Adapter1"
	bluezDeviceIface        = "org.bluez.Device1"
)

type BluetoothDevice struct {
	Path      dbus.ObjectPath
	Address   string
	Name      string
	Icon      string
	Connected bool
	Paired    bool
}

type BluetoothState struct {
	Powered   bool
	Connected bool
	Name      string
}

type BluetoothService struct {
	mu          sync.RWMutex
	state       BluetoothState
	subscribers map[int]func(BluetoothState)
	nextSubID   int

	conn    *dbus.Conn
	signals chan *dbus.Signal
	done    chan struct{}
}

func NewBluetoothService() *BluetoothService {
	s := &BluetoothService{
		subscribers: make(map[int]func(BluetoothState)),
		signals:     make(chan *dbus.Signal, 32),
		done:        make(chan struct{}),
	}

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		log.Printf("Bluetooth service connect error: %v", err)
		return s
	}
	s.conn = conn
	s.conn.Signal(s.signals)

	for _, iface := range []string{bluezAdapterIface, bluezDeviceIface} {
		if err := s.conn.AddMatchSignal(
			dbus.WithMatchInterface(bluezPropsIface),
			dbus.WithMatchArg(0, iface),
		); err != nil {
			log.Printf("Bluetooth service match error for %s: %v", iface, err)
		}
	}
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchInterface(bluezObjectManagerIface),
	); err != nil {
		log.Printf("Bluetooth service object-manager match error: %v", err)
	}

	go s.run()
	return s
}

func (s *BluetoothService) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}

	if s.conn != nil {
		s.conn.RemoveSignal(s.signals)
		_ = s.conn.Close()
	}
}

func (s *BluetoothService) ListDevices() ([]BluetoothDevice, bool) {
	if s.conn == nil {
		return nil, false
	}
	obj := s.conn.Object(bluezService, bluezRootPath)
	var managed map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := obj.Call(bluezObjectManagerIface+".GetManagedObjects", 0).Store(&managed); err != nil {
		return nil, false
	}

	powered := false
	for _, ifaces := range managed {
		if props, ok := ifaces[bluezAdapterIface]; ok {
			if p, ok := variantBool(props["Powered"]); ok && p {
				powered = true
				break
			}
		}
	}
	if !powered {
		return nil, false
	}

	var devices []BluetoothDevice
	for path, ifaces := range managed {
		props, ok := ifaces[bluezDeviceIface]
		if !ok {
			continue
		}
		addr, _ := variantString(props["Address"])
		name, _ := variantString(props["Name"])
		if name == "" {
			name, _ = variantString(props["Alias"])
		}
		if name == "" {
			name = addr
		}
		icon, _ := variantString(props["Icon"])
		conn, _ := variantBool(props["Connected"])
		paired, _ := variantBool(props["Paired"])

		devices = append(devices, BluetoothDevice{
			Path:      path,
			Address:   addr,
			Name:      name,
			Icon:      icon,
			Connected: conn,
			Paired:    paired,
		})
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Connected != devices[j].Connected {
			return devices[i].Connected
		}
		if devices[i].Paired != devices[j].Paired {
			return devices[i].Paired
		}
		return devices[i].Name < devices[j].Name
	})

	return devices, true
}

func (s *BluetoothService) SetPowered(powered bool) error {
	arg := "on"
	if !powered {
		arg = "off"
	}
	go func() {
		_ = exec.Command("bluetoothctl", "power", arg).Run()
		s.refresh()
	}()
	return nil
}

func (s *BluetoothService) ToggleDevice(dev BluetoothDevice) {
	go func() {
		if dev.Connected {
			_ = exec.Command("bluetoothctl", "disconnect", dev.Address).Run()
		} else {
			_ = exec.Command("bluetoothctl", "connect", dev.Address).Run()
		}
	}()
}

func (s *BluetoothService) adapterPath() dbus.ObjectPath {
	if s.conn == nil {
		return ""
	}
	obj := s.conn.Object(bluezService, bluezRootPath)
	var managed map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := obj.Call(bluezObjectManagerIface+".GetManagedObjects", 0).Store(&managed); err != nil {
		return ""
	}
	for path, ifaces := range managed {
		if _, ok := ifaces[bluezAdapterIface]; ok {
			return path
		}
	}
	return ""
}

// StartDiscovery tells BlueZ to scan for nearby devices so unpaired/connectable
// devices show up in the list.
func (s *BluetoothService) StartDiscovery() {
	go func() {
		adapter := s.adapterPath()
		if adapter == "" {
			return
		}
		_ = s.conn.Object(bluezService, adapter).Call(bluezAdapterIface+".StartDiscovery", 0).Err
	}()
}

func (s *BluetoothService) StopDiscovery() {
	go func() {
		adapter := s.adapterPath()
		if adapter == "" {
			return
		}
		_ = s.conn.Object(bluezService, adapter).Call(bluezAdapterIface+".StopDiscovery", 0).Err
	}()
}

func (s *BluetoothService) State() BluetoothState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *BluetoothService) Subscribe(fn func(BluetoothState)) func() {
	if fn == nil {
		return func() {}
	}

	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = fn
	state := s.state
	s.mu.Unlock()

	fn(state)

	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *BluetoothService) run() {
	s.refresh()
	for {
		select {
		case <-s.done:
			return
		case sig, ok := <-s.signals:
			if !ok {
				return
			}
			if sig == nil {
				continue
			}
			switch sig.Name {
			case bluezPropsIface + ".PropertiesChanged",
				bluezObjectManagerIface + ".InterfacesAdded",
				bluezObjectManagerIface + ".InterfacesRemoved":
				s.refresh()
			}
		}
	}
}

func (s *BluetoothService) refresh() {
	if s.conn == nil {
		return
	}

	next := readBluetoothState(s.conn)

	s.mu.Lock()
	if next == s.state {
		s.mu.Unlock()
		return
	}
	s.state = next
	subs := make([]func(BluetoothState), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	s.mu.Unlock()

	for _, fn := range subs {
		if fn != nil {
			fn(next)
		}
	}
}

func readBluetoothState(conn *dbus.Conn) BluetoothState {
	obj := conn.Object(bluezService, bluezRootPath)
	var managed map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := obj.Call(bluezObjectManagerIface+".GetManagedObjects", 0).Store(&managed); err != nil {
		log.Printf("Bluetooth managed objects error: %v", err)
		return BluetoothState{}
	}

	state := BluetoothState{}
	for _, ifaces := range managed {
		props, ok := ifaces[bluezAdapterIface]
		if !ok {
			continue
		}
		if powered, ok := variantBool(props["Powered"]); ok && powered {
			state.Powered = true
			break
		}
	}

	for _, ifaces := range managed {
		props, ok := ifaces[bluezDeviceIface]
		if !ok {
			continue
		}
		connected, ok := variantBool(props["Connected"])
		if !ok || !connected {
			continue
		}
		state.Connected = true
		if name, ok := variantString(props["Name"]); ok && name != "" {
			state.Name = name
		} else if alias, ok := variantString(props["Alias"]); ok {
			state.Name = alias
		}
		break
	}

	return state
}

func variantBool(v dbus.Variant) (bool, bool) {
	val, ok := v.Value().(bool)
	return val, ok
}

func variantString(v dbus.Variant) (string, bool) {
	val, ok := v.Value().(string)
	return val, ok
}

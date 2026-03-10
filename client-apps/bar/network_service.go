package main

import (
	"log"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	nmService                    = "org.freedesktop.NetworkManager"
	nmPath                       = dbus.ObjectPath("/org/freedesktop/NetworkManager")
	nmIface                      = "org.freedesktop.NetworkManager"
	nmPropsIface                 = "org.freedesktop.DBus.Properties"
	nmDeviceIface                = "org.freedesktop.NetworkManager.Device"
	nmWirelessDeviceIface        = "org.freedesktop.NetworkManager.Device.Wireless"
	nmAccessPointIface           = "org.freedesktop.NetworkManager.AccessPoint"
	nmDeviceTypeWifi      uint32 = 2
	nmDeviceStateActive   uint32 = 100
)

type WifiState struct {
	Connected bool
	SSID      string
	Strength  uint8
}

type NetworkService struct {
	mu          sync.RWMutex
	state       WifiState
	subscribers map[int]func(WifiState)
	nextSubID   int

	conn    *dbus.Conn
	signals chan *dbus.Signal
	done    chan struct{}
}

func NewNetworkService() *NetworkService {
	s := &NetworkService{
		subscribers: make(map[int]func(WifiState)),
		signals:     make(chan *dbus.Signal, 32),
		done:        make(chan struct{}),
	}

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		log.Printf("Network service connect error: %v", err)
		return s
	}
	s.conn = conn
	s.conn.Signal(s.signals)

	for _, iface := range []string{
		nmIface,
		nmDeviceIface,
		nmWirelessDeviceIface,
		nmAccessPointIface,
	} {
		if err := s.conn.AddMatchSignal(
			dbus.WithMatchInterface(nmPropsIface),
			dbus.WithMatchArg(0, iface),
		); err != nil {
			log.Printf("Network service match error for %s: %v", iface, err)
		}
	}

	s.refresh()
	go s.run()
	return s
}

func (s *NetworkService) Close() {
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

func (s *NetworkService) State() WifiState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *NetworkService) Subscribe(fn func(WifiState)) func() {
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

func (s *NetworkService) run() {
	for {
		select {
		case <-s.done:
			return
		case sig, ok := <-s.signals:
			if !ok {
				return
			}
			if sig == nil || sig.Name != nmPropsIface+".PropertiesChanged" {
				continue
			}
			s.refresh()
		}
	}
}

func (s *NetworkService) refresh() {
	if s.conn == nil {
		return
	}

	next := readWifiState(s.conn)

	s.mu.Lock()
	if next == s.state {
		s.mu.Unlock()
		return
	}
	s.state = next
	subs := make([]func(WifiState), 0, len(s.subscribers))
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

func readWifiState(conn *dbus.Conn) WifiState {
	obj := conn.Object(nmService, nmPath)
	var devicePaths []dbus.ObjectPath
	if err := obj.Call(nmIface+".GetDevices", 0).Store(&devicePaths); err != nil {
		log.Printf("Network devices error: %v", err)
		return WifiState{}
	}

	for _, path := range devicePaths {
		if path == "" || path == "/" {
			continue
		}

		device := conn.Object(nmService, path)
		deviceType, err := getUint32Property(device, nmDeviceIface, "DeviceType")
		if err != nil || deviceType != nmDeviceTypeWifi {
			continue
		}

		state, err := getUint32Property(device, nmDeviceIface, "State")
		if err != nil || state < nmDeviceStateActive {
			continue
		}

		apPath, err := getObjectPathProperty(device, nmWirelessDeviceIface, "ActiveAccessPoint")
		if err != nil || apPath == "" || apPath == "/" {
			continue
		}

		ap := conn.Object(nmService, apPath)
		ssidBytes, err := getBytesProperty(ap, nmAccessPointIface, "Ssid")
		if err != nil {
			log.Printf("Network SSID error: %v", err)
			continue
		}
		strength, err := getByteProperty(ap, nmAccessPointIface, "Strength")
		if err != nil {
			log.Printf("Network strength error: %v", err)
			continue
		}

		return WifiState{
			Connected: true,
			SSID:      string(ssidBytes),
			Strength:  strength,
		}
	}

	return WifiState{}
}

func getProperty(obj dbus.BusObject, iface, prop string) (dbus.Variant, error) {
	var value dbus.Variant
	err := obj.Call(nmPropsIface+".Get", 0, iface, prop).Store(&value)
	return value, err
}

func getUint32Property(obj dbus.BusObject, iface, prop string) (uint32, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return 0, err
	}
	return value.Value().(uint32), nil
}

func getObjectPathProperty(obj dbus.BusObject, iface, prop string) (dbus.ObjectPath, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return "", err
	}
	return value.Value().(dbus.ObjectPath), nil
}

func getBytesProperty(obj dbus.BusObject, iface, prop string) ([]byte, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return nil, err
	}
	return value.Value().([]byte), nil
}

func getByteProperty(obj dbus.BusObject, iface, prop string) (uint8, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return 0, err
	}
	return value.Value().(byte), nil
}

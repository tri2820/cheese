package main

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

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

type AccessPoint struct {
	SSID      string
	Strength  uint8
	Security  bool
	Connected bool
}

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

	conn            *dbus.Conn
	signals         chan *dbus.Signal
	done            chan struct{}
	wirelessEnabled bool
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

func (s *NetworkService) IsWirelessEnabled() bool {
	if s.conn == nil {
		return false
	}
	obj := s.conn.Object(nmService, nmPath)
	val, err := obj.GetProperty(nmIface + ".WirelessEnabled")
	if err != nil {
		return true
	}
	if b, ok := val.Value().(bool); ok {
		return b
	}
	return true
}

// WirelessEnabled returns the last known radio state without blocking on D-Bus.
func (s *NetworkService) WirelessEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.wirelessEnabled
}

func (s *NetworkService) SetWirelessEnabled(enabled bool) error {
	if s.conn == nil {
		return nil
	}
	obj := s.conn.Object(nmService, nmPath)
	go func() {
		_ = obj.SetProperty(nmIface+".WirelessEnabled", dbus.MakeVariant(enabled))
		arg := "on"
		if !enabled {
			arg = "off"
		}
		_ = exec.Command("nmcli", "radio", "wifi", arg).Run()
		s.refresh()
	}()
	return nil
}

func (s *NetworkService) ConnectToSSID(ap AccessPoint) {
	go func() {
		out, err := exec.Command("nmcli", "-t", "-f", "NAME", "connection", "show").Output()
		saved := false
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.TrimSpace(line) == ap.SSID {
					saved = true
					break
				}
			}
		}

		if saved {
			_ = exec.Command("nmcli", "connection", "up", "id", ap.SSID).Run()
		} else if !ap.Security {
			_ = exec.Command("nmcli", "dev", "wifi", "connect", ap.SSID).Run()
		} else {
			_ = exec.Command("ghostty", "--class=floating", "-e", "nmcli", "dev", "wifi", "connect", ap.SSID, "--ask").Start()
		}
		time.Sleep(500 * time.Millisecond)
		s.refresh()
	}()
}

func (s *NetworkService) ListAccessPoints() ([]AccessPoint, bool) {
	enabled := s.IsWirelessEnabled()
	if !enabled || s.conn == nil {
		return nil, false
	}

	obj := s.conn.Object(nmService, nmPath)
	var devicePaths []dbus.ObjectPath
	if err := obj.Call(nmIface+".GetDevices", 0).Store(&devicePaths); err != nil {
		return nil, enabled
	}

	seen := make(map[string]*AccessPoint)
	for _, dp := range devicePaths {
		dev := s.conn.Object(nmService, dp)
		tVal, err := dev.GetProperty(nmDeviceIface + ".DeviceType")
		if err != nil || tVal.Value().(uint32) != nmDeviceTypeWifi {
			continue
		}

		var activeAP dbus.ObjectPath
		activeVal, err := dev.GetProperty(nmWirelessDeviceIface + ".ActiveAccessPoint")
		if err == nil {
			activeAP = activeVal.Value().(dbus.ObjectPath)
		}

		var apPaths []dbus.ObjectPath
		if err := dev.Call(nmWirelessDeviceIface+".GetAccessPoints", 0).Store(&apPaths); err != nil {
			continue
		}

		for _, apPath := range apPaths {
			ap := s.conn.Object(nmService, apPath)
			ssidVal, err := ap.GetProperty(nmAccessPointIface + ".Ssid")
			if err != nil {
				continue
			}
			ssidBytes, ok := ssidVal.Value().([]byte)
			if !ok {
				continue
			}
			ssid := strings.TrimSpace(string(ssidBytes))
			if ssid == "" {
				continue
			}

			strVal, _ := ap.GetProperty(nmAccessPointIface + ".Strength")
			var strength uint8
			if strVal.Value() != nil {
				if s, ok := strVal.Value().(byte); ok {
					strength = s
				} else if s, ok := strVal.Value().(uint32); ok {
					strength = uint8(s)
				}
			}

			wpaVal, _ := ap.GetProperty(nmAccessPointIface + ".WpaFlags")
			rsnVal, _ := ap.GetProperty(nmAccessPointIface + ".RsnFlags")
			sec := false
			if wpaVal.Value() != nil {
				if w, ok := wpaVal.Value().(uint32); ok && w > 0 {
					sec = true
				}
			}
			if rsnVal.Value() != nil {
				if r, ok := rsnVal.Value().(uint32); ok && r > 0 {
					sec = true
				}
			}

			conn := (apPath == activeAP)
			if existing, exists := seen[ssid]; exists {
				if strength > existing.Strength {
					existing.Strength = strength
				}
				if conn {
					existing.Connected = true
				}
			} else {
				seen[ssid] = &AccessPoint{
					SSID:      ssid,
					Strength:  strength,
					Security:  sec,
					Connected: conn,
				}
			}
		}
	}

	list := make([]AccessPoint, 0, len(seen))
	for _, ap := range seen {
		list = append(list, *ap)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Connected != list[j].Connected {
			return list[i].Connected
		}
		return list[i].Strength > list[j].Strength
	})

	return list, enabled
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
	s.refresh()
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
	enabled := s.IsWirelessEnabled()

	s.mu.Lock()
	s.wirelessEnabled = enabled
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
	switch v := value.Value().(type) {
	case uint32:
		return v, nil
	case uint8:
		return uint32(v), nil
	case uint16:
		return uint32(v), nil
	case int32:
		return uint32(v), nil
	default:
		return 0, fmt.Errorf("unexpected type %T", value.Value())
	}
}

func getObjectPathProperty(obj dbus.BusObject, iface, prop string) (dbus.ObjectPath, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return "", err
	}
	if p, ok := value.Value().(dbus.ObjectPath); ok {
		return p, nil
	}
	if s, ok := value.Value().(string); ok {
		return dbus.ObjectPath(s), nil
	}
	return "", fmt.Errorf("unexpected type %T", value.Value())
}

func getBytesProperty(obj dbus.BusObject, iface, prop string) ([]byte, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return nil, err
	}
	if b, ok := value.Value().([]byte); ok {
		return b, nil
	}
	if s, ok := value.Value().(string); ok {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("unexpected type %T", value.Value())
}

func getByteProperty(obj dbus.BusObject, iface, prop string) (uint8, error) {
	value, err := getProperty(obj, iface, prop)
	if err != nil {
		return 0, err
	}
	switch v := value.Value().(type) {
	case uint8:
		return v, nil
	case uint32:
		return uint8(v), nil
	case int32:
		return uint8(v), nil
	default:
		return 0, fmt.Errorf("unexpected type %T", value.Value())
	}
}

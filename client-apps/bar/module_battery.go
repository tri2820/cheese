package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type BatteryModule struct {
	TextModule
	path string
	stop chan struct{}
}

func NewBatteryModule() *BatteryModule {
	return &BatteryModule{
		path: firstBatteryPath(),
		stop: make(chan struct{}),
	}
}

func (m *BatteryModule) Start(markDirty func()) {
	m.refresh()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				before := m.Text()
				m.refresh()
				if m.Text() != before {
					markDirty()
				}
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *BatteryModule) Close() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *BatteryModule) refresh() {
	if m.path == "" {
		m.SetText("")
		return
	}

	capacityBytes, err := os.ReadFile(filepath.Join(m.path, "capacity"))
	if err != nil {
		return
	}
	statusBytes, err := os.ReadFile(filepath.Join(m.path, "status"))
	if err != nil {
		return
	}

	capacity, err := strconv.Atoi(strings.TrimSpace(string(capacityBytes)))
	if err != nil {
		return
	}
	status := strings.TrimSpace(string(statusBytes))

	m.SetText(strconv.Itoa(capacity) + "% " + batteryIcon(capacity, status))
}

func firstBatteryPath() string {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		base := filepath.Join("/sys/class/power_supply", entry.Name())
		typeBytes, err := os.ReadFile(filepath.Join(base, "type"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(typeBytes)) == "Battery" {
			return base
		}
	}

	return ""
}

func batteryIcon(capacity int, status string) string {
	if status == "Charging" {
		switch {
		case capacity >= 95:
			return "󰂅"
		case capacity >= 80:
			return "󰂋"
		case capacity >= 60:
			return "󰂊"
		case capacity >= 40:
			return "󰢞"
		case capacity >= 20:
			return "󰂉"
		default:
			return "󰢜"
		}
	}

	switch {
	case capacity >= 95:
		return "󰁹"
	case capacity >= 80:
		return "󰂂"
	case capacity >= 60:
		return "󰂀"
	case capacity >= 40:
		return "󰁾"
	case capacity >= 20:
		return "󰁼"
	default:
		return "󰁺"
	}
}

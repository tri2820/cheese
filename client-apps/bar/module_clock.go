package main

import (
	"log"
	"sync"
	"time"
)

type ClockModule struct {
	TextModule
	stop      chan struct{}
	mu        sync.RWMutex
	markDirty func()
	local     *time.Location
	singapore *time.Location
	useLocal  bool
}

func NewClockModule() *ClockModule {
	local := time.Now().Location()
	singapore, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		log.Printf("Clock module load location error: %v", err)
		singapore = time.FixedZone("Singapore", 8*60*60)
	}
	return &ClockModule{
		stop:      make(chan struct{}),
		local:     local,
		singapore: singapore,
		useLocal:  true,
	}
}

func (m *ClockModule) Start(markDirty func()) {
	m.mu.Lock()
	m.markDirty = markDirty
	m.mu.Unlock()
	m.updateText(time.Now(), false)

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case t := <-ticker.C:
				m.updateText(t, true)
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *ClockModule) Close() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *ClockModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}

	m.mu.Lock()
	m.useLocal = !m.useLocal
	m.mu.Unlock()
	m.updateText(time.Now(), true)
}

func (m *ClockModule) updateText(now time.Time, dirty bool) {
	m.mu.RLock()
	useLocal := m.useLocal
	local := m.local
	singapore := m.singapore
	markDirty := m.markDirty
	m.mu.RUnlock()

	loc := singapore
	label := " Singapore"
	if useLocal {
		loc = local
		label = ""
	}

	m.SetText(now.In(loc).Format("2 Jan 15:04:05") + label)
	if dirty && markDirty != nil {
		markDirty()
	}
}

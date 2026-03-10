package main

import "time"

type ClockModule struct {
	TextModule
	stop chan struct{}
}

func NewClockModule() *ClockModule {
	return &ClockModule{stop: make(chan struct{})}
}

func (m *ClockModule) Start(markDirty func()) {
	m.SetText(time.Now().Format("2 Jan 15:04:05"))

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case t := <-ticker.C:
				m.SetText(t.Format("2 Jan 15:04:05"))
				markDirty()
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

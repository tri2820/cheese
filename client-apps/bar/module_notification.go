package main

import (
	"fmt"
	"image"
)

type NotificationModule struct {
	TextModule
	service     *NotificationService
	flyout      *NotificationFlyout
	unsubscribe func()
}

func NewNotificationModule(service *NotificationService, flyout *NotificationFlyout) *NotificationModule {
	m := &NotificationModule{
		service: service,
		flyout:  flyout,
	}
	m.updateText()
	return m
}

func (m *NotificationModule) Start(markDirty func()) {
	m.unsubscribe = m.service.Subscribe(func() {
		before := m.Text()
		m.updateText()
		if m.Text() != before {
			markDirty()
		}
	})
}

func (m *NotificationModule) Close() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

func (m *NotificationModule) updateText() {
	count := m.service.Count()
	if count > 0 {
		m.SetText(fmt.Sprintf("󰂚 %d", count))
	} else {
		m.SetText("󰂚")
	}
}

func (m *NotificationModule) HoverRect(rect image.Rectangle, point image.Point) (image.Rectangle, bool) {
	return rect, true
}

func (m *NotificationModule) OnClickAt(button uint32, rect image.Rectangle, point image.Point) bool {
	if button != 0x110 {
		return false
	}
	m.flyout.Toggle(int32(rect.Min.X))
	return true
}

func (m *NotificationModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}
	m.flyout.Toggle(barSidePad)
}

package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"os/exec"
	"strconv"

	"golang.org/x/image/font"
)

const (
	taskbarIconSize = 18
	taskbarItemPadX = 4
)

type taskbarItem struct {
	window NiriWindow
	icon   image.Image
}

type TaskbarModule struct {
	output      string
	niri        *NiriService
	resolver    *IconResolver
	unsubscribe func()
	items       []taskbarItem
}

func NewTaskbarModule(output string, niri *NiriService, resolver *IconResolver) *TaskbarModule {
	return &TaskbarModule{
		output:   output,
		niri:     niri,
		resolver: resolver,
	}
}

func (m *TaskbarModule) Start(markDirty func()) {
	m.unsubscribe = m.niri.Subscribe(func(state NiriState) {
		windows := windowsForOutput(state, m.output)
		next := make([]taskbarItem, 0, len(windows))
		for _, win := range windows {
			log.Printf("taskbar window: output=%q id=%d app_id=%q title=%q focused=%v", m.output, win.ID, win.AppID, win.Title, win.IsFocused)
			next = append(next, taskbarItem{
				window: win,
				icon:   m.resolver.ResolveAppIcon(win.AppID, taskbarIconSize),
			})
		}
		if taskbarItemsEqual(m.items, next) {
			return
		}
		m.items = next
		markDirty()
	})
}

func (m *TaskbarModule) Close() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

func (m *TaskbarModule) Width(face font.Face) int {
	if len(m.items) == 0 {
		return 0
	}
	return len(m.items) * (taskbarIconSize + 2*taskbarItemPadX)
}

func (m *TaskbarModule) Draw(dst draw.Image, rect image.Rectangle, face font.Face) {
	x := rect.Min.X
	y := rect.Min.Y + (rect.Dy()-taskbarIconSize)/2
	for _, item := range m.items {
		slotRect := image.Rect(x, rect.Min.Y, x+taskbarIconSize+2*taskbarItemPadX, rect.Max.Y)
		iconRect := image.Rect(x+taskbarItemPadX, y, x+taskbarItemPadX+taskbarIconSize, y+taskbarIconSize)
		if item.window.IsFocused {
			drawTaskbarFocusBorder(dst, slotRect)
		}
		if item.icon != nil {
			draw.Draw(dst, iconRect, item.icon, image.Point{}, draw.Over)
		} else {
			drawTaskbarFallback(dst, iconRect)
		}
		x += slotRect.Dx()
	}
}

func (m *TaskbarModule) HoverRect(rect image.Rectangle, point image.Point) (image.Rectangle, bool) {
	_, slotRect, ok := m.itemAt(rect, point)
	return slotRect, ok
}

func (m *TaskbarModule) OnClickAt(button uint32, rect image.Rectangle, point image.Point) bool {
	if button != 0x110 {
		return false
	}
	item, _, ok := m.itemAt(rect, point)
	if !ok {
		return false
	}
	cmd := exec.Command("niri", "msg", "action", "focus-window", "--id", strconv.FormatInt(item.window.ID, 10))
	if err := cmd.Run(); err != nil {
		log.Printf("taskbar focus click error: id=%d err=%v", item.window.ID, err)
	}
	return true
}

func taskbarItemsEqual(a, b []taskbarItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].window.ID != b[i].window.ID || a[i].window.AppID != b[i].window.AppID || a[i].window.IsFocused != b[i].window.IsFocused {
			return false
		}
	}
	return true
}

func (m *TaskbarModule) itemAt(rect image.Rectangle, point image.Point) (taskbarItem, image.Rectangle, bool) {
	x := rect.Min.X
	for _, item := range m.items {
		slotRect := image.Rect(x, rect.Min.Y, x+taskbarIconSize+2*taskbarItemPadX, rect.Max.Y)
		if point.In(slotRect) {
			return item, slotRect, true
		}
		x += slotRect.Dx()
	}
	return taskbarItem{}, image.Rectangle{}, false
}

func drawTaskbarFallback(dst draw.Image, rect image.Rectangle) {
	draw.Draw(dst, rect, image.NewUniform(color.RGBA{R: 0x50, G: 0x50, B: 0x50, A: 0xff}), image.Point{}, draw.Src)
}

func drawTaskbarFocusBorder(dst draw.Image, rect image.Rectangle) {
	border := image.Rect(rect.Min.X, rect.Max.Y-2, rect.Max.X, rect.Max.Y)
	if border.Empty() {
		return
	}
	draw.Draw(dst, border, image.NewUniform(color.RGBA{R: 0x53, G: 0xa7, B: 0xff, A: 0xff}), image.Point{}, draw.Src)
}

package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"sync"

	"golang.org/x/image/font"
)

const (
	taskbarIconSize         = 18
	taskbarItemPadX         = 4
	taskbarPadY             = 2
	taskbarPadX             = 2
	taskbarGroupGap         = 4
	taskbarMinSlotsPerGroup = 1
)

type taskbarItem struct {
	window NiriWindow
	icon   image.Image
}

type taskbarGroup struct {
	workspaceID int64
	isActive    bool
	items       []taskbarItem
}

type TaskbarModule struct {
	output      string
	niri        *NiriService
	resolver    *IconResolver
	unsubscribe func()
	mu          sync.RWMutex
	groups      []taskbarGroup
}

func NewTaskbarModule(output string, niri *NiriService, resolver *IconResolver) *TaskbarModule {
	return &TaskbarModule{
		output:   output,
		niri:     niri,
		resolver: resolver,
	}
}

func (m *TaskbarModule) DrawOwnBackground() bool {
	return true
}

func (m *TaskbarModule) Start(markDirty func()) {
	m.unsubscribe = m.niri.Subscribe(func(state NiriState) {
		next := buildTaskbarGroups(state, m.output, m.resolver)
		m.mu.Lock()
		if taskbarGroupsEqual(m.groups, next) {
			m.mu.Unlock()
			return
		}
		m.groups = next
		m.mu.Unlock()
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
	groups := m.snapshotGroups()
	if len(groups) == 0 {
		return 0
	}

	width := 0
	for i, group := range groups {
		width += taskbarGroupWidth(group)
		if i < len(groups)-1 {
			width += taskbarGroupGap
		}
	}
	return width
}

func (m *TaskbarModule) Draw(dst draw.Image, rect image.Rectangle, face font.Face) {
	groups := m.snapshotGroups()
	if len(groups) == 0 {
		return
	}

	y := rect.Min.Y + taskbarPadY + (rect.Dy()-2*taskbarPadY-taskbarIconSize)/2
	x := rect.Min.X
	for i, group := range groups {
		groupWidth := taskbarGroupWidth(group)
		groupRect := image.Rect(x, rect.Min.Y, x+groupWidth, rect.Max.Y)
		drawModuleBackground(dst, groupRect)

		slotX := x + taskbarPadX
		for _, item := range group.items {
			slotRect := image.Rect(slotX, rect.Min.Y+taskbarPadY, slotX+taskbarSlotWidth(), rect.Max.Y-taskbarPadY)
			indicatorRect := image.Rect(slotX, rect.Min.Y, slotX+taskbarSlotWidth(), rect.Max.Y)
			iconRect := image.Rect(slotX+taskbarItemPadX, y, slotX+taskbarItemPadX+taskbarIconSize, y+taskbarIconSize)
			if item.window.IsFocused {
				drawTaskbarFocusBorder(dst, indicatorRect)
			}
			if item.icon != nil {
				draw.Draw(dst, iconRect, item.icon, image.Point{}, draw.Over)
			} else {
				drawTaskbarFallback(dst, iconRect)
			}
			slotX += slotRect.Dx()
		}

		if group.isActive && len(group.items) == 0 {
			drawTaskbarFocusBorder(dst, groupRect)
		}

		x += groupWidth
		if i < len(groups)-1 {
			x += taskbarGroupGap
		}
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
		return true
	}
	return true
}

func taskbarGroupsEqual(a, b []taskbarGroup) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].workspaceID != b[i].workspaceID || a[i].isActive != b[i].isActive {
			return false
		}
		if !taskbarItemsEqual(a[i].items, b[i].items) {
			return false
		}
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
	groups := m.snapshotGroups()
	x := rect.Min.X
	for i, group := range groups {
		groupWidth := taskbarGroupWidth(group)
		slotX := x + taskbarPadX
		for _, item := range group.items {
			slotRect := image.Rect(slotX, rect.Min.Y+taskbarPadY, slotX+taskbarSlotWidth(), rect.Max.Y-taskbarPadY)
			if point.In(slotRect) {
				return item, slotRect, true
			}
			slotX += slotRect.Dx()
		}

		x += groupWidth
		if i < len(groups)-1 {
			x += taskbarGroupGap
		}
	}
	return taskbarItem{}, image.Rectangle{}, false
}

func (m *TaskbarModule) snapshotGroups() []taskbarGroup {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.groups) == 0 {
		return nil
	}

	groups := make([]taskbarGroup, len(m.groups))
	for i := range m.groups {
		groups[i].workspaceID = m.groups[i].workspaceID
		groups[i].isActive = m.groups[i].isActive
		if len(m.groups[i].items) == 0 {
			continue
		}
		groups[i].items = make([]taskbarItem, len(m.groups[i].items))
		copy(groups[i].items, m.groups[i].items)
	}
	return groups
}

func buildTaskbarGroups(state NiriState, output string, resolver *IconResolver) []taskbarGroup {
	workspaces := taskbarWorkspacesForOutput(state.Workspaces, output)
	groups := make([]taskbarGroup, 0, len(workspaces))
	for _, workspace := range workspaces {
		windows := taskbarWindowsForWorkspace(state.Windows, workspace.ID)
		if len(windows) == 0 {
			continue
		}
		items := make([]taskbarItem, 0, len(windows))
		for _, win := range windows {
			items = append(items, taskbarItem{
				window: win,
				icon:   resolver.ResolveAppIcon(win.AppID, taskbarIconSize),
			})
		}
		groups = append(groups, taskbarGroup{
			workspaceID: workspace.ID,
			isActive:    workspace.IsActive,
			items:       items,
		})
	}
	return groups
}

func taskbarWorkspacesForOutput(workspaces []NiriWorkspace, output string) []NiriWorkspace {
	filtered := make([]NiriWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace.Output == output {
			filtered = append(filtered, workspace)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Idx != filtered[j].Idx {
			return filtered[i].Idx < filtered[j].Idx
		}
		return filtered[i].ID < filtered[j].ID
	})
	return filtered
}

func taskbarWindowsForWorkspace(windows []NiriWindow, workspaceID int64) []NiriWindow {
	filtered := make([]NiriWindow, 0)
	for _, win := range windows {
		if win.WorkspaceID == workspaceID {
			filtered = append(filtered, win)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		xi := filtered[i].Layout.PosInScrollingLayout[0]
		xj := filtered[j].Layout.PosInScrollingLayout[0]
		if xi != xj {
			return xi < xj
		}
		return filtered[i].ID < filtered[j].ID
	})
	return filtered
}

func taskbarSlotWidth() int {
	return taskbarIconSize + 2*taskbarItemPadX
}

func taskbarGroupWidth(group taskbarGroup) int {
	slots := len(group.items)
	if slots < taskbarMinSlotsPerGroup {
		slots = taskbarMinSlotsPerGroup
	}
	return 2*taskbarPadX + slots*taskbarSlotWidth()
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

package main

import (
	"html"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
	xdraw "golang.org/x/image/draw"
)

const (
	notifThumbSize         = 44
	notifThumbCornerRadius = 6
	maxStoredNotifications = 50
	dbusInterfaceName      = "org.freedesktop.Notifications"
	dbusPath               = "/org/freedesktop/Notifications"
)

type Notification struct {
	ID        uint32
	AppName   string
	Summary   string
	Body      string
	AppIcon   string
	Thumbnail image.Image
	CreatedAt time.Time
	Urgency   byte
	Unread    bool
}

type NotificationService struct {
	mu             sync.RWMutex
	conn           *dbus.Conn
	notifications  []Notification
	nextID         uint32
	subscribers    map[int]func()
	newSubscribers map[int]func()
	nextSubID      int
	hasUnread      bool
}

func NewNotificationService() *NotificationService {
	s := &NotificationService{
		notifications:  make([]Notification, 0, maxStoredNotifications),
		subscribers:    make(map[int]func()),
		newSubscribers: make(map[int]func()),
	}

	go s.initDBus()
	return s
}

func (s *NotificationService) initDBus() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("NotificationService: Failed to connect to session bus: %v", err)
		return
	}

	reply, err := conn.RequestName(dbusInterfaceName, dbus.NameFlagReplaceExisting|dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Printf("NotificationService: Failed to request D-Bus name: %v", err)
		_ = conn.Close()
		return
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Printf("NotificationService: Could not become primary owner of %s (reply: %v)", dbusInterfaceName, reply)
	}

	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	server := &notificationDBusServer{service: s}
	if err := conn.Export(server, dbusPath, dbusInterfaceName); err != nil {
		log.Printf("NotificationService: Failed to export D-Bus object: %v", err)
		return
	}

	log.Printf("NotificationService: Exported %s successfully", dbusInterfaceName)
}

func (s *NotificationService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

func (s *NotificationService) Subscribe(fn func()) func() {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = fn
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *NotificationService) SubscribeNew(fn func()) func() {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.newSubscribers[id] = fn
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		delete(s.newSubscribers, id)
		s.mu.Unlock()
	}
}

func (s *NotificationService) notifySubscribers() {
	s.mu.RLock()
	subs := make([]func(), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	s.mu.RUnlock()

	go func() {
		for _, fn := range subs {
			if fn != nil {
				fn()
			}
		}
	}()
}

func (s *NotificationService) notifyNewSubscribers() {
	s.mu.RLock()
	subs := make([]func(), 0, len(s.newSubscribers))
	for _, fn := range s.newSubscribers {
		subs = append(subs, fn)
	}
	s.mu.RUnlock()

	go func() {
		for _, fn := range subs {
			if fn != nil {
				fn()
			}
		}
	}()
}

func cleanNotificationMarkup(input string) string {
	if input == "" {
		return ""
	}
	s := strings.ReplaceAll(input, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")

	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' && inTag {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}

func (s *NotificationService) AddNotification(replacesID uint32, appName, summary, body, appIcon string, urgency byte, thumb image.Image) uint32 {
	id := replacesID
	if id == 0 {
		id = atomic.AddUint32(&s.nextID, 1)
	}

	n := Notification{
		ID:        id,
		AppName:   strings.TrimSpace(appName),
		Summary:   cleanNotificationMarkup(summary),
		Body:      cleanNotificationMarkup(body),
		AppIcon:   appIcon,
		Thumbnail: thumb,
		CreatedAt: time.Now(),
		Urgency:   urgency,
		Unread:    true,
	}

	s.mu.Lock()
	replaced := false
	if replacesID != 0 {
		for i := range s.notifications {
			if s.notifications[i].ID == replacesID {
				s.notifications[i] = n
				replaced = true
				break
			}
		}
	}
	if !replaced {
		s.notifications = append([]Notification{n}, s.notifications...)
		if len(s.notifications) > maxStoredNotifications {
			s.notifications = s.notifications[:maxStoredNotifications]
		}
	}
	s.hasUnread = true
	s.mu.Unlock()

	s.notifySubscribers()
	s.notifyNewSubscribers()
	return id
}

func (s *NotificationService) List() []Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]Notification, len(s.notifications))
	copy(res, s.notifications)
	return res
}

func (s *NotificationService) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.notifications)
}

func (s *NotificationService) HasUnread() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasUnread
}

func (s *NotificationService) MarkAllRead() {
	s.mu.Lock()
	changed := s.hasUnread
	s.hasUnread = false
	for i := range s.notifications {
		if s.notifications[i].Unread {
			s.notifications[i].Unread = false
			changed = true
		}
	}
	s.mu.Unlock()
	if changed {
		s.notifySubscribers()
	}
}

func (s *NotificationService) Dismiss(id uint32) {
	s.mu.Lock()
	for i, n := range s.notifications {
		if n.ID == id {
			s.notifications = append(s.notifications[:i], s.notifications[i+1:]...)
			break
		}
	}
	if len(s.notifications) == 0 {
		s.hasUnread = false
	}
	s.mu.Unlock()

	s.notifySubscribers()
}

func (s *NotificationService) ClearAll() {
	s.mu.Lock()
	s.notifications = s.notifications[:0]
	s.hasUnread = false
	s.mu.Unlock()

	s.notifySubscribers()
}

type notificationDBusServer struct {
	service *NotificationService
}

func (d *notificationDBusServer) GetCapabilities() ([]string, *dbus.Error) {
	return []string{"body", "body-markup", "actions", "icon-static"}, nil
}

func (d *notificationDBusServer) Notify(appName string, replacesID uint32, appIcon string, summary string, body string, actions []string, hints map[string]dbus.Variant, expireTimeout int32) (uint32, *dbus.Error) {
	urgency := byte(1)
	if u, ok := hints["urgency"]; ok {
		if val, ok := u.Value().(byte); ok {
			urgency = val
		}
	}

	thumb := extractNotificationThumbnail(appIcon, hints)
	id := d.service.AddNotification(replacesID, appName, summary, body, appIcon, urgency, thumb)
	return id, nil
}

func (d *notificationDBusServer) CloseNotification(id uint32) *dbus.Error {
	d.service.Dismiss(id)
	return nil
}

func (d *notificationDBusServer) GetServerInformation() (string, string, string, string, *dbus.Error) {
	return "cheese-bar", "tri2820", "1.0", "1.2", nil
}

func extractNotificationThumbnail(appIcon string, hints map[string]dbus.Variant) image.Image {
	// 1. Check raw pixel data in hints (image-data, image_data, icon_data) per Desktop Notifications Spec
	for _, key := range []string{"image-data", "image_data", "icon_data"} {
		if v, ok := hints[key]; ok {
			if img := parseDBusImageData(v); img != nil {
				return roundAndScaleThumbnail(img, notifThumbSize, notifThumbCornerRadius)
			}
		}
	}

	// 2. Check image-path / image_path in hints
	for _, key := range []string{"image-path", "image_path"} {
		if v, ok := hints[key]; ok {
			if pathStr, ok := v.Value().(string); ok && pathStr != "" {
				if img := loadImageSpec(pathStr); img != nil {
					return roundAndScaleThumbnail(img, notifThumbSize, notifThumbCornerRadius)
				}
			}
		}
	}

	// 3. Check appIcon parameter
	if appIcon != "" {
		if img := loadImageSpec(appIcon); img != nil {
			return roundAndScaleThumbnail(img, notifThumbSize, notifThumbCornerRadius)
		}
	}

	return nil
}

func parseDBusImageData(v dbus.Variant) image.Image {
	fields, ok := v.Value().([]interface{})
	if !ok || len(fields) < 7 {
		return nil
	}

	width, ok1 := toInt(fields[0])
	height, ok2 := toInt(fields[1])
	rowstride, ok3 := toInt(fields[2])
	hasAlpha, ok4 := fields[3].(bool)
	bitsPerSample, ok5 := toInt(fields[4])
	channels, ok6 := toInt(fields[5])
	data, ok7 := fields[6].([]byte)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
		return nil
	}
	if width <= 0 || height <= 0 || bitsPerSample != 8 || (channels != 3 && channels != 4) {
		return nil
	}

	pixelStride := channels
	expectedLen := (height-1)*rowstride + width*pixelStride
	if len(data) < expectedLen {
		return nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		rowOffset := y * rowstride
		for x := 0; x < width; x++ {
			px := rowOffset + x*pixelStride
			r := data[px+0]
			g := data[px+1]
			b := data[px+2]
			a := byte(0xff)
			if hasAlpha && channels == 4 {
				a = data[px+3]
			}
			dstOff := img.PixOffset(x, y)
			img.Pix[dstOff+0] = r
			img.Pix[dstOff+1] = g
			img.Pix[dstOff+2] = b
			img.Pix[dstOff+3] = a
		}
	}
	return img
}

func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int32:
		return int(val), true
	case int:
		return val, true
	case uint32:
		return int(val), true
	default:
		return 0, false
	}
}

func loadImageSpec(spec string) image.Image {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}

	path := spec
	if strings.HasPrefix(path, "file://") {
		trimmed := strings.TrimPrefix(path, "file://")
		if unescaped, err := url.PathUnescape(trimmed); err == nil {
			path = unescaped
		} else {
			path = trimmed
		}
	} else if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	if !filepath.IsAbs(path) {
		resolved := findIconPath(path)
		if resolved == "" {
			return nil
		}
		path = resolved
	}

	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return loadSVGIcon(path, notifThumbSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return src
}

func roundAndScaleThumbnail(src image.Image, size int, radius int) image.Image {
	if src == nil || size <= 0 {
		return nil
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}

	cropSize := sw
	if sh < cropSize {
		cropSize = sh
	}
	cropX := b.Min.X + (sw-cropSize)/2
	cropY := b.Min.Y + (sh-cropSize)/2
	cropRect := image.Rect(cropX, cropY, cropX+cropSize, cropY+cropSize)

	scaled := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, cropRect, xdraw.Over, nil)

	if radius <= 0 {
		return scaled
	}

	out := image.NewRGBA(image.Rect(0, 0, size, size))
	rect := out.Bounds()
	const samples = 4
	invSamples := 1.0 / float64(samples*samples)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			coverage := 0.0
			for sy := 0; sy < samples; sy++ {
				py := float64(y) + (float64(sy)+0.5)/float64(samples)
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(samples)
					if pointInRoundedRect(px, py, rect, float64(radius)) {
						coverage += invSamples
					}
				}
			}
			if coverage <= 0 {
				continue
			}
			c := color.NRGBAModel.Convert(scaled.At(x, y)).(color.NRGBA)
			c.A = uint8(math.Round(float64(c.A) * coverage))
			out.Set(x, y, c)
		}
	}
	return out
}

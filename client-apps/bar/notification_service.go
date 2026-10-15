package main

import (
	"html"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

type Notification struct {
	ID        uint32
	AppName   string
	Summary   string
	Body      string
	AppIcon   string
	CreatedAt time.Time
	Urgency   byte
}

type NotificationService struct {
	mu            sync.RWMutex
	conn          *dbus.Conn
	notifications []Notification
	nextID        uint32
	subscribers   map[int]func()
	nextSubID     int
	hasUnread     bool
}

const (
	maxStoredNotifications = 50
	dbusInterfaceName      = "org.freedesktop.Notifications"
	dbusPath               = "/org/freedesktop/Notifications"
)

func NewNotificationService() *NotificationService {
	s := &NotificationService{
		notifications: make([]Notification, 0, maxStoredNotifications),
		subscribers:   make(map[int]func()),
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

func cleanNotificationMarkup(input string) string {
	if input == "" {
		return ""
	}
	s := strings.ReplaceAll(input, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")

	// Strip simple HTML tags (<b>, <i>, <u>, <a>, etc.)
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

func (s *NotificationService) AddNotification(replacesID uint32, appName, summary, body, appIcon string, urgency byte) uint32 {
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
		CreatedAt: time.Now(),
		Urgency:   urgency,
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
	s.hasUnread = false
	s.mu.Unlock()
	s.notifySubscribers()
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

	id := d.service.AddNotification(replacesID, appName, summary, body, appIcon, urgency)
	return id, nil
}

func (d *notificationDBusServer) CloseNotification(id uint32) *dbus.Error {
	d.service.Dismiss(id)
	return nil
}

func (d *notificationDBusServer) GetServerInformation() (string, string, string, string, *dbus.Error) {
	return "cheese-bar", "tri2820", "1.0", "1.2", nil
}

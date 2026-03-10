package main

import (
	"bufio"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/draw"
)

type IconResolver struct {
	mu         sync.Mutex
	cache      map[string]image.Image
	pathCache  map[string]string
	desktopMap map[string]string
}

func NewIconResolver() *IconResolver {
	return &IconResolver{
		cache:      make(map[string]image.Image),
		pathCache:  make(map[string]string),
		desktopMap: make(map[string]string),
	}
}

func (r *IconResolver) ResolveAppIcon(appID string, size int) image.Image {
	if appID == "" || size <= 0 {
		return nil
	}

	key := fmt.Sprintf("%s:%d", appID, size)
	r.mu.Lock()
	if img, ok := r.cache[key]; ok {
		r.mu.Unlock()
		log.Printf("taskbar icon cache hit: app_id=%q size=%d", appID, size)
		return img
	}
	if len(r.desktopMap) == 0 {
		r.desktopMap = buildDesktopMap()
	}
	r.mu.Unlock()

	iconName := r.resolveIconName(appID)
	if iconName == "" {
		log.Printf("taskbar icon resolve failed: app_id=%q candidates=%v", appID, appIDCandidates(appID))
		return nil
	}
	path := r.resolveIconPath(iconName)
	if path == "" {
		log.Printf("taskbar icon path failed: app_id=%q icon=%q", appID, iconName)
		return nil
	}
	log.Printf("taskbar icon resolved: app_id=%q icon=%q path=%q size=%d", appID, iconName, path, size)
	img := loadIconImage(path, size)
	if img == nil {
		log.Printf("taskbar icon load failed: app_id=%q icon=%q path=%q", appID, iconName, path)
		return nil
	}

	r.mu.Lock()
	r.cache[key] = img
	r.mu.Unlock()
	return img
}

func (r *IconResolver) resolveIconName(appID string) string {
	candidates := appIDCandidates(appID)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, candidate := range candidates {
		if icon, ok := r.desktopMap[candidate]; ok && icon != "" {
			log.Printf("taskbar desktop match: app_id=%q candidate=%q icon=%q", appID, candidate, icon)
			return icon
		}
	}
	return ""
}

func (r *IconResolver) resolveIconPath(iconName string) string {
	r.mu.Lock()
	if path, ok := r.pathCache[iconName]; ok {
		r.mu.Unlock()
		return path
	}
	r.mu.Unlock()

	path := findIconPath(iconName)

	r.mu.Lock()
	r.pathCache[iconName] = path
	r.mu.Unlock()
	return path
}

func appIDCandidates(appID string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	add(appID)
	add(strings.ToLower(appID))
	if idx := strings.LastIndex(appID, "."); idx >= 0 && idx < len(appID)-1 {
		add(appID[idx+1:])
		add(strings.ToLower(appID[idx+1:]))
	}
	if idx := strings.Index(appID, "-"); idx > 0 {
		add(appID[:idx])
		add(strings.ToLower(appID[:idx]))
	}
	return out
}

func buildDesktopMap() map[string]string {
	result := make(map[string]string)
	for _, file := range desktopFiles() {
		appID, startupWMClass, icon := parseDesktopFile(file)
		if icon == "" {
			continue
		}
		for _, key := range appIDCandidates(appID) {
			if _, ok := result[key]; !ok {
				result[key] = icon
			}
		}
		for _, key := range appIDCandidates(startupWMClass) {
			if _, ok := result[key]; !ok {
				result[key] = icon
			}
		}
	}
	return result
}

func desktopFiles() []string {
	roots := []string{
		filepath.Join(os.Getenv("HOME"), ".local/share/applications"),
		"/run/current-system/sw/share/applications",
		"/home/tri/.nix-profile/share/applications",
		"/etc/profiles/per-user/tri/share/applications",
	}
	var files []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}
			files = append(files, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(files)
	return files
}

func parseDesktopFile(path string) (appID, startupWMClass, icon string) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", ""
	}
	defer file.Close()

	appID = strings.TrimSuffix(filepath.Base(path), ".desktop")
	inDesktopEntry := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inDesktopEntry = line == "[Desktop Entry]"
			continue
		}
		if !inDesktopEntry || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Icon":
			icon = value
		case "StartupWMClass":
			startupWMClass = value
		}
	}
	return appID, startupWMClass, icon
}

func findIconPath(iconName string) string {
	if filepath.IsAbs(iconName) {
		if _, err := os.Stat(iconName); err == nil {
			return iconName
		}
	}

	searchRoots := []string{
		"/run/current-system/sw/share/icons",
		"/home/tri/.nix-profile/share/icons",
		"/etc/profiles/per-user/tri/share/icons",
		"/run/current-system/sw/share/pixmaps",
		"/home/tri/.nix-profile/share/pixmaps",
		"/etc/profiles/per-user/tri/share/pixmaps",
	}
	var matches []string
	for _, root := range searchRoots {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if base != iconName {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".png" || ext == ".svg" {
				matches = append(matches, path)
			}
			return nil
		})
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		return iconPathScore(matches[i]) < iconPathScore(matches[j])
	})
	return matches[0]
}

func iconPathScore(path string) int {
	score := 1000
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		score -= 100
	case ".svg":
		score -= 50
	}
	if strings.Contains(path, "/48x48/") {
		score -= 10
	}
	if strings.Contains(path, "/64x64/") {
		score -= 8
	}
	if strings.Contains(path, "/32x32/") {
		score -= 6
	}
	if strings.Contains(path, "/256x256/") {
		score -= 4
	}
	return score
}

func loadIconImage(path string, size int) image.Image {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return loadRasterIcon(path, size)
	case ".svg":
		return loadSVGIcon(path, size)
	default:
		return nil
	}
}

func loadRasterIcon(path string, size int) image.Image {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func loadSVGIcon(path string, size int) image.Image {
	tmpDir, err := os.MkdirTemp("", "cheese-bar-resvg-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "icon.png")
	cmd := exec.Command("resvg", "--width", fmt.Sprintf("%d", size), "--height", fmt.Sprintf("%d", size), path, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("taskbar resvg failed: path=%q err=%v output=%q", path, err, string(out))
		return nil
	}

	f, err := os.Open(outPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

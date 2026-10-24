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
	"time"

	"golang.org/x/image/draw"
)

type IconResolver struct {
	mu sync.Mutex

	cache   map[string]image.Image // "appID:size" -> decoded icon (successes only)
	desktop map[string][]string    // appID candidate -> icon names, in preference order

	// iconIndex maps an icon name to every file with that base name. Both maps are
	// built into fresh maps and swapped in, so a snapshot taken under mu can be
	// read without holding it.
	iconIndex map[string][]string
	lastBuild time.Time
	building  bool
}

// iconIndexRefresh rate-limits rebuilds triggered by a miss, so an app installed
// while the bar is running gets its icon without a restart.
const iconIndexRefresh = 10 * time.Second

func NewIconResolver() *IconResolver {
	r := &IconResolver{
		cache:     make(map[string]image.Image),
		desktop:   map[string][]string{},
		iconIndex: map[string][]string{},
	}
	go r.warm()
	return r
}

// warm builds the desktop and icon indexes off the caller's thread so the first
// real lookup (which runs on the Wayland main thread) never walks the theme tree.
func (r *IconResolver) warm() { r.build() }

func (r *IconResolver) build() {
	desktop := buildDesktopMap()
	index := buildIconIndex()
	r.mu.Lock()
	r.desktop = desktop
	r.iconIndex = index
	r.lastBuild = time.Now()
	r.building = false
	r.mu.Unlock()
}

// refresh rebuilds both indexes, at most once per iconIndexRefresh.
func (r *IconResolver) refresh() bool {
	r.mu.Lock()
	if r.building || time.Since(r.lastBuild) < iconIndexRefresh {
		r.mu.Unlock()
		return false
	}
	r.building = true
	r.mu.Unlock()
	r.build()
	return true
}

func (r *IconResolver) snapshot() (map[string][]string, map[string][]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.desktop, r.iconIndex
}

func (r *IconResolver) ResolveAppIcon(appID string, size int) image.Image {
	if appID == "" || size <= 0 {
		return nil
	}

	key := fmt.Sprintf("%s:%d", appID, size)
	r.mu.Lock()
	if img, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return img
	}
	r.mu.Unlock()

	desktop, index := r.snapshot()
	for attempt := 0; attempt < 2; attempt++ {
		for _, iconName := range resolveIconNames(desktop, appID) {
			for _, path := range index[iconName] {
				img := loadIconImage(path, size)
				if img == nil {
					continue
				}
				r.mu.Lock()
				r.cache[key] = img
				r.mu.Unlock()
				return img
			}
		}
		// Miss: the app may have been installed (or its theme populated) since the
		// last build. Rebuild once, rate-limited, and try again.
		if !r.refresh() {
			break
		}
		desktop, index = r.snapshot()
	}
	return nil
}

// ResolveIconPath resolves an icon *name* (from a desktop file or a notification
// hint) to an image file path. Misses are not cached, so a transient failure is
// retried on the next call.
func (r *IconResolver) ResolveIconPath(iconName string) string {
	if iconName == "" {
		return ""
	}
	if filepath.IsAbs(iconName) {
		if _, err := os.Stat(iconName); err == nil {
			return iconName
		}
		return ""
	}
	_, index := r.snapshot()
	if path := bestIconPath(index[iconName]); path != "" {
		return path
	}
	if !r.refresh() {
		return ""
	}
	_, index = r.snapshot()
	return bestIconPath(index[iconName])
}

// resolveIconNames lists the icon names to try for an app id, in preference order.
func resolveIconNames(desktop map[string][]string, appID string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, candidate := range appIDCandidates(appID) {
		for _, iconName := range desktop[candidate] {
			if iconName == "" {
				continue
			}
			if _, ok := seen[iconName]; ok {
				continue
			}
			seen[iconName] = struct{}{}
			out = append(out, iconName)
		}
	}
	return out
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

func buildDesktopMap() map[string][]string {
	result := make(map[string][]string)
	add := func(key, icon string) {
		if key == "" || icon == "" {
			return
		}
		for _, existing := range result[key] {
			if existing == icon {
				return
			}
		}
		result[key] = append(result[key], icon)
	}
	for _, file := range desktopFiles() {
		appID, startupWMClass, icon := parseDesktopFile(file)
		if icon == "" {
			continue
		}
		for _, key := range appIDCandidates(appID) {
			add(key, icon)
		}
		for _, key := range appIDCandidates(startupWMClass) {
			add(key, icon)
		}
	}
	return result
}

func desktopFiles() []string {
	home := os.Getenv("HOME")
	roots := []string{
		filepath.Join(home, ".local/share/applications"),
		"/run/current-system/sw/share/applications",
		filepath.Join(home, ".nix-profile/share/applications"),
		"/etc/profiles/per-user/" + os.Getenv("USER") + "/share/applications",
		filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		"/var/lib/flatpak/exports/share/applications",
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

// defaultIconResolver backs the package-level findIconPath helper (used by the
// notification service) so both share one icon index.
var defaultIconResolver = NewIconResolver()

func findIconPath(iconName string) string {
	return defaultIconResolver.ResolveIconPath(iconName)
}

// buildIconIndex maps every icon base name under the search roots to its files.
func buildIconIndex() map[string][]string {
	index := make(map[string][]string)
	for _, root := range iconSearchRoots() {
		walkIconFiles(root, func(path string) {
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".png" && ext != ".svg" {
				return
			}
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			index[name] = append(index[name], path)
		})
	}
	for name, paths := range index {
		if len(paths) > 1 {
			index[name] = bestIconPaths(paths)
		}
	}
	return index
}

// iconSearchRoots covers system, Nix profile, user theme and flatpak icon trees.
func iconSearchRoots() []string {
	home := os.Getenv("HOME")
	roots := []string{
		"/run/current-system/sw/share/icons",
		filepath.Join(home, ".nix-profile/share/icons"),
		"/etc/profiles/per-user/" + os.Getenv("USER") + "/share/icons",
		"/run/current-system/sw/share/pixmaps",
		filepath.Join(home, ".nix-profile/share/pixmaps"),
		"/etc/profiles/per-user/" + os.Getenv("USER") + "/share/pixmaps",
		filepath.Join(home, ".local/share/icons"),
		filepath.Join(home, ".local/share/pixmaps"),
		filepath.Join(home, ".local/share/flatpak/exports/share/icons"),
		"/var/lib/flatpak/exports/share/icons",
	}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			out = append(out, root)
		}
	}
	return out
}

// walkIconFiles visits every regular file under root, following symlinked
// directories (NixOS icon trees are symlinks into the store, which
// filepath.WalkDir refuses to descend into). Real paths are tracked so link
// cycles terminate.
func walkIconFiles(root string, fn func(path string)) {
	visited := make(map[string]struct{})
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 12 {
			return
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return
		}
		if _, seen := visited[real]; seen {
			return
		}
		visited[real] = struct{}{}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			info, err := os.Stat(path) // follows symlinks
			if err != nil {
				continue
			}
			if info.IsDir() {
				walk(path, depth+1)
				continue
			}
			if info.Mode().IsRegular() {
				fn(path)
			}
		}
	}
	walk(root, 0)
}

func bestIconPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func bestIconPaths(paths []string) []string {
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.SliceStable(sorted, func(i, j int) bool {
		return iconPathScore(sorted[i]) < iconPathScore(sorted[j])
	})
	return sorted
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

// svgRendererWarned keeps a missing resvg from logging on every icon attempt.
var svgRendererWarned sync.Once

func loadSVGIcon(path string, size int) image.Image {
	tmpDir, err := os.MkdirTemp("", "cheese-bar-resvg-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "icon.png")
	cmd := exec.Command("resvg", "--width", fmt.Sprintf("%d", size), "--height", fmt.Sprintf("%d", size), path, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		// SVG-only icons need `resvg` in the bar's PATH; warn once, not per icon.
		svgRendererWarned.Do(func() {
			log.Printf("taskbar resvg failed (SVG icons will be skipped until resvg is on PATH): path=%q err=%v output=%q", path, err, strings.TrimSpace(string(out)))
		})
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

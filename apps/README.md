# Cheese Apps

Desktop applications built with the Cheese toolkit.

## cheesebar

A simple status bar (like waybar) using layer shell and SHM rendering.

### Running

```bash
cd apps/cheesebar
go build -o cheesebar
./cheesebar
```

### Features

- Layer shell surface at top of screen
- SHM-based rendering (pure Go, no GPU required)
- Real-time clock display
- Exclusive zone support (reserves space for the bar)

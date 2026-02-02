#!/usr/bin/env bash
# Generate Wayland protocol bindings (flat structure, one package per directory)

set -e

PROTOCOLS_PATH="${PROTOCOLS_PATH:-$(nix eval --raw nixpkgs#wayland-protocols --apply 'x: "${x}/share/wayland-protocols"' 2>/dev/null)}"

# Try to find wayland.xml
WAYLAND_XML="${WAYLAND_XML:-$(find /nix/store -name wayland.xml -path "*/wayland-scanner/*" 2>/dev/null | head -1)}"

# Fallback to any wayland.xml
if [ -z "$WAYLAND_XML" ]; then
    WAYLAND_XML="$(find /nix/store -name wayland.xml 2>/dev/null | grep -E "wayland-scanner|share/wayland" | head -1)"
fi

if [ -z "$PROTOCOLS_PATH" ] || [ ! -d "$PROTOCOLS_PATH" ]; then
    echo "Error: Could not find wayland-protocols path"
    echo "Set PROTOCOLS_PATH environment variable or run in nix develop"
    exit 1
fi

if [ -z "$WAYLAND_XML" ] || [ ! -f "$WAYLAND_XML" ]; then
    echo "Error: Could not find wayland.xml"
    echo "Set WAYLAND_XML environment variable"
    exit 1
fi

echo "Protocols path: $PROTOCOLS_PATH"
echo "Wayland XML: $WAYLAND_XML"
echo ""

echo "Building scanner..."
go build -o wayland-scanner ./cmd/wayland-scanner
echo ""

# Clean up old directory structure
echo "Cleaning up old directory structure..."
rm -rf protocols/stable protocols/staging protocols/unstable
echo ""

# Ensure protocols directory exists
mkdir -p protocols/client

echo "Generating core wayland protocol into client package..."
./wayland-scanner \
    -i "$WAYLAND_XML" \
    -o "protocols/client/wayland.go" \
    -pkg "client" 2>&1 | grep -v "^Generated" || true
echo ""

# Count for progress
total=$(find "$PROTOCOLS_PATH" -name "*.xml" | wc -l)
current=0

# Generate all other protocols (each in its own directory)
echo "Generating protocols..."
for xml in $(find "$PROTOCOLS_PATH" -name "*.xml" | sort); do
    current=$((current + 1))
    base=$(basename "$xml" .xml | tr '-' '_')

    # Skip wayland.xml (already generated into client package)
    if [ "$base" = "wayland" ]; then
        continue
    fi

    echo "  [$current/$total] $base"

    # Create directory for this protocol
    mkdir -p "protocols/$base"

    ./wayland-scanner \
        -i "$xml" \
        -o "protocols/$base/$base.go" \
        -pkg "$base" 2>&1 | grep -v "^Generated" || true
done

echo ""
echo "Done!"
echo ""
echo "Generated protocol summary:"
echo "  Directories: $(find protocols -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)"
echo "  Core: protocols/client/wayland.go (wayland protocol)"

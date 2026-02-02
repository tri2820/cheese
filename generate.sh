#!/usr/bin/env bash
# Generate Wayland protocol bindings including wlroots protocols

set -e

PROTOCOLS_PATH="${PROTOCOLS_PATH:-$(nix eval --raw nixpkgs#wayland-protocols --apply 'x: "${x}/share/wayland-protocols"' 2>/dev/null)}"

# Try to find wayland.xml
WAYLAND_XML="${WAYLAND_XML:-$(find /nix/store -name wayland.xml -path "*/wayland-scanner/*" 2>/dev/null | head -1)}"

# Find wlroots protocols
WLROOTS_PROTOCOLS="${WLROOTS_PROTOCOLS:-$(find /nix/store -type d -name protocols -path "*wlroots*source/protocols" 2>/dev/null | head -1)}"

# Fallback to any wayland.xml
if [ -z "$WAYLAND_XML" ]; then
    WAYLAND_XML="$(find /nix/store -name wayland.xml 2>/dev/null | grep -E "wayland-scanner|share/wayland" | head -1)"
fi

if [ -z "$PROTOCOLS_PATH" ] || [ ! -d "$PROTOCOLS_PATH" ]; then
    echo "Error: Could not find wayland-protocols path"
    exit 1
fi

if [ -z "$WAYLAND_XML" ] || [ ! -f "$WAYLAND_XML" ]; then
    echo "Error: Could not find wayland.xml"
    exit 1
fi

echo "Protocols path: $PROTOCOLS_PATH"
echo "Wayland XML: $WAYLAND_XML"
if [ -n "$WLROOTS_PROTOCOLS" ]; then
    echo "wlroots protocols: $WLROOTS_PROTOCOLS"
fi
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

# Collect all XML files
xml_files=()
while IFS= read -r xml; do
    xml_files+=("$xml")
done < <(find "$PROTOCOLS_PATH" -name "*.xml" | sort)

# Add wlroots protocols if found in a directory
if [ -n "$WLROOTS_PROTOCOLS" ] && [ -d "$WLROOTS_PROTOCOLS" ]; then
    while IFS= read -r xml; do
        xml_files+=("$xml")
    done < <(find "$WLROOTS_PROTOCOLS" -name "*.xml" | sort)
fi

# Also search for individual wlr protocol XMLs scattered in nix store
# (e.g., in Qt bundles, compositor sources, etc.)
echo "Searching for additional wlr-* protocols..."
wlr_count=0
for pattern in "*/protocols/wlr-*.xml" "*/wayland/protocols/wlr-*/*.xml"; do
    while IFS= read -r xml; do
        # Check if we haven't already added this protocol
        base=$(basename "$xml" .xml | tr '-' '_')
        already_added=false
        for existing in "${xml_files[@]}"; do
            if [ "$(basename "$existing" .xml | tr '-' '_')" = "$base" ]; then
                already_added=true
                break
            fi
        done
        if [ "$already_added" = false ]; then
            xml_files+=("$xml")
            wlr_count=$((wlr_count + 1))
            echo "  Found: $(basename "$xml")"
        fi
    done < <(find /nix/store -path "$pattern" 2>/dev/null | sort -u)
done
if [ $wlr_count -gt 0 ]; then
    echo "  Added $wlr_count wlr protocols"
fi
echo ""

total=${#xml_files[@]}
current=0

# Generate all protocols
echo "Generating protocols..."
for xml in "${xml_files[@]}"; do
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
if [ -n "$WLROOTS_PROTOCOLS" ]; then
    echo "  wlroots: $(find protocols -mindepth 1 -maxdepth 1 -type d -name "wlr_*" 2>/dev/null | wc -l) protocols"
fi

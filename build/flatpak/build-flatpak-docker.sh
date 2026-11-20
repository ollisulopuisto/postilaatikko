#!/bin/bash
# Build Postilaatikko Flatpak using Docker (hybrid approach: build binary on host in container, then package)

set -e

if [ "$(id -u)" = 0 ]; then
    echo "Run this build as the normal workspace owner, not root." >&2
    exit 1
fi

cd "$(dirname "$0")/../.."

echo "=== Postilaatikko Flatpak Docker Builder ==="
echo ""

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed"
    echo ""
    echo "Install it with:"
    echo "  Ubuntu/Debian: sudo apt install docker.io"
    echo "  Fedora:        sudo dnf install docker"
    echo "  Arch:          sudo pacman -S docker"
    exit 1
fi

# Check if Docker daemon is running
if ! docker info &> /dev/null; then
    echo "❌ Docker daemon is not running"
    echo ""
    echo "Start it with:"
    echo "  sudo systemctl start docker"
    exit 1
fi

echo "Building Docker image (this may take a few minutes on first run)..."
docker build -t postilaatikko-flatpak-builder -f build/flatpak/Dockerfile build/flatpak

echo ""
echo "Building Postilaatikko in Docker container..."
echo ""

# Get version from git tag
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "dev")

# Run the build in Docker
docker run --rm \
    --user "$(stat -c %u .):$(stat -c %g .)" \
    --env HOME=/tmp/build-home \
    -v "$(pwd):/workspace" \
    -w /workspace \
    postilaatikko-flatpak-builder \
    bash -c "
        test \"\$(id -u):\$(id -g)\" = \"\$(stat -c %u:%g /workspace)\" || exit 1
        test \"\$(id -u)\" != 0 || exit 1
        mkdir -p /tmp/build-home
        echo 'Installing frontend dependencies...'
        cd frontend && npm ci && cd ..

        echo ''
        echo 'Building Postilaatikko binary...'
        make build-linux

        echo ''
        echo 'Packaging into Flatpak...'
        flatpak-builder --force-clean --repo=repo build-dir build/flatpak/flathub/io.github.ollisulopuisto.postilaatikko.yml

        echo ''
        echo 'Creating .flatpak bundle...'
        mkdir -p build/bin
        flatpak build-bundle repo build/bin/Postilaatikko-${VERSION}.flatpak io.github.ollisulopuisto.postilaatikko
    "

echo ""
echo "✅ Build complete!"
echo ""
echo "Flatpak bundle created: build/bin/Postilaatikko-${VERSION}.flatpak"
echo ""
echo "To install locally:"
echo "  flatpak install --user build/bin/Postilaatikko-${VERSION}.flatpak"
echo ""
echo "To run:"
echo "  flatpak run io.github.ollisulopuisto.postilaatikko"

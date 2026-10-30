# BUILD

Set .env file

## Local macOS app and DMG

On a Mac with Xcode Command Line Tools, Go 1.26+, and Node.js 24 installed, run
the repository script from the repository root:

```bash
./scripts/build-macos-local.sh
```

The script installs the pinned Wails CLI and frontend dependencies, builds and
ad-hoc signs the app for the Mac's native architecture, then creates and verifies
a drag-and-drop DMG. The DMG includes `Postilaatikko.app` and an `Applications`
shortcut. Artifacts are written to `build/bin/`:

- `postilaatikko.app`
- `Postilaatikko-macos-arm64.dmg` on Apple Silicon, or
  `Postilaatikko-macos-amd64.dmg` on Intel

For a faster repeat build when frontend dependencies have not changed, skip the
lockfile install:

```bash
./scripts/build-macos-local.sh --skip-npm-ci
```

To build only the app without creating a DMG:

```bash
./scripts/build-macos-local.sh --app-only
```

Run `./scripts/build-macos-local.sh --help` for options. This local build is
not notarized and does not publish a GitHub release. For the published DMG,
use the repository's GitHub Actions release workflow.

### 🔨 Building from Source
---

**Flatpak (Recommended for Linux):**

```bash
# Install flatpak-builder
sudo dnf install flatpak-builder  # Fedora
sudo apt install flatpak-builder  # Ubuntu/Debian
sudo pacman -S flatpak-builder    # Arch

# Set Microsoft and Google oAuth creds
cp .env.example .env
# Fill in your own creds

# Or via make
make flatpak

# Install
flatpak --user install build/bin/Eterno Mail.flatpak

# Run
flatpak run io.github.hkdb.Eterno Mail
```

See [build/flatpak/README.md](../build/flatpak/README.md) for detailed Flatpak build instructions and Flathub submission guide.

**Native Binary:**

```bash
# Install dependencies (Ubuntu/Debian)
sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev

# Set Microsoft and Google oAuth creds
cp .env.example .env
# Fill in your own creds

# Build
make build

# Run
./build/bin/aerion
```



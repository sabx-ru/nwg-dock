# nwg-dock

A lightweight native GTK3 application dock for the Sway Wayland
compositor, written in Go.

Based on the original [nwg-dock](https://github.com/nwg-piotr/nwg-dock),
extended with native multi-monitor management, window-aware
Intellihide, an integrated status panel and drag-hover window activation.

Designed for minimal Linux installations and floating-window workflows
without a traditional desktop environment.

## Features

- Vertically centered application dock on the left screen edge.
- Pinned applications and running window indicators.
- Application activation and workspace switching.
- Native multi-monitor management.
- Independent Intellihide on each active output.
- Window overlap detection through Sway IPC.
- Automatic reveal using 3-pixel edge sensors.
- Drag-hover activation of running applications.
- Optional expandable bottom-left status panel.
- GTK3 CSS customization.
- Automatic handling of active output changes.
- No external dock controller or Python daemon required.
- Original nwg-dock mode retained for compatibility.

## Native architecture

The native manager starts one dock worker per active Sway output.

Each worker manages two independent GTK layer-shell surfaces:

- A vertically centered application dock.
- An optional bottom-left status panel.

Visibility is determined by the geometry of visible windows on the
current workspace of the corresponding output.

Panels remain visible when unobstructed and hide when covered.
Moving the pointer to the corresponding screen edge reveals them.

Native mode does not require Swaybar, Waybar or an external
Intellihide controller.

## Drag and drop

The dock supports drag-hover activation of running applications.

1. Start dragging a file from a file manager such as Thunar.
2. Move the pointer onto the icon of a running application.
3. Hold the pointer over the icon for approximately 400 ms.
4. The application is activated and its workspace becomes visible.
5. Continue dragging the file into the destination application.

A hidden dock can also be revealed during an active drag operation
by moving the pointer to its screen edge.

The dock does not consume or transfer file contents itself.
The actual drop operation is handled by the destination application.

## Installation on Debian 13

The following instructions describe a minimal Debian 13 (Trixie)
installation using Sway and GTK3.

Debian Testing and Unstable repositories are not required.

### 1. Install Debian

Install Debian 13 using the official minimal or network installer:

https://www.debian.org/distrib/

During software selection, deselect desktop environments.

A standard console installation with system utilities is sufficient.

Log in as a regular user with sudo access.

Update the package lists:

```sh
sudo apt update
```

### 2. Install the minimal Wayland environment

```sh
sudo apt install --no-install-recommends \
    sway \
    swaybg \
    swayidle \
    swaylock \
    foot \
    dbus-user-session \
    libgl1-mesa-dri \
    libegl-mesa0 \
    xdg-utils \
    xdg-user-dirs \
    xdg-desktop-portal \
    xdg-desktop-portal-wlr \
    xdg-desktop-portal-gtk \
    adwaita-icon-theme \
    hicolor-icon-theme \
    fonts-dejavu-core \
    fonts-font-awesome \
    ca-certificates
```

This provides a basic Wayland session, terminal, graphics libraries,
desktop portals, fonts and application icons.

Install graphics firmware appropriate for your hardware if required.

No display manager or full desktop environment is necessary.

### 3. Install desktop applications

The following packages are optional but provide a practical
minimal desktop environment:

```sh
sudo apt install --no-install-recommends \
    thunar \
    gvfs \
    gvfs-backends \
    firefox-esr \
    grim \
    slurp \
    wl-clipboard \
    brightnessctl \
    wob
```

For X11 applications running through Wayland:

```sh
sudo apt install --no-install-recommends xwayland
```

For desktop audio:

```sh
sudo apt install --no-install-recommends \
    pipewire \
    pipewire-pulse \
    wireplumber \
    pavucontrol
```

For graphical network and Bluetooth functionality, install and
configure the corresponding system services and desktop tools
as required.

None of these optional applications is a mandatory dock dependency.

### 4. Install nwg-dock runtime dependencies

```sh
sudo apt install --no-install-recommends \
    libgtk-3-0t64 \
    libgtk-layer-shell0 \
    librsvg2-common \
    adwaita-icon-theme \
    hicolor-icon-theme \
    fonts-font-awesome
```

GTK3 and gtk-layer-shell are required at runtime.

The SVG loader is used for dock resources and application icons.

FontAwesome is used by the expandable status panel toggle.

### 5. Install build dependencies

```sh
sudo apt install --no-install-recommends \
    git \
    build-essential \
    golang-go \
    pkg-config \
    libgtk-3-dev \
    libgtk-layer-shell-dev \
    libwayland-dev
```

Requirements:

- Go 1.23 or newer.
- C compiler and CGO support.
- GTK3 development headers.
- gtk-layer-shell development headers.
- Wayland development headers.
- pkg-config.

Debian 13 provides a suitable Go version.

### Optional: Debian Backports

A regular Debian Stable installation does not require Backports
to build nwg-dock.

If your system already uses Wayland libraries from Debian Backports,
the development packages must match the installed runtime libraries.

If Backports is not already configured, it can be enabled using
the official Debian repository:

```sh
sudo tee /etc/apt/sources.list.d/debian-backports.sources >/dev/null <<'BACKPORTS'
Types: deb
URIs: https://deb.debian.org/debian
Suites: trixie-backports
Components: main
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
BACKPORTS

sudo apt update
```

Install the matching Wayland development package:

```sh
sudo apt install --no-install-recommends \
    libwayland-dev/trixie-backports
```

Do not add Debian Testing or Unstable to resolve build dependencies.

Avoid mixing development headers and runtime libraries from
incompatible Debian releases.

## Build from source

Clone the repository:

```sh
mkdir -p ~/git
git clone https://github.com/sabx-ru/nwg-dock.git ~/git/nwg-dock
cd ~/git/nwg-dock
```

Verify the build environment:

```sh
go version
go env CGO_ENABLED
pkg-config --modversion gtk+-3.0
pkg-config --modversion gtk-layer-shell-0
pkg-config --modversion wayland-client
```

Run native core tests:

```sh
go test native_core.go native_core_test.go
```

Build the application:

```sh
make build
```

Go downloads the required modules automatically according to
go.mod and go.sum.

The resulting executable is:

```text
bin/nwg-dock
```

Verify the executable:

```sh
file bin/nwg-dock
ldd bin/nwg-dock
```

No shared library should be reported as "not found".

Do not build using sudo.

## Installation

### Recommended: user-local installation

Install the executable into the user's local binary directory:

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 bin/nwg-dock "$HOME/.local/bin/nwg-dock"
```

Install the required shared resources:

```sh
sudo install -d /usr/share/nwg-dock
sudo cp -r images /usr/share/nwg-dock/
sudo install -m 644 config/* /usr/share/nwg-dock/
```

Create user directories:

```sh
mkdir -p "$HOME/.config/nwg-dock"
mkdir -p "$HOME/.cache"
```

Install the default application dock stylesheet without replacing
an existing user configuration:

```sh
cp -n config/style.css "$HOME/.config/nwg-dock/style.css"
```

Add the local binary directory to PATH if necessary:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

For persistent PATH configuration, add the same line to ~/.profile.

The default resource directory is /usr/share/nwg-dock.

If XDG_DATA_HOME is explicitly set, the current implementation
looks for nwg-dock resources under that directory instead.
Install the resources there if using a custom XDG data directory.

The images directory contains functional SVG resources required
by the application. It must not be removed.

### Alternative: system-wide installation

Instead of the user-local installation, use:

```sh
sudo make install
```

This installs the executable and resources under /usr by default.

The Makefile installation target also stops existing nwg-dock
processes.

Use only one installation method to avoid ambiguity between
multiple installed executables.

## Sway configuration

### 1. Create the user configuration

For a fresh Sway installation:

```sh
mkdir -p ~/.config/sway

if [ ! -f ~/.config/sway/config ]; then
    cp /etc/sway/config ~/.config/sway/config
fi
```

Edit:

```sh
nano ~/.config/sway/config
```

Remove or comment out existing Swaybar configuration blocks
if nwg-dock should replace the traditional desktop panel.

Do not overwrite an existing customized Sway configuration.

### 2. Configure floating-window mode

The following configuration provides a floating-first workflow
similar to a traditional desktop operating system.

Add or adapt these directives:

```text
set $mod Mod4

for_window [all] border none
for_window [all] floating enable

floating_modifier $mod normal

bindsym $mod+Return exec foot
bindsym $mod+q kill
bindsym $mod+space floating toggle
```

Avoid duplicate bindings already present in your configuration.

Native nwg-dock also works with tiled windows.
Floating-window mode is optional.

### 3. Configure touchpad behavior

Optional touchpad settings:

```text
input type:touchpad {
    tap enabled
    dwt enabled
    natural_scroll enabled
    scroll_factor 0.5
}
```

Optional workspace and scratchpad gestures:

```text
bindgesture swipe:3:left workspace next_on_output
bindgesture swipe:3:right workspace prev_on_output
bindgesture swipe:3:down move scratchpad
```

These gestures require compatible touchpad hardware.

The scratchpad can be used to hide windows.
Existing windows can be restored through their dock icons.

Optional application shortcuts:

```text
bindsym $mod+e exec thunar
bindsym $mod+b exec firefox-esr
```

### 4. Configure display idle behavior

Optional automatic display power-off after five minutes:

```text
exec swayidle -w \
    timeout 300 'swaymsg "output * power off"' \
    resume 'swaymsg "output * power on"' \
    after-resume 'swaymsg "output * power on"'
```

### 5. Start nwg-dock automatically

For the recommended user-local installation, add:

```text
exec ~/.local/bin/nwg-dock -native-manager
```

For a system-wide installation, use:

```text
exec /usr/bin/nwg-dock -native-manager
```

Use only one startup command.

Do not create separate dock startup commands for individual
monitors. The native manager handles active outputs automatically.

### 6. Start Sway

Validate the configuration:

```sh
sway -C -c ~/.config/sway/config
```

From a regular local console session, start Sway:

```sh
sway
```

Do not run Sway as root.

### Optional: automatic Sway startup on tty1

To start Sway automatically after logging into the first console,
add the following to ~/.profile:

```sh
export PATH="$HOME/.local/bin:$PATH"

if [ -z "$SSH_CONNECTION" ] &&
   [ -z "$WAYLAND_DISPLAY" ] &&
   [ "$(tty)" = "/dev/tty1" ]; then
    exec sway
fi
```

This does not enable passwordless login.

Hardware-specific GPU configuration is outside the scope
of nwg-dock.

## Status panel

The optional native status panel is displayed independently
at the bottom-left corner of each active output.

Without an enabled status configuration, only the application
dock is displayed.

Configuration files:

```text
~/.config/nwg-dock/style.css
~/.config/nwg-dock/status.json
~/.config/nwg-dock/status.css
```

If XDG_CONFIG_HOME is set, its nwg-dock subdirectory is used.

### Minimal working status configuration

Create a status panel with a clock, date and expandable system
information.

This example requires no external backend scripts:

```sh
mkdir -p ~/.config/nwg-dock

cat > ~/.config/nwg-dock/status.json <<'JSON'
{
  "enabled": true,
  "intellihide": true,
  "width": 56,
  "match_dock_width": true,
  "bottom_margin": 0,
  "rows": [
    ["time"],
    ["date"]
  ],
  "expanded_rows": [
    ["uptime"],
    ["memory"]
  ],
  "items": {
    "time": {
      "command": "date '+%H:%M'",
      "interval": 5,
      "color": "#cad3f5"
    },
    "date": {
      "command": "date '+%d.%m'",
      "interval": 30,
      "color": "#cad3f5"
    },
    "uptime": {
      "command": "uptime -p",
      "interval": 60,
      "color": "#a6da95"
    },
    "memory": {
      "command": "free -m | awk '/^Mem:/ {printf \"%.0f%%\\n\", $3/$2*100}'",
      "interval": 5,
      "color": "#8aadf4"
    }
  }
}
JSON
```

The following stylesheet provides a compact dark appearance:

```sh
cat > ~/.config/nwg-dock/status.css <<'CSS'
window#nwg-status {
    background-color: #24273a;
    color: #cad3f5;
    border-radius: 0 10px 0 0;
}

#status-box {
    padding: 4px;
}

#status-box button {
    background: transparent;
    border: none;
    box-shadow: none;
    min-width: 0;
    min-height: 0;
    padding: 5px 2px;
    font-size: 12px;
    color: #cad3f5;
}

#status-toggle label {
    font-family: FontAwesome;
    font-size: 18px;
}
CSS
```

These commands replace existing status configuration files.
On an already configured system, merge the settings manually.

Additional configuration examples are available in
examples/native-status/.

### Status panel capabilities

The panel supports:

- Configurable rows and items.
- Expandable sections.
- Per-item update intervals.
- Custom colors and optional Pango markup.
- Battery percentage extraction.
- Mouse button handlers.
- Independent Intellihide.
- Automatic matching of application dock width.

Each item executes a configured shell command.

The command output uses an i3blocks-compatible convention:

- First line: displayed text.
- Optional third line: hexadecimal color.

Mouse clicks are passed to commands through BLOCK_BUTTON:

- 1: left mouse button.
- 2: middle mouse button.
- 3: right mouse button.

An interval of zero disables periodic updates but retains
initial execution and click-triggered execution.

Use only trusted commands and status configurations.

Custom Wi-Fi, Bluetooth, VPN, battery and power-profile controls
can be implemented using external executable scripts.

The dock itself does not manage these services.

## Usage

Start the native manager inside a running Sway session:

```sh
~/.local/bin/nwg-dock -native-manager
```

For a custom status configuration:

```sh
~/.local/bin/nwg-dock \
    -native-manager \
    -status-config ~/.config/nwg-dock/status.json
```

Enable diagnostic logging:

```sh
~/.local/bin/nwg-dock -native-manager -debug
```

Display available command-line options:

```sh
~/.local/bin/nwg-dock -h
```

Original nwg-dock functionality remains available without
the native-manager option.

## Persistent configuration

The default user configuration and state locations are:

```text
~/.config/nwg-dock/
~/.cache/nwg-dock-pinned
```

The pinned applications file contains persistent application
identifiers and their order.

Although stored under ~/.cache, this file should not be removed
if the existing pinned application layout must be preserved.

Configuration and stylesheet changes do not require rebuilding
the application. Restart the dock to apply them.

## Updating nwg-dock

Update the source repository:

```sh
cd ~/git/nwg-dock
git pull --ff-only
```

Run tests and rebuild:

```sh
go test native_core.go native_core_test.go && make build
```

After a successful build, stop the existing dock:

```sh
pkill -TERM -u "$(id -u)" -x nwg-dock
```

Wait until its processes have exited.

Install the updated executable:

```sh
install -m 755 bin/nwg-dock ~/.local/bin/nwg-dock
```

Update the shared resources:

```sh
sudo cp -r images /usr/share/nwg-dock/
sudo install -m 644 config/* /usr/share/nwg-dock/
```

Start the new version from the active Sway session:

```sh
swaymsg exec "$HOME/.local/bin/nwg-dock -native-manager"
```

Existing user styles, status settings and pinned applications
are preserved.

## Removing build dependencies

After successful installation, Go and development packages are
not required to run the dock.

First ensure the runtime libraries remain installed:

```sh
sudo apt-mark manual \
    libgtk-3-0t64 \
    libgtk-layer-shell0 \
    librsvg2-common
```

Preview removal of temporary build dependencies:

```sh
apt -s purge \
    golang-go \
    libgtk-3-dev \
    libgtk-layer-shell-dev \
    libwayland-dev
```

Inspect the proposed changes before executing the actual purge.

Do not remove development packages needed by other software.

Preview unused dependency cleanup:

```sh
apt -s autoremove --purge
```

Review the package list carefully before running autoremove.

The installed dock, GTK runtime libraries and shared SVG
resources must remain available.

## Troubleshooting

Check the active Sway session:

```sh
echo "$WAYLAND_DISPLAY"
echo "$SWAYSOCK"
swaymsg -t get_outputs
swaymsg -t get_workspaces
```

Check installed dock processes:

```sh
pgrep -a -x nwg-dock
```

In native mode, multiple processes are expected:
one manager and one worker per active output.

Check runtime libraries:

```sh
ldd ~/.local/bin/nwg-dock
```

Check required resources:

```sh
ls /usr/share/nwg-dock/images/
```

For detailed Intellihide and drag-hover diagnostics, stop the
existing dock and launch:

```sh
~/.local/bin/nwg-dock -native-manager -debug
```

Debug output includes output names, panel geometry, workspace
changes, overlap detection, visibility transitions and
drag-hover events.

After troubleshooting, restart the application without -debug.

## Uninstallation

For the user-local installation, stop the running dock:

```sh
pkill -TERM -u "$(id -u)" -x nwg-dock
```

Remove the executable:

```sh
rm -f ~/.local/bin/nwg-dock
```

Remove its startup command from the Sway configuration.

Shared resources can be removed if no other installed nwg-dock
version uses them:

```sh
sudo rm -r /usr/share/nwg-dock
```

For a system-wide installation performed with make install:

```sh
sudo make uninstall
```

User configuration and pinned applications are retained unless
explicitly deleted.

## Limitations

- Native mode is designed for Sway.
- Native panel placement is currently fixed to the left edge.
- Drag-hover activates existing application windows.
- Actual file acceptance depends on the destination application.
- Drag cancellation depends on compositor and Wayland behavior.
- Hardware-specific services and status backends are not bundled
  with the dock.

## License

MIT.

Original project by nwg-piotr.

Native extensions maintained by sabx-ru.

https://github.com/nwg-piotr/nwg-dock

# Native Intellihide and status panel for Sway

This fork extends nwg-dock 0.4.3 with an optional native Go manager. The original
CLI mode remains available without `-native-manager`. The original license and
upstream history are retained. Native mode runs one worker per active Sway
output and uses Sway IPC to determine window overlap; it requires no Python
controller or Swaybar. Its current layout is a left-centered application dock
and an optional bottom-left status panel, both with independent Intellihide.

## Build

Install Go, pkg-config, GTK3 and gtk-layer-shell development packages. On Debian:

```sh
sudo apt install --no-install-recommends golang-go pkg-config libgtk-3-dev libgtk-layer-shell-dev
mkdir -p bin
go test native_core.go native_core_test.go
go build -trimpath -o bin/nwg-dock .
```

If Wayland libraries are installed from backports, use matching development
packages (e.g. `libwayland-dev/trixie-backports` on Debian 13). The original
`make build` target also builds the fork. Normal nwg-dock styles, pinned apps
and installed image assets are still used; follow upstream instructions for
these assets on a new installation. FontAwesome is needed for the expansion
arrow (on Debian: `fonts-font-awesome`).

## Test in an existing Sway session

Stop any existing native manager before running another. The native manager
uses a runtime lock. From the checkout root:

```sh
./bin/nwg-dock -native-manager -status-config "$PWD/examples/native-status/status.json" -debug
```

This portable example shows the clock and an expandable date without external
status scripts. For permanent configuration, copy the example JSON and CSS to
`$XDG_CONFIG_HOME/nwg-dock/` (normally `~/.config/nwg-dock/`) and adjust as
needed. Restart the manager after configuration changes. Start the installed
binary with `-native-manager`; `-status-config` is optional and defaults to
`status.json` in the nwg-dock user configuration directory. CSS is loaded from
the JSON file's directory. Missing JSON disables the status panel, not the
application dock. `-debug` enables diagnostic state, geometry and crossing logs.

## Status configuration

`examples/native-status/i3blocks-status.json` is a more extensive example with
battery percentage, clock, keyboard layout, VPN, Bluetooth and Wi-Fi. It
requires user-provided scripts at the paths listed in the file; they are not
bundled. Do not enable it before supplying compatible handlers. The shared
`status.css` sets icon, arrow and text sizes independently.

- `rows`: visible rows; `expanded_rows`: additional rows shown by the arrow.
- `width`, `match_dock_width`, `bottom_margin`: status geometry.
- `enabled`, `intellihide`: status visibility and independent overlap handling.
- `items`: command, interval in seconds (0 = initial run and clicks only),
  optional color, markup and `percentage_only` settings. Commands execute via
  the shell and receive `BLOCK_BUTTON` on clicks (1, 2 or 3). Output follows
  the i3blocks convention: first line is text, optional third line is a hex
  color. Only use trusted config files and commands.

## Current limitations

Native manager currently fixes application dock position to the left, with
36px application icons, overlay layer and no workspace/launcher buttons.
Native status only supports the bottom-left layout. Very tall docks may overlap
one another. The existing optional external IPC Intellihide bridge is retained
for compatibility but not required in native mode. The pure Go core tests do
not cover GTK integration; test hover, menus, hotplug and layout in a real
Sway session before installing.

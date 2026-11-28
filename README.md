# nwg-dock

A native GTK3 dock for the Sway Wayland compositor, written in Go.

Based on [nwg-dock](https://github.com/nwg-piotr/nwg-dock),
extended with native Intellihide, multi-monitor support,
an integrated status panel and drag-hover window activation.

## Features

- Left-centered application dock.
- Pinned applications and running window indicators.
- Native multi-monitor management.
- Independent Intellihide for each output.
- Window overlap detection through Sway IPC.
- Automatic edge reveal.
- Optional expandable bottom-left status panel.
- Drag-hover activation of running applications.
- GTK3 CSS customization.
- No external dock controller required.
- Original nwg-dock mode retained for compatibility.

## Drag and drop

Drag a file onto the icon of a running application and
hold it there for 400 ms.

The dock activates the corresponding window and switches
to its workspace without interrupting the drag operation.

The dock also reveals itself during dragging when hidden
by Intellihide.

Files are dropped directly into the destination application,
not onto the dock itself.

## Dependencies

Runtime:

- Sway
- GTK3
- gtk-layer-shell
- FontAwesome (for the status panel expansion arrow)

Build:

- Go 1.23 or newer
- C compiler
- pkg-config
- GTK3 development libraries
- gtk-layer-shell development libraries

## Build

On Debian:

```sh
sudo apt install --no-install-recommends \
    golang-go pkg-config libgtk-3-dev libgtk-layer-shell-dev

git clone https://github.com/sabx-ru/nwg-dock.git
cd nwg-dock

go test native_core.go native_core_test.go
make build
```

If Wayland libraries come from Debian Backports,
install matching development packages.

## Installation

```sh
sudo make install
```

## Usage

Start the native manager:

```sh
nwg-dock -native-manager
```

Sway autostart:

```text
exec nwg-dock -native-manager
```

Debug mode:

```sh
nwg-dock -native-manager -debug
```

For all command-line options:

```sh
nwg-dock -h
```

## Configuration

User configuration directory:

```text
~/.config/nwg-dock/
```

- `style.css` — application dock appearance.
- `status.json` — optional status panel configuration.
- `status.css` — status panel appearance.

Status panel configuration examples are available in
`examples/native-status/`.

The status panel supports configurable rows, expandable
items, update intervals, click handlers and i3blocks-style
command output.

Without a status configuration, only the application
dock is displayed.

## Limitations

Native mode currently uses a left-centered application
dock and a bottom-left status panel.

Drag cancellation and file acceptance depend on the
compositor and the participating applications.

## License

MIT.

Original project by nwg-piotr.
Native extensions maintained by sabx-ru.

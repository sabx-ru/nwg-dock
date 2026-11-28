package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dlasky/gotk3-layershell/layershell"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	log "github.com/sirupsen/logrus"
)

type nativeItemConfig struct {
	PercentageOnly bool   `json:"percentage_only"`
	Command        string `json:"command"`
	Interval       int    `json:"interval"`
	Markup         bool   `json:"markup"`
	Color          string `json:"color"`
}
type nativeStatusConfig struct {
	Enabled        bool                        `json:"enabled"`
	Intellihide    bool                        `json:"intellihide"`
	Width          int                         `json:"width"`
	MatchDockWidth bool                        `json:"match_dock_width"`
	ExpandedRows   [][]string                  `json:"expanded_rows"`
	BottomMargin   int                         `json:"bottom_margin"`
	Rows           [][]string                  `json:"rows"`
	Items          map[string]nativeItemConfig `json:"items"`
}

func readNativeStatus() (nativeStatusConfig, error) {
	var c nativeStatusConfig
	p := *nativeStatusPath
	if p == "" {
		p = filepath.Join(configDirectory, "status.json")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if !c.Enabled {
		return c, nil
	}
	if c.Width < 32 || c.Width > 300 || c.BottomMargin < 0 || c.BottomMargin > 300 || len(c.Rows) == 0 || len(c.Rows) > 12 || len(c.ExpandedRows) > 12 {
		return c, fmt.Errorf("invalid status geometry/rows")
	}
	for _, row := range append(append([][]string{}, c.Rows...), c.ExpandedRows...) {
		if len(row) == 0 || len(row) > 6 {
			return c, fmt.Errorf("invalid status row")
		}
		for _, id := range row {
			item, ok := c.Items[id]
			if !ok || item.Command == "" || item.Interval < 0 {
				return c, fmt.Errorf("invalid item %q", id)
			}
		}
	}
	return c, nil
}

type nativeSurface struct {
	win, edge    *gtk.Window
	box          *gtk.EventBox
	bottom       bool
	margin, w, h int
	state        nativeState
	status       *nativeStatusView
	traceLast    string
	traceRegion  string
}

// Diagnostic output is enabled only with -debug. GTK state is read on its main thread.
func nativeTrace(format string, args ...interface{}) {
	if !*debug {
		return
	}
	fmt.Fprintf(os.Stderr, "%s native output=%s pid=%d %s\n", time.Now().Format("15:04:05.000"), *targetOutput, os.Getpid(), fmt.Sprintf(format, args...))
}
func (s *nativeSurface) traceName() string {
	if s.bottom {
		return "status"
	}
	return "applications"
}
func (s *nativeSurface) traceState(actual, desired bool) {
	if !*debug {
		return
	}
	inside, expanded, holds := false, false, false
	if s.status != nil {
		inside = s.status.pointer.inside
		expanded = s.status.expansion.open
		holds = s.status.expansion.holds(time.Now())
	}
	state := fmt.Sprintf("ws=%q covered=%t hover=%t leavePending=%t clearPending=%t inside=%t expanded=%t holds=%t desired=%t actual=%t size=%dx%d", s.state.Workspace, s.state.Covered, s.state.Hover, !s.state.LeaveAt.IsZero(), !s.state.ClearAt.IsZero(), inside, expanded, holds, desired, actual, s.w, s.h)
	if state != s.traceLast {
		s.traceLast = state
		nativeTrace("panel=%s STATE %s", s.traceName(), state)
	}
}
func (s *nativeSurface) traceCrossing(source, kind string, event *gdk.Event, accepted bool) {
	if !*debug {
		return
	}
	c := gdk.EventCrossingNewFromEvent(event)
	nativeTrace("panel=%s %s %s mode=%v detail=%v accepted=%t", s.traceName(), source, kind, c.Mode(), c.Detail(), accepted)
}

func newNativeSurface(win *gtk.Window, monitor *gdk.Monitor, bottom bool, margin int) (*nativeSurface, error) {
	s := &nativeSurface{win: win, bottom: bottom, margin: margin, state: nativeState{Visible: true}}
	edge, e := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if e != nil {
		return nil, e
	}
	box, e := gtk.EventBoxNew()
	if e != nil {
		return nil, e
	}
	s.edge = edge
	s.box = box
	layershell.InitForWindow(edge)
	layershell.SetMonitor(edge, monitor)
	layershell.SetLayer(edge, layershell.LAYER_SHELL_LAYER_TOP)
	layershell.SetAnchor(edge, layershell.LAYER_SHELL_EDGE_LEFT, true)
	if bottom {
		layershell.SetAnchor(edge, layershell.LAYER_SHELL_EDGE_BOTTOM, true)
		layershell.SetMargin(edge, layershell.LAYER_SHELL_EDGE_BOTTOM, margin)
	}
	layershell.SetExclusiveZone(edge, -1)
	edge.SetOpacity(0.005)
	edge.Add(box)
	edge.SetSizeRequest(3, 40)
	box.SetSizeRequest(3, 40)
	box.Connect("enter-notify-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		accepted := gdk.EventCrossingNewFromEvent(event).Mode() == gdk.CROSSING_NORMAL
		s.traceCrossing("edge", "ENTER", event, accepted)
		if accepted {
			s.state.pointer("EDGE", time.Now())
			s.apply()
		}
		return false
	})
	box.Connect("leave-notify-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		accepted := gdk.EventCrossingNewFromEvent(event).Mode() == gdk.CROSSING_NORMAL
		s.traceCrossing("edge", "LEAVE", event, accepted && !(s.status != nil && s.status.pointer.inside))
		if accepted {
			if s.status != nil && s.status.pointer.inside {
				return false
			}
			s.state.pointer("LEAVE", time.Now())
			s.apply()
		}
		return false
	})
	// Native application dock: use the surface-aware pointer classifier already
	// used by status. In particular, a top-level NONLINEAR_VIRTUAL leave is
	// a genuine subtree exit, not an internal crossing between buttons.
	if !bottom {
		registerDockDrag(box, nil, func() { s.apply() })
		registerDockDrag(win, nil, nil)
		win.AddEvents(int(gdk.POINTER_MOTION_MASK | gdk.ENTER_NOTIFY_MASK | gdk.LEAVE_NOTIFY_MASK))
		win.Connect("enter-notify-event", func(_ *gtk.Window, event *gdk.Event) bool {
			accepted := nativeStatusPointerEvent(win, event) == 1
			s.traceCrossing("window", "ENTER", event, accepted)
			if accepted {
				s.state.pointer("ENTER", time.Now())
				s.apply()
			}
			return false
		})
		win.Connect("leave-notify-event", func(_ *gtk.Window, event *gdk.Event) bool {
			accepted := nativeStatusPointerEvent(win, event) == -1
			s.traceCrossing("window", "LEAVE", event, accepted)
			if accepted {
				s.state.pointer("LEAVE", time.Now())
				s.apply()
			}
			return false
		})
		// Real motion inside the panel cancels a pending leave. Never use
		// cached global Wayland pointer coordinates to infer an exit.
		win.Connect("motion-notify-event", func(_ *gtk.Window, event *gdk.Event) bool {
			if s.state.Covered && (!s.state.Hover || !s.state.LeaveAt.IsZero()) && nativeStatusPointerEvent(win, event) == 1 {
				nativeTrace("panel=applications MOTION inside=true restoring hover")
				s.state.pointer("ENTER", time.Now())
				s.apply()
			}
			return false
		})
	}
	win.Connect("size-allocate", func() { s.geometry() })
	edge.ShowAll()
	return s, nil
}
func (s *nativeSurface) geometry() {
	if !s.win.IsVisible() {
		return
	}
	w, h := s.win.GetAllocatedWidth(), s.win.GetAllocatedHeight()
	if w < 16 || h < 16 || w >= 10000 || h >= 10000 || (w == s.w && h == s.h) {
		return
	}
	s.w = w
	s.h = h
	nativeTrace("panel=%s GEOMETRY %dx%d", s.traceName(), w, h)
	s.edge.SetSizeRequest(3, h)
	s.box.SetSizeRequest(3, h)
	if s.status != nil {
		s.edge.Resize(3, h)
	}
}
func (s *nativeSurface) apply() {
	visible := s.state.Visible || (s.status != nil && s.status.expansion.holds(time.Now())) || (!s.bottom && dockDragHolding())
	s.traceState(s.win.IsVisible(), visible)
	if visible && !s.win.IsVisible() {
		nativeTrace("panel=%s SHOW", s.traceName())
		s.win.ShowAll()
	} else if !visible && s.win.IsVisible() {
		nativeTrace("panel=%s HIDE", s.traceName())
		s.win.Hide()
	}
}

type nativeSnapshot struct {
	output *nativeOutput
	tree   nativeNode
	ok     bool
}

func startNativeDock(dock *gtk.Window, monitor *gdk.Monitor) error {
	if *marginTop != 0 || *marginBottom != 0 || *marginLeft != 0 || *marginRight != 0 {
		return fmt.Errorf("native mode currently requires zero main dock margins")
	}
	main, e := newNativeSurface(dock, monitor, false, 0)
	if e != nil {
		return e
	}
	surfaces := []*nativeSurface{main}
	var statusView *nativeStatusView
	c, e := readNativeStatus()
	if e != nil {
		log.Warnf("native status disabled: %v", e)
	} else if c.Enabled {
		status, e := buildNativeStatus(c, monitor)
		if e != nil {
			return e
		}
		statusView = status
		if c.Intellihide {
			s, e := newNativeSurface(status.win, monitor, true, c.BottomMargin)
			if e != nil {
				return e
			}
			s.status = status
			status.surface = s
			surfaces = append(surfaces, s)
		}
		status.win.ShowAll()
	}
	updates := make(chan nativeSnapshot, 1)
	go func() {
		ipc := nativeIPC{}
		defer ipc.close()
		for {
			var outputs []nativeOutput
			var snap nativeSnapshot
			if ipc.query(3, &outputs) == nil {
				for i := range outputs {
					if outputs[i].Active && outputs[i].Name == *targetOutput {
						snap.output = &outputs[i]
						break
					}
				}
				if snap.output != nil && ipc.query(4, &snap.tree) == nil {
					snap.ok = true
				}
			}
			select {
			case <-updates:
			default:
			}
			select {
			case updates <- snap:
			default:
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	lastGood := time.Now()
	stale := false
	glib.TimeoutAdd(40, func() bool {
		now := time.Now()
		for _, s := range surfaces {
			s.geometry()
		}
		if statusView != nil {
			if c.MatchDockWidth && main.w > 0 {
				statusView.setWidth(main.w)
			}
			if statusView.expansion.expired(now) {
				statusView.setExpanded(false)
			}
		}
		select {
		case snap := <-updates:
			if snap.ok {
				lastGood = now
				out := nativeFind(&snap.tree, "output", snap.output.Name)
				var ws *nativeNode
				if out != nil {
					ws = nativeFind(out, "workspace", snap.output.CurrentWorkspace)
				}
				for _, s := range surfaces {
					if s.w > 0 && s.h > 0 {
						if s.status != nil && s.state.Workspace != "" && s.state.Workspace != snap.output.CurrentWorkspace {
							s.status.setExpanded(false)
						}
						region := nativeRegion(*snap.output, s.w, s.h, s.bottom, s.margin)
						covered := ws != nil && nativeCovered(ws, region)
						if *debug {
							info := fmt.Sprintf("outputRect=%+v region=%+v workspace=%q workspaceFound=%t covered=%t", snap.output.Rect, region, snap.output.CurrentWorkspace, ws != nil, covered)
							if info != s.traceRegion {
								s.traceRegion = info
								nativeTrace("panel=%s OVERLAP %s", s.traceName(), info)
							}
						}
						s.state.update(covered, snap.output.CurrentWorkspace, now)
					}
				}
			}
		default:
		}
		isStale := now.Sub(lastGood) > 3*time.Second
		if isStale != stale {
			stale = isStale
			nativeTrace("IPC stale=%t age=%s", stale, now.Sub(lastGood).Round(time.Millisecond))
		}
		for _, s := range surfaces {
			if isStale {
				s.state.Covered = false
				s.state.ClearAt = time.Time{}
				s.state.reset()
			}
			if s.status != nil {
				s.status.pointer.keep(&s.state, now)
			}
			s.state.tick(now)
			s.apply()
		}
		return true
	})
	return nil
}

var nativeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var nativePercentage = regexp.MustCompile(`([0-9]+|--)%`)

type nativeResult struct {
	text, color string
	failed      bool
}
type nativeButton struct {
	label  *gtk.Label
	cfg    nativeItemConfig
	busy   bool
	next   time.Time
	result chan nativeResult
}

func (b *nativeButton) run(button uint) {
	if b.busy {
		return
	}
	b.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", b.cfg.Command)
		env := []string{}
		for _, s := range os.Environ() {
			if !strings.HasPrefix(s, "BLOCK_BUTTON=") {
				env = append(env, s)
			}
		}
		if button != 0 {
			env = append(env, fmt.Sprintf("BLOCK_BUTTON=%d", button))
		}
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			b.result <- nativeResult{text: "?", color: "#ed8796", failed: true}
			return
		}
		lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
		r := nativeResult{text: strings.TrimSuffix(lines[0], "\r"), color: b.cfg.Color}
		if len(lines) > 2 && nativeColor.MatchString(strings.TrimSpace(lines[2])) {
			r.color = strings.TrimSpace(lines[2])
		}
		if !nativeColor.MatchString(r.color) {
			r.color = "#cad3f5"
		}
		b.result <- r
	}()
}
func (b *nativeButton) tick(now time.Time) {
	select {
	case r := <-b.result:
		b.busy = false
		text := r.text
		if b.cfg.PercentageOnly && !r.failed {
			text = nativePercentage.FindString(text)
			if text == "" {
				text = "?"
			}
		}
		if !b.cfg.Markup || r.failed {
			text = html.EscapeString(text)
		}
		b.label.SetMarkup(fmt.Sprintf(`<span foreground="%s">%s</span>`, r.color, text))
		if b.cfg.Interval > 0 {
			b.next = now.Add(time.Duration(b.cfg.Interval) * time.Second)
		} else {
			b.next = time.Time{}
		}
	default:
	}
	if !b.busy && !b.next.IsZero() && !now.Before(b.next) {
		b.run(0)
	}
}

type nativeStatusView struct {
	surface            *nativeSurface
	pointer            nativeStatusPointer
	win                *gtk.Window
	outer, expandedBox *gtk.Box
	toggleLabel        *gtk.Label
	expansion          nativeExpansion
	width              int
}

func (v *nativeStatusView) handlePointer(event *gdk.Event) bool {
	now := time.Now()
	classification := nativeStatusPointerEvent(v.win, event)
	if *debug && ((classification == 1 && !v.pointer.inside) || classification == -1) {
		nativeTrace("panel=status POINTER classification=%d insideBefore=%t expanded=%t", classification, v.pointer.inside, v.expansion.open)
	}
	switch classification {
	case 1:
		v.pointer.inside = true
		v.expansion.enter()
		if v.surface != nil {
			v.pointer.keep(&v.surface.state, now)
			v.surface.apply()
		}
	case -1:
		if !v.pointer.inside {
			return false
		}
		v.pointer.inside = false
		v.expansion.leave(now)
		if v.surface != nil {
			v.surface.state.pointer("LEAVE", now)
			v.surface.apply()
		}
	}
	return false // don't consume button events
}

func (v *nativeStatusView) trackButton(b *gtk.Button) {
	b.AddEvents(int(gdk.POINTER_MOTION_MASK | gdk.ENTER_NOTIFY_MASK | gdk.LEAVE_NOTIFY_MASK))
	for _, signal := range []string{"enter-notify-event", "leave-notify-event", "motion-notify-event"} {
		b.Connect(signal, func(_ *gtk.Button, event *gdk.Event) bool { return v.handlePointer(event) })
	}
}

func (v *nativeStatusView) setWidth(w int) {
	if w < 16 || w == v.width {
		return
	}
	v.width = w
	v.outer.SetSizeRequest(w, -1)
	if v.win.IsVisible() {
		v.win.Resize(w, 1)
	}
}
func (v *nativeStatusView) setExpanded(open bool) {
	if v.expandedBox == nil || v.expansion.open == open {
		return
	}
	v.expansion.set(open)
	nativeTrace("panel=status EXPAND open=%t", open)
	if open {
		v.toggleLabel.SetText("\uf107")
		v.expandedBox.Show()
	} else {
		v.toggleLabel.SetText("\uf106")
		v.expandedBox.Hide()
	}
	if v.win.IsVisible() {
		v.win.Resize(v.width, 1)
	}
}
func buildNativeStatus(c nativeStatusConfig, monitor *gdk.Monitor) (*nativeStatusView, error) {
	win, e := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if e != nil {
		return nil, e
	}
	_ = win.SetProperty("name", "nwg-status")
	layershell.InitForWindow(win)
	layershell.SetMonitor(win, monitor)
	layershell.SetLayer(win, layershell.LAYER_SHELL_LAYER_OVERLAY)
	layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_LEFT, true)
	layershell.SetAnchor(win, layershell.LAYER_SHELL_EDGE_BOTTOM, true)
	layershell.SetMargin(win, layershell.LAYER_SHELL_EDGE_BOTTOM, c.BottomMargin)
	layershell.SetExclusiveZone(win, -1)
	outer, e := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if e != nil {
		return nil, e
	}
	_ = outer.SetProperty("name", "status-box")
	outer.SetSizeRequest(c.Width, -1)
	win.Add(outer)
	provider, _ := gtk.CssProviderNew()
	cssDir := configDirectory
	if *nativeStatusPath != "" {
		cssDir = filepath.Dir(*nativeStatusPath)
	}
	if err := provider.LoadFromPath(filepath.Join(cssDir, "status.css")); err == nil {
		screen, _ := gdk.ScreenGetDefault()
		gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION+1)
	}
	view := &nativeStatusView{win: win, outer: outer, width: c.Width}
	win.AddEvents(int(gdk.POINTER_MOTION_MASK | gdk.ENTER_NOTIFY_MASK | gdk.LEAVE_NOTIFY_MASK))
	for _, signal := range []string{"enter-notify-event", "leave-notify-event", "motion-notify-event"} {
		win.Connect(signal, func(_ *gtk.Window, event *gdk.Event) bool { return view.handlePointer(event) })
	}
	win.Connect("hide", func() { view.pointer.inside = false })
	buttons := []*nativeButton{}
	addRows := func(parent *gtk.Box, rows [][]string) {
		for _, row := range rows {
			r, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
			r.SetHomogeneous(true)
			parent.PackStart(r, false, false, 0)
			for _, id := range row {
				item := c.Items[id]
				button, _ := gtk.ButtonNew()
				view.trackButton(button)
				label, _ := gtk.LabelNew("…")
				button.Add(label)
				r.PackStart(button, true, true, 0)
				b := &nativeButton{label: label, cfg: item, result: make(chan nativeResult, 1)}
				buttons = append(buttons, b)
				button.Connect("button-press-event", func(_ *gtk.Button, event *gdk.Event) bool {
					n := gdk.EventButtonNewFromEvent(event).Button()
					if n == 1 || n == 2 || n == 3 {
						b.run(uint(n))
						return true
					}
					return false
				})
				b.run(0)
			}
		}
	}
	if len(c.ExpandedRows) > 0 {
		extra, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
		if err != nil {
			return nil, err
		}
		_ = extra.SetProperty("name", "status-expanded")
		view.expandedBox = extra
		outer.PackStart(extra, false, false, 0)
		addRows(extra, c.ExpandedRows)
		// Populate children once, then keep this parent hidden through future ShowAll calls.
		extra.ShowAll()
		if err := extra.SetProperty("no-show-all", true); err != nil {
			return nil, err
		}
		extra.Hide()
		toggle, err := gtk.ButtonNew()
		if err != nil {
			return nil, err
		}
		label, err := gtk.LabelNew("\uf106")
		if err != nil {
			return nil, err
		}
		_ = toggle.SetProperty("name", "status-toggle")
		view.toggleLabel = label
		toggle.Add(label)
		outer.PackStart(toggle, false, false, 0)
		view.trackButton(toggle)
		toggle.Connect("clicked", func() { view.setExpanded(!view.expansion.open) })
	}
	addRows(outer, c.Rows)
	glib.TimeoutAdd(100, func() bool {
		now := time.Now()
		for _, b := range buttons {
			b.tick(now)
		}
		return true
	})
	return view, nil
}

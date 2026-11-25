package main

// The Intellihide bridge is compiled only into the local nwg-dock binary.
// GDK crossing events must be filtered by their *originating surface* and
// crossing detail, not by gdk_window_get_device_position(): on Wayland that
// query can refer to cached pointer state after the pointer has left.

/*
#cgo pkg-config: gdk-3.0
#include <gdk/gdk.h>

static gboolean intellihide_event_is_on_surface(GdkEvent *event, GdkWindow *surface) {
    return event != NULL && surface != NULL && gdk_event_get_window(event) == surface;
}

// Status buttons have input-only child windows. Translate their event coordinates
// into the entire panel's surface; internal crossings are not panel exits.
// 1 = inside, -1 = actual exit, 0 = ignore.
static int intellihide_status_pointer(GdkEvent *event, GdkWindow *surface) {
    if (!event || !surface) return 0;
    GdkEventType type = gdk_event_get_event_type(event);
    if (type != GDK_ENTER_NOTIFY && type != GDK_LEAVE_NOTIFY &&
        type != GDK_MOTION_NOTIFY) return 0;
    GdkWindow *origin = gdk_event_get_window(event);
    if (!origin) return 0;
    if (type == GDK_ENTER_NOTIFY || type == GDK_LEAVE_NOTIFY) {
        if (event->crossing.mode != GDK_CROSSING_NORMAL) return 0;
        if (type == GDK_LEAVE_NOTIFY && origin == surface) {
            if (event->crossing.detail == GDK_NOTIFY_INFERIOR) return 0;
            // A virtual crossing may be internal; use panel coordinates for it.
            // NONLINEAR_VIRTUAL is a real departure from this subtree.
            if (event->crossing.detail != GDK_NOTIFY_VIRTUAL) return -1;
        }
    }
    gdouble x, y;
    if (!gdk_event_get_coords(event, &x, &y)) return 0;
    GdkWindow *w = origin;
    while (w != surface) {
        if (!w) return 0;
        GdkWindow *parent = gdk_window_get_parent(w);
        if (!parent || parent == w) return 0;
        gdouble px, py;
        gdk_window_coords_to_parent(w, x, y, &px, &py);
        x = px; y = py; w = parent;
    }
    gboolean inside = x >= 0 && y >= 0 &&
        x < gdk_window_get_width(surface) && y < gdk_window_get_height(surface);
    if (type == GDK_LEAVE_NOTIFY) return inside ? 0 : -1;
    return inside ? 1 : 0;
}
*/
import "C"

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strings"
	"unsafe"

	"github.com/dlasky/gotk3-layershell/layershell"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	log "github.com/sirupsen/logrus"
)

var intellihideIPC = flag.String("intellihide-ipc", "", "private Unix socket for SHOW/HIDE and GTK geometry/pointer events")

const intellihideEdgeWidth = 3

// GTK may propagate a crossing from a button, image or other child widget
// to the top-level callback. Only crossings whose original GDK window is
// the dock's own top-level surface can mean entering/leaving the whole dock.
func isRealDockCrossing(dock *gtk.Window, event *gdk.Event) bool {
	crossing := gdk.EventCrossingNewFromEvent(event)
	if crossing.Mode() != gdk.CROSSING_NORMAL {
		return false
	}
	switch crossing.Detail() {
	case gdk.NOTIFY_INFERIOR, gdk.NOTIFY_VIRTUAL, gdk.NOTIFY_NONLINEAR_VIRTUAL:
		return false
	}
	window, err := dock.GetWindow()
	if err != nil || window == nil {
		return false
	}
	return C.intellihide_event_is_on_surface(
		(*C.GdkEvent)(unsafe.Pointer(event.Native())),
		(*C.GdkWindow)(unsafe.Pointer(window.Native())),
	) != 0
}

func startIntellihide(dock *gtk.Window, monitor *gdk.Monitor) {
	conn, err := net.Dial("unix", *intellihideIPC)
	if err != nil {
		log.Fatalf("intellihide: can't connect to controller: %v", err)
	}

	outgoing := make(chan string, 128)
	publish := func(message string) {
		select {
		case outgoing <- message:
		default:
			log.Warn("intellihide: controller not reading events")
		}
	}
	go func() {
		for msg := range outgoing {
			if _, err := fmt.Fprintln(conn, msg); err != nil {
				log.Warnf("intellihide: controller connection lost: %v", err)
				return
			}
		}
	}()

	edge, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatalf("intellihide: can't create edge hotspot: %v", err)
	}
	layershell.InitForWindow(edge)
	layershell.SetMonitor(edge, monitor)
	layershell.SetLayer(edge, layershell.LAYER_SHELL_LAYER_TOP)
	layershell.SetAnchor(edge, layershell.LAYER_SHELL_EDGE_LEFT, true)
	layershell.SetExclusiveZone(edge, -1)
	edge.SetOpacity(0.005)
	box, err := gtk.EventBoxNew()
	if err != nil {
		log.Fatalf("intellihide: can't create hotspot event box: %v", err)
	}
	edge.Add(box)
	box.Connect("enter-notify-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		crossing := gdk.EventCrossingNewFromEvent(event)
		if crossing.Mode() == gdk.CROSSING_NORMAL {
			publish("EDGE")
		}
		return false
	})
	// A cursor can leave the 3px hotspot without ever entering the actual
	// dock (e.g. a fast diagonal movement). This is a genuine mouse exit too.
	// When it moves into the dock, dock ENTER cancels the delayed LEAVE.
	box.Connect("leave-notify-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		crossing := gdk.EventCrossingNewFromEvent(event)
		if crossing.Mode() == gdk.CROSSING_NORMAL {
			publish("LEAVE")
		}
		return false
	})

	lastW, lastH := 0, 0
	updateGeometry := func() {
		// GTK may briefly allocate the hidden window at 1x1. Keep the last
		// real size (including the hotspot's height) until the dock reappears.
		if !dock.IsVisible() {
			return
		}
		w, h := dock.GetAllocatedWidth(), dock.GetAllocatedHeight()
		if w < 16 || h < 16 || (w == lastW && h == lastH) {
			return
		}
		lastW, lastH = w, h
		edge.SetSizeRequest(intellihideEdgeWidth, h)
		box.SetSizeRequest(intellihideEdgeWidth, h)
		publish(fmt.Sprintf("GEOM %d %d", w, h))
	}

	edge.SetSizeRequest(intellihideEdgeWidth, 40)
	box.SetSizeRequest(intellihideEdgeWidth, 40)
	edge.ShowAll()

	dock.Connect("size-allocate", func() {
		updateGeometry()
	})
	dock.Connect("enter-notify-event", func(_ *gtk.Window, event *gdk.Event) bool {
		if isRealDockCrossing(dock, event) {
			publish("ENTER")
		}
		return false
	})
	dock.Connect("leave-notify-event", func(_ *gtk.Window, event *gdk.Event) bool {
		if isRealDockCrossing(dock, event) {
			publish("LEAVE")
		}
		return false
	})

	glib.TimeoutAdd(uint(120), func() bool {
		updateGeometry()
		return false
	})

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			command := strings.TrimSpace(scanner.Text())
			if command != "SHOW" && command != "HIDE" {
				continue
			}
			glib.TimeoutAdd(uint(0), func() bool {
				if command == "SHOW" && !dock.IsVisible() {
					dock.ShowAll()
				} else if command == "HIDE" && dock.IsVisible() {
					dock.Hide()
				}
				return false
			})
		}
		// A disconnected controller should not leave the dock hidden.
		glib.TimeoutAdd(uint(0), func() bool {
			if !dock.IsVisible() {
				dock.ShowAll()
			}
			return false
		})
		log.Warn("intellihide: controller disconnected; keeping dock shown")
	}()
}

// Only the status panel uses this classifier; the main dock stays unchanged.
func nativeStatusPointerEvent(win *gtk.Window, event *gdk.Event) int {
	surface, err := win.GetWindow()
	if err != nil || surface == nil || !win.IsVisible() {
		return 0
	}
	return int(C.intellihide_status_pointer(
		(*C.GdkEvent)(unsafe.Pointer(event.Native())),
		(*C.GdkWindow)(unsafe.Pointer(surface.Native()))))
}

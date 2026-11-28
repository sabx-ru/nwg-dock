package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdint.h>

extern void dockDragEvent(uintptr_t token, int event);

static gboolean dock_drag_motion(GtkWidget *w, GdkDragContext *ctx,
    gint x, gint y, guint time, gpointer data) {
    gdk_drag_status(ctx, 0, time); // never accept the payload
    dockDragEvent((uintptr_t)data, 1);
    return TRUE;
}
static void dock_drag_leave(GtkWidget *w, GdkDragContext *ctx,
    guint time, gpointer data) {
    dockDragEvent((uintptr_t)data, 2);
}
static gboolean dock_drag_drop(GtkWidget *w, GdkDragContext *ctx,
    gint x, gint y, guint time, gpointer data) {
    dockDragEvent((uintptr_t)data, 2);
    return FALSE;
}
static void dock_drag_unmap(GtkWidget *w, gpointer data) {
    dockDragEvent((uintptr_t)data, 2);
}
static void dock_drag_destroy(GtkWidget *w, gpointer data) {
    dockDragEvent((uintptr_t)data, 3);
}
static void dock_drag_register(uintptr_t ptr, uintptr_t token) {
    GtkWidget *w = GTK_WIDGET((gpointer)ptr);
    gpointer data = (gpointer)token;
    gtk_drag_dest_set(w, 0, NULL, 0, 0);
    gtk_drag_dest_set_track_motion(w, TRUE);
    g_signal_connect(w, "drag-motion", G_CALLBACK(dock_drag_motion), data);
    g_signal_connect(w, "drag-leave", G_CALLBACK(dock_drag_leave), data);
    g_signal_connect(w, "drag-drop", G_CALLBACK(dock_drag_drop), data);
    g_signal_connect(w, "unmap", G_CALLBACK(dock_drag_unmap), data);
    g_signal_connect(w, "destroy", G_CALLBACK(dock_drag_destroy), data);
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"

	"github.com/gotk3/gotk3/glib"
	"github.com/joshuarubin/go-sway"
	log "github.com/sirupsen/logrus"
)

// All hover state belongs to the GTK main thread of one native worker.
const dockDragDelay = 400 * time.Millisecond
const dockDragGrace = 180 * time.Millisecond

type dockDragTarget struct {
	ids    []int64
	reveal func()
}

var dockDragTargets = map[uintptr]dockDragTarget{}
var dockDragNext, dockDragActive uintptr
var dockDragTimer glib.SourceHandle
var dockDragUntil time.Time

func registerDockDrag(w interface{ Native() uintptr }, ids []int64, reveal func()) {
	dockDragNext++
	token := dockDragNext
	dockDragTargets[token] = dockDragTarget{ids: ids, reveal: reveal}
	C.dock_drag_register(C.uintptr_t(w.Native()), C.uintptr_t(token))
}

func dockDragHolding() bool {
	return dockDragActive != 0 || time.Now().Before(dockDragUntil)
}

func dockDragCancelTimer() {
	if dockDragTimer != 0 {
		glib.SourceRemove(dockDragTimer)
		dockDragTimer = 0
	}
}

//export dockDragEvent
func dockDragEvent(raw C.uintptr_t, event C.int) {
	token := uintptr(raw)
	target, ok := dockDragTargets[token]
	if !ok {
		return
	}
	if event == 1 {
		if dockDragActive == token {
			return
		}
		dockDragCancelTimer()
		dockDragActive = token
		dockDragUntil = time.Time{}
		nativeTrace("DND enter target=%d windows=%v", token, target.ids)
		if target.reveal != nil {
			target.reveal()
		}
		if len(target.ids) != 0 {
			dockDragTimer = glib.TimeoutAdd(uint(dockDragDelay/time.Millisecond), func() bool {
				dockDragTimer = 0
				if dockDragActive == token {
					ids := append([]int64(nil), target.ids...)
					go dockDragFocus(ids)
				}
				return false
			})
		}
	} else {
		if dockDragActive == token {
			dockDragCancelTimer()
			dockDragActive = 0
			dockDragUntil = time.Now().Add(dockDragGrace)
			nativeTrace("DND leave target=%d", token)
		}
		if event == 3 {
			delete(dockDragTargets, token)
		}
	}
}

// Choose the most recently focused live window by following Sway's focus
// stacks. No app_id interpolation, shell command or file path is involved.
type dockDragNode struct {
	ID       int64
	Focus    []int64
	Nodes    []dockDragNode
	Floating []dockDragNode `json:"floating_nodes"`
}

func (n *dockDragNode) pick(ids map[int64]bool) int64 {
	if ids[n.ID] {
		return n.ID
	}
	children := append(append([]dockDragNode(nil), n.Nodes...), n.Floating...)
	seen := map[int64]bool{}
	for _, id := range n.Focus {
		for i := range children {
			if children[i].ID == id {
				seen[id] = true
				if found := children[i].pick(ids); found != 0 {
					return found
				}
				break
			}
		}
	}
	for i := range children {
		if !seen[children[i].ID] {
			if found := children[i].pick(ids); found != 0 {
				return found
			}
		}
	}
	return 0
}

func dockDragFocus(ids []int64) {
	id := ids[0]
	if len(ids) > 1 {
		wanted := map[int64]bool{}
		for _, v := range ids {
			wanted[v] = true
		}
		var tree dockDragNode
		ipc := nativeIPC{}
		err := ipc.query(4, &tree)
		ipc.close()
		if err != nil {
			log.Warnf("DND tree: %v", err)
			return
		}
		id = tree.pick(wanted)
		if id == 0 {
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	client, err := sway.New(ctx)
	if err != nil {
		log.Warnf("DND IPC: %v", err)
		return
	}
	if _, err = client.RunCommand(ctx, fmt.Sprintf("[con_id=%d] focus", id)); err != nil {
		log.Warnf("DND focus: %v", err)
		return
	}
	nativeTrace("DND focus con_id=%d", id)
}

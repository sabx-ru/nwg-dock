package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

type nativeRect struct{ X, Y, Width, Height int }

func (a nativeRect) overlaps(b nativeRect) bool {
	return a.Width > 0 && a.Height > 0 && b.Width > 0 && b.Height > 0 && a.X < b.X+b.Width && a.X+a.Width > b.X && a.Y < b.Y+b.Height && a.Y+a.Height > b.Y
}

type nativeOutput struct {
	Name             string
	Active           bool
	Rect             nativeRect
	CurrentWorkspace string `json:"current_workspace"`
}
type nativeNode struct {
	Type, Name           string
	AppID                *string `json:"app_id"`
	Window               *int64
	Visible              *bool
	Rect                 nativeRect
	WindowProperties     struct{ Class string } `json:"window_properties"`
	Nodes, FloatingNodes []nativeNode
}

// Sway uses snake_case for floating_nodes; use explicit JSON tags.
func (n *nativeNode) UnmarshalJSON(b []byte) error {
	type alias nativeNode
	var x struct {
		*alias
		Float []nativeNode `json:"floating_nodes"`
	}
	x.alias = (*alias)(n)
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	n.FloatingNodes = x.Float
	return nil
}
func nativeFind(n *nativeNode, typ, name string) *nativeNode {
	if n.Type == typ && n.Name == name {
		return n
	}
	for i := range n.Nodes {
		if v := nativeFind(&n.Nodes[i], typ, name); v != nil {
			return v
		}
	}
	for i := range n.FloatingNodes {
		if v := nativeFind(&n.FloatingNodes[i], typ, name); v != nil {
			return v
		}
	}
	return nil
}
func nativeCovered(n *nativeNode, r nativeRect) bool {
	if n.Visible != nil && !*n.Visible {
		return false
	}
	if (n.Type == "con" || n.Type == "floating_con") && (n.AppID != nil || n.Window != nil || n.WindowProperties.Class != "") && n.Rect.overlaps(r) {
		return true
	}
	for i := range n.Nodes {
		if nativeCovered(&n.Nodes[i], r) {
			return true
		}
	}
	for i := range n.FloatingNodes {
		if nativeCovered(&n.FloatingNodes[i], r) {
			return true
		}
	}
	return false
}
func nativeRegion(o nativeOutput, w, h int, bottom bool, margin int) nativeRect {
	y := o.Rect.Y + (o.Rect.Height-h)/2
	if bottom {
		y = o.Rect.Y + o.Rect.Height - h - margin
	}
	return nativeRect{o.Rect.X, y, w, h}
}

type nativeIPC struct{ conn net.Conn }

func (s *nativeIPC) close() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}
func (s *nativeIPC) query(kind uint32, out interface{}) (err error) {
	defer func() {
		if err != nil {
			s.close()
		}
	}()
	if s.conn == nil {
		p := os.Getenv("SWAYSOCK")
		if p == "" {
			return fmt.Errorf("SWAYSOCK is not set")
		}
		s.conn, err = net.DialTimeout("unix", p, 2*time.Second)
		if err != nil {
			return err
		}
	}
	if err = s.conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	h := make([]byte, 14)
	copy(h, "i3-ipc")
	binary.LittleEndian.PutUint32(h[10:], kind)
	if _, err = s.conn.Write(h); err != nil {
		return err
	}
	if _, err = io.ReadFull(s.conn, h); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(h[6:])
	if string(h[:6]) != "i3-ipc" || binary.LittleEndian.Uint32(h[10:]) != kind || size > 64*1024*1024 {
		return fmt.Errorf("invalid Sway IPC reply")
	}
	b := make([]byte, size)
	if _, err = io.ReadFull(s.conn, b); err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

type nativeState struct {
	Covered, Hover, Visible    bool
	LeaveAt, ClearAt, LastShow time.Time
	Workspace                  string
}

func (s *nativeState) apply(now time.Time) {
	v := !s.Covered || s.Hover
	if v && !s.Visible {
		s.LastShow = now
	}
	s.Visible = v
}
func (s *nativeState) reset() { s.Hover = false; s.LeaveAt = time.Time{} }
func (s *nativeState) update(covered bool, workspace string, now time.Time) {
	if workspace != s.Workspace {
		s.Workspace = workspace
		s.reset()
		s.ClearAt = time.Time{}
	}
	if covered {
		s.ClearAt = time.Time{}
		if !s.Covered {
			s.Covered = true
			s.reset()
		}
	} else if s.Covered {
		if s.ClearAt.IsZero() {
			s.ClearAt = now.Add(140 * time.Millisecond)
		} else if !now.Before(s.ClearAt) {
			s.Covered = false
			s.ClearAt = time.Time{}
			s.reset()
		}
	}
	s.tick(now)
}
func (s *nativeState) pointer(event string, now time.Time) {
	if !s.Covered {
		s.reset()
	} else {
		switch event {
		case "EDGE":
			s.Hover = true
			s.LeaveAt = time.Time{}
		case "ENTER":
			if s.Visible {
				s.Hover = true
				s.LeaveAt = time.Time{}
			}
		case "LEAVE":
			if s.Hover {
				s.LeaveAt = now.Add(100 * time.Millisecond)
				guard := s.LastShow.Add(250 * time.Millisecond)
				if guard.After(s.LeaveAt) {
					s.LeaveAt = guard
				}
			}
		}
	}
	s.apply(now)
}
func (s *nativeState) tick(now time.Time) {
	if !s.LeaveAt.IsZero() && !now.Before(s.LeaveAt) {
		s.reset()
	}
	s.apply(now)
}

// Expansion pins only the status block while its controls are being used.
// Crossing child widgets is filtered in the GTK layer before calling leave.
type nativeExpansion struct {
	open        bool
	closeAt     time.Time
	settleUntil time.Time
}

func (e *nativeExpansion) set(open bool) {
	if e.open && !open {
		e.settleUntil = time.Now().Add(500 * time.Millisecond)
	}
	if open {
		e.settleUntil = time.Time{}
	}
	e.open = open
	e.closeAt = time.Time{}
}
func (e *nativeExpansion) holds(now time.Time) bool { return e.open || now.Before(e.settleUntil) }
func (e *nativeExpansion) enter()                   { e.closeAt = time.Time{} }
func (e *nativeExpansion) leave(now time.Time) {
	if e.open {
		e.closeAt = now.Add(300 * time.Millisecond)
	}
}
func (e *nativeExpansion) expired(now time.Time) bool {
	return e.open && !e.closeAt.IsZero() && !now.Before(e.closeAt)
}

// Actual status-panel hover survives overlap changes, which intentionally reset
// the generic dock state. The edge sensor must not cancel a confirmed enter.
type nativeStatusPointer struct{ inside bool }

func (p *nativeStatusPointer) keep(s *nativeState, now time.Time) {
	if p.inside {
		s.pointer("EDGE", now)
	}
}

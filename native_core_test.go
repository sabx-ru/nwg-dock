package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNativeCoverage(t *testing.T) {
	var tree nativeNode
	err := json.Unmarshal([]byte(`{"type":"workspace","floating_nodes":[{"type":"floating_con","app_id":"firefox","rect":{"x":0,"y":90,"width":80,"height":100}}]}`), &tree)
	if err != nil {
		t.Fatal(err)
	}
	if !nativeCovered(&tree, nativeRect{0, 100, 72, 80}) || nativeCovered(&tree, nativeRect{0, 300, 72, 80}) {
		t.Fatal("floating overlap")
	}
}
func TestNativeState(t *testing.T) {
	n := time.Now()
	s := nativeState{Visible: true}
	s.update(true, "1", n)
	if s.Visible {
		t.Fatal("covered")
	}
	s.pointer("EDGE", n)
	if !s.Visible {
		t.Fatal("edge")
	}
	s.pointer("LEAVE", n)
	s.tick(n.Add(200 * time.Millisecond))
	if !s.Visible {
		t.Fatal("show guard")
	}
	s.tick(n.Add(260 * time.Millisecond))
	if s.Visible {
		t.Fatal("leave")
	}
	s.update(false, "1", n.Add(time.Second))
	s.update(false, "1", n.Add(1200*time.Millisecond))
	if !s.Visible {
		t.Fatal("clear")
	}
	s.update(true, "1", n.Add(2*time.Second))
	s.pointer("EDGE", n.Add(2*time.Second))
	s.update(true, "2", n.Add(3*time.Second))
	if s.Visible {
		t.Fatal("workspace reset")
	}
}
func TestNativeRegion(t *testing.T) {
	o := nativeOutput{Rect: nativeRect{-100, 1200, 1920, 1080}}
	if r := nativeRegion(o, 72, 100, true, 10); r != (nativeRect{-100, 2170, 72, 100}) {
		t.Fatal(r)
	}
	if r := nativeRegion(o, 72, 100, false, 0); r.Y != 1690 {
		t.Fatal(r)
	}
}

func TestNativeExpansion(t *testing.T) {
	n := time.Now()
	var e nativeExpansion
	e.set(true)
	e.leave(n)
	if e.expired(n.Add(299 * time.Millisecond)) {
		t.Fatal("premature collapse")
	}
	e.enter()
	if e.expired(n.Add(time.Second)) {
		t.Fatal("re-enter must cancel")
	}
	e.leave(n)
	if !e.expired(n.Add(300 * time.Millisecond)) {
		t.Fatal("must collapse")
	}
	e.set(false)
	if e.expired(n.Add(time.Second)) {
		t.Fatal("collapsed")
	}
}

func TestNativeStatusPointer(t *testing.T) {
	now := time.Now()
	s := nativeState{Visible: true}
	p := nativeStatusPointer{inside: true}
	s.update(true, "1", now)
	p.keep(&s, now)
	if !s.Visible || !s.Hover {
		t.Fatal("overlap change must preserve actual status hover")
	}
	s.pointer("LEAVE", now.Add(time.Millisecond))
	p.keep(&s, now.Add(300*time.Millisecond))
	s.tick(now.Add(300 * time.Millisecond))
	if !s.Visible || !s.LeaveAt.IsZero() {
		t.Fatal("inside must cancel pending leave")
	}
	s.update(true, "2", now.Add(time.Second))
	p.keep(&s, now.Add(time.Second))
	if !s.Visible {
		t.Fatal("workspace transition lost actual hover")
	}
	p.inside = false
	s.pointer("LEAVE", now.Add(2*time.Second))
	p.keep(&s, now.Add(3*time.Second))
	s.tick(now.Add(3 * time.Second))
	if s.Visible {
		t.Fatal("true exit must hide covered status")
	}
}

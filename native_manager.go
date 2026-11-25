package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var nativeManager = flag.Bool("native-manager", false, "manage native docks on all active Sway outputs")
var nativeChild = flag.Bool("native-child", false, "native Intellihide and status block for one output")
var nativeStatusPath = flag.String("status-config", "", "path to status JSON (default: nwg-dock/status.json)")

type nativeWorker struct {
	cmd      *exec.Cmd
	done     chan error
	dir      string
	stopping bool
}

func (w *nativeWorker) stop() {
	if !w.stopping {
		w.stopping = true
		_ = w.cmd.Process.Signal(syscall.SIGTERM)
	}
}
func nativeEnv(key, value string) []string {
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, key+"=") {
			env = append(env, v)
		}
	}
	return append(env, key+"="+value)
}
func runNativeManager() error {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		return fmt.Errorf("XDG_RUNTIME_DIR is not set")
	}
	lock, e := os.OpenFile(filepath.Join(runtime, "nwg-dock-intellihide.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("another dock manager is running: %w", e)
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp(runtime, "nwg-native-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	workers := map[string]*nativeWorker{}
	retry := map[string]time.Time{}
	defer func() {
		for _, w := range workers {
			w.stop()
		}
		for _, w := range workers {
			select {
			case <-w.done:
			case <-time.After(2 * time.Second):
				_ = w.cmd.Process.Kill()
				<-w.done
			}
		}
	}()
	ipc := nativeIPC{}
	defer ipc.close()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		for name, w := range workers {
			select {
			case err := <-w.done:
				fmt.Fprintf(os.Stderr, "native dock %s exited: %v\n", name, err)
				os.RemoveAll(w.dir)
				delete(workers, name)
				retry[name] = time.Now().Add(2 * time.Second)
			default:
			}
		}
		var outputs []nativeOutput
		if err := ipc.query(3, &outputs); err == nil {
			active := map[string]bool{}
			for _, o := range outputs {
				if o.Active {
					active[o.Name] = true
				}
			}
			for name, w := range workers {
				if !active[name] {
					w.stop()
				}
			}
			for name := range active {
				if workers[name] != nil || time.Now().Before(retry[name]) {
					continue
				}
				d, err := os.MkdirTemp(dir, "output-")
				if err != nil {
					return err
				}
				args := []string{"-native-child", "-r", "-p", "left", "-o", name, "-i", "36", "-nows", "-nolauncher", "-l", "overlay"}
				if *nativeStatusPath != "" {
					args = append(args, "-status-config", *nativeStatusPath)
				}
				if *debug {
					args = append(args, "-debug")
				}
				cmd := exec.Command(exe, args...)
				cmd.Env = nativeEnv("TMPDIR", d)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err = cmd.Start(); err != nil {
					os.RemoveAll(d)
					retry[name] = time.Now().Add(2 * time.Second)
					fmt.Fprintln(os.Stderr, err)
					continue
				}
				w := &nativeWorker{cmd: cmd, done: make(chan error, 1), dir: d}
				workers[name] = w
				go func() { w.done <- cmd.Wait() }()
			}
		} else {
			fmt.Fprintln(os.Stderr, "native manager IPC:", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var pidFile = dataPath("daemon.pid")

// isDaemonRunning checks the PID file and whether that process is alive.
func isDaemonRunning() (bool, int) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = os.Remove(pidFile)
		return false, 0
	}
	return true, pid
}

// startDaemon re-execs this binary with --daemon and saves the child PID.
func startDaemon() error {
	if running, _ := isDaemonRunning(); running {
		return fmt.Errorf("daemon already running")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
}

// stopDaemon sends SIGTERM to the daemon process.
func stopDaemon() error {
	running, pid := isDaemonRunning()
	if !running {
		return fmt.Errorf("daemon not running")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = os.Remove(pidFile)
	return proc.Signal(syscall.SIGTERM)
}

// runDaemonLoop is the main daemon entry point, called when --daemon flag is set.
func runDaemonLoop(cfg *Config) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	if cfg.DaemonMode == DaemonModeResponsive {
		runResponsive(cfg, sigs)
	} else {
		runNightly(cfg, sigs)
	}
	_ = os.Remove(pidFile)
}

func runResponsive(cfg *Config, sigs chan os.Signal) {
	tick := func() {
		fresh, _ := loadConfig()
		store, _ := loadStore()
		if fresh == nil || store == nil {
			return
		}
		for _, t := range store.Tasks {
			if needsAgentResponse(t, false) {
				processTask(fresh, t, store)
			}
		}
	}

	tick() // run immediately on start
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tick()
		case <-sigs:
			return
		}
	}
}

func runNightly(cfg *Config, sigs chan os.Signal) {
	for {
		d := durationUntil(cfg.NightlyTime)
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
			fresh, _ := loadConfig()
			store, _ := loadStore()
			if fresh != nil && store != nil {
				for _, t := range store.Tasks {
					if needsAgentResponse(t, true) {
						processTask(fresh, t, store)
					}
				}
			}
		case <-sigs:
			timer.Stop()
			return
		}
	}
}

// durationUntil returns the time until the next occurrence of "HH:MM".
func durationUntil(hhmm string) time.Duration {
	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return 24 * time.Hour
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

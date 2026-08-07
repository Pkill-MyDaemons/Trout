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
var daemonLockFile = dataPath("daemon.lock")

// acquireDaemonLock ensures only one daemon process can ever be actively
// processing tasks at a time, using an OS-level exclusive lock rather than
// relying solely on the PID file (which is racy: two "start" presses in
// quick succession, or a stale PID getting reused by an unrelated process,
// can both fool isDaemonRunning). Without this, two daemon processes can
// end up claiming and processing the same task independently — each
// running its own full multi-round tool-use loop against the LLM provider,
// silently doubling API/token usage until the account runs out of budget.
// The lock is held for the lifetime of the process (never explicitly
// unlocked); the OS releases it on exit.
var daemonLockHandle *os.File

func acquireDaemonLock() error {
	f, err := os.OpenFile(daemonLockFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("another daemon instance already holds the lock")
	}
	daemonLockHandle = f
	return nil
}

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
	// cmd.Stdout = nil
	// cmd.Stderr = nil
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
	if err := acquireDaemonLock(); err != nil {
		daemonLog("startup aborted: %v", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	if cfg.DaemonMode == DaemonModeResponsive {
		runResponsive(cfg, sigs)
	} else if cfg.DaemonMode == DaemonModeNightly {
		runNightly(cfg, sigs)
	} else {
		runInstant(cfg, sigs)
	}
	_ = os.Remove(pidFile)
}

func runResponsive(cfg *Config, sigs chan os.Signal) {
	tick := func() {
		daemonLog("tick")
		fresh, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			return
		}
		store, err := loadStore()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading store: %v\n", err)
			return
		}

		if err := pollGmailInbox(fresh, store); err != nil {
			daemonLog("gmail poll error: %v", err)
			fmt.Fprintf(os.Stderr, "gmail poll: %v\n", err)
		}

		for _, t := range store.Tasks {
			if needsAgentResponse(t) && tryClaimTask(t.ID) {
				go func(task *Task) {
					defer releaseTask(task.ID)
					processTask(fresh, task, store)
				}(t)
			}
		}
	}

	tick()
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
func runInstant(cfg *Config, sigs chan os.Signal) {
	tick := func() {
		daemonLog("tick")
		fresh, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			return
		}
		store, err := loadStore()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading store: %v\n", err)
			return
		}

		if err := pollGmailInbox(fresh, store); err != nil {
			daemonLog("gmail poll error: %v", err)
			fmt.Fprintf(os.Stderr, "gmail poll: %v\n", err)
		}

		for _, t := range store.Tasks {
			if needsAgentResponse(t) && tryClaimTask(t.ID) {
				go func(task *Task) {
					defer releaseTask(task.ID)
					processTask(fresh, task, store)
				}(t)
			}
		}
	}

	tick()
	ticker := time.NewTicker(5 * time.Second)
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
				if err := pollGmailInbox(fresh, store); err != nil {
					fmt.Fprintf(os.Stderr, "gmail poll: %v\n", err)
				}
				for _, t := range store.Tasks {
					if needsAgentResponse(t) {
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

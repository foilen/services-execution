package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// gracePeriod is how long a service is given to shut down cleanly after
// receiving SIGTERM before it gets force-killed with SIGKILL.
const gracePeriod = 10 * time.Second

type managedProcess struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}
}

var (
	processesMu sync.Mutex
	processes   []*managedProcess

	shutdownOnce sync.Once

	exitCodeMu sync.Mutex
	exitCode   int
)

func main() {

	// Get the configuration
	args := os.Args[1:]

	if len(args) != 1 {
		panic("You need to specify the config file to use")
	}

	rootConfig, err := getRootConfiguration(args[0])
	if err != nil {
		panic(err)
	}

	// Forward termination signals from the outside (Docker/systemd stop, Ctrl-C, ...)
	// into a graceful shutdown of all the child processes.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigChan
		fmt.Println("Received signal", sig, "; shutting down")
		shutdownAll()
	}()

	// Start all processes
	var wg sync.WaitGroup
	for i, service := range rootConfig.Services {
		fmt.Println(i, service)

		// Run through a shell so quoted paths/arguments (e.g. containing spaces) are
		// parsed correctly instead of being naively split on spaces.
		cmd := exec.Command("/bin/sh", "-c", service.Command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = service.WorkingDirectory
		uid := uint32(os.Getuid())
		if service.UserID != nil {
			uid = *service.UserID
		}
		gid := uint32(os.Getgid())
		if service.GroupID != nil {
			gid = *service.GroupID
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{}
		// NoSetGroups avoids the setgroups() syscall, which requires CAP_SETGID
		// unconditionally on Linux even when Uid/Gid match the calling process.
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid, NoSetGroups: true}
		// Run in its own process group so it doesn't receive signals (e.g. SIGINT
		// from an interactive terminal) meant for the supervisor's group. Shutdown
		// signals are delivered individually to the whole descendant tree (see
		// killTree) since children can set up their own session/process group
		// (e.g. php-fpm calling setsid()) and stop being reachable via pgid.
		cmd.SysProcAttr.Setpgid = true
		err := cmd.Start()
		if err != nil {
			fmt.Println(i, "Problem to start", err)
			setExitCode(1)
			shutdownAll()
			break
		}

		mp := &managedProcess{
			name: fmt.Sprintf("%d (PID %d)", i, cmd.Process.Pid),
			cmd:  cmd,
			done: make(chan struct{}),
		}
		processesMu.Lock()
		processes = append(processes, mp)
		processesMu.Unlock()
		fmt.Println(i, "PID:", cmd.Process.Pid)

		// Wait for it to end
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Println(mp.name, "Start waiting")
			err := mp.cmd.Wait()
			fmt.Println(mp.name, "Ended", err)
			close(mp.done)

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					setExitCode(exitErr.ExitCode())
				} else {
					setExitCode(1)
				}
			}

			shutdownAll()
		}()
	}

	fmt.Println("Wait for all processes to end")
	wg.Wait()
	fmt.Println("All processes ended")

	os.Exit(getExitCode())
}

// shutdownAll asks every still-running process to stop gracefully (SIGTERM), then
// force-kills (SIGKILL) any of them still alive after gracePeriod. It only takes
// effect once, since the first exiting/crashing service should trigger the shutdown
// of the others exactly one time.
func shutdownAll() {
	shutdownOnce.Do(func() {
		fmt.Println("Stopping all the processes gracefully")

		processesMu.Lock()
		snapshot := make([]*managedProcess, len(processes))
		copy(snapshot, processes)
		processesMu.Unlock()

		for _, mp := range snapshot {
			select {
			case <-mp.done:
				continue
			default:
			}
			fmt.Println(mp.name, "Sending SIGTERM")
			killTree(mp.cmd.Process.Pid, syscall.SIGTERM)
		}

		var wg sync.WaitGroup
		for _, mp := range snapshot {
			wg.Add(1)
			go func(mp *managedProcess) {
				defer wg.Done()
				select {
				case <-mp.done:
				case <-time.After(gracePeriod):
					fmt.Println(mp.name, "Did not stop in time; sending SIGKILL")
					killTree(mp.cmd.Process.Pid, syscall.SIGKILL)
				}
			}(mp)
		}
		wg.Wait()
	})
}

// killTree signals rootPid and every one of its descendants (recursively), as found
// in /proc at the time of the call. Descendants are found via each process's PPid in
// /proc/<pid>/stat, which stays accurate even for processes that called setsid() to
// detach into their own session/process group (e.g. php-fpm), so signaling by pgid
// alone would miss them.
func killTree(rootPid int, sig syscall.Signal) {
	for _, pid := range processTree(rootPid) {
		_ = syscall.Kill(pid, sig)
	}
}

// processTree returns rootPid and all of its descendants, found by scanning /proc.
func processTree(rootPid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{rootPid}
	}

	childrenByParent := make(map[int][]int)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		ppid, err := readPPID(pid)
		if err != nil {
			continue
		}
		childrenByParent[ppid] = append(childrenByParent[ppid], pid)
	}

	var result []int
	queue := []int{rootPid}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		result = append(result, pid)
		queue = append(queue, childrenByParent[pid]...)
	}
	return result
}

// readPPID reads the parent PID of pid from /proc/<pid>/stat. The comm field (2nd
// field) is wrapped in parens and may itself contain spaces/parens, so the fields
// are located from the last ")" rather than by naive whitespace splitting.
func readPPID(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	afterComm := strings.LastIndex(string(data), ")")
	if afterComm < 0 {
		return 0, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}
	fields := strings.Fields(string(data)[afterComm+1:])
	// fields[0] is state, fields[1] is ppid.
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}
	return strconv.Atoi(fields[1])
}

func setExitCode(code int) {
	if code == 0 {
		return
	}
	exitCodeMu.Lock()
	defer exitCodeMu.Unlock()
	if exitCode == 0 {
		exitCode = code
	}
}

func getExitCode() int {
	exitCodeMu.Lock()
	defer exitCodeMu.Unlock()
	return exitCode
}

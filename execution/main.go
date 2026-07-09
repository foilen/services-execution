package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: service.UserID, Gid: service.GroupID}
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
			_ = mp.cmd.Process.Signal(syscall.SIGTERM)
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
					_ = mp.cmd.Process.Kill()
				}
			}(mp)
		}
		wg.Wait()
	})
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

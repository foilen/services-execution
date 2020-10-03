package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
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

	// Start all processes
	var wg sync.WaitGroup
	var processes []*os.Process
	for i, service := range rootConfig.Services {
		fmt.Println(i, service)
		commandAndArguments := strings.Split(service.Command, " ")
		cmd := exec.Command(commandAndArguments[0], commandAndArguments[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = service.WorkingDirectory
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: service.UserID, Gid: service.GroupID}
		err := cmd.Start()
		if err != nil {
			fmt.Println(i, "Problem to start", err)
			killAll(processes)
			break
		}
		processes = append(processes, cmd.Process)
		fmt.Println(i, "PID:", cmd.Process.Pid)

		// Wait for it to end
		wg.Add(1)
		go func() {
			fmt.Println(cmd.Process.Pid, "Start waiting")
			_, err = cmd.Process.Wait()
			fmt.Println(cmd.Process.Pid, "Ended", err)
			killAll(processes)
			wg.Done()
		}()
	}

	fmt.Println("Wait for all processes to end")
	wg.Wait()
	fmt.Println("All processes ended")
}

var allKilled = false

func killAll(processes []*os.Process) {

	if allKilled {
		return
	}

	fmt.Println("Forcing stop of all the processes")

	for _, process := range processes {
		_ = process.Kill()
	}

}

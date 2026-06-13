// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sphragis-oss/sphragis/internal/control"
)

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "start",
		Short:   "Start the gateway in the background",
		GroupID: groupGateway,
		Args:    cobra.NoArgs,
		RunE:    func(_ *cobra.Command, _ []string) error { return start() },
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "stop",
		Short:   "Stop the background gateway",
		GroupID: groupGateway,
		Args:    cobra.NoArgs,
		RunE:    func(_ *cobra.Command, _ []string) error { return stop() },
	}
}

func restartCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "restart",
		Short:   "Restart the background gateway",
		GroupID: groupGateway,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = stop()
			return start()
		},
	}
}

func start() error {
	if pid, ok := control.Running(); ok {
		return fmt.Errorf("already running (pid %d)", pid)
	}
	if err := control.EnsureHome(); err != nil {
		return err
	}
	logf, err := os.OpenFile(control.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], "serve")
	cmd.Env = os.Environ()
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := control.WritePID(cmd.Process.Pid); err != nil {
		return err
	}
	fmt.Printf("sphragis started (pid %d)\nlogs: %s\n", cmd.Process.Pid, control.LogPath())
	return nil
}

func stop() error {
	pid, ok := control.Running()
	if !ok {
		control.RemovePID()
		return fmt.Errorf("not running")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		if _, ok := control.Running(); !ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	control.RemovePID()
	fmt.Printf("sphragis stopped (pid %d)\n", pid)
	return nil
}

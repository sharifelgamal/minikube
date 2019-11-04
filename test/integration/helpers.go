/*
Copyright 2019 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

// These are test helpers that:
//
// - Accept *testing.T arguments (see helpers.go)
// - Are used in multiple tests
// - Must not compare test values

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/process"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/minikube/pkg/kapi"
)

var (
	// startTimes is a list of startup times, to guarantee --start-offset
	startTimes = []time.Time{}
	// startTimesMutex is a lock to update startTimes without a race condition
	startTimesMutex = &sync.Mutex{}
)

// RunResult stores the result of an cmd.Run call
type RunResult struct {
	Stdout   *bytes.Buffer
	Stderr   *bytes.Buffer
	ExitCode int
	Args     []string
}

// Command returns a human readable command string that does not induce eye fatigue
func (rr RunResult) Command() string {
	var sb strings.Builder
	sb.WriteString(strings.TrimPrefix(rr.Args[0], "../../"))
	for _, a := range rr.Args[1:] {
		if strings.Contains(a, " ") {
			sb.WriteString(fmt.Sprintf(` "%s"`, a))
			continue
		}
		sb.WriteString(fmt.Sprintf(" %s", a))
	}
	return sb.String()
}

// Output returns human-readable output for an execution result
func (rr RunResult) Output() string {
	var sb strings.Builder
	if rr.Stdout.Len() > 0 {
		sb.WriteString(fmt.Sprintf("-- stdout --\n%s\n-- /stdout --", rr.Stdout.Bytes()))
	}
	if rr.Stderr.Len() > 0 {
		sb.WriteString(fmt.Sprintf("\n** stderr ** \n%s\n** /stderr **", rr.Stderr.Bytes()))
	}
	return sb.String()
}

// Run is a test helper to log a command being executed \_(ツ)_/¯
func Run(t *testing.T, cmd *exec.Cmd) (*RunResult, error) {
	t.Helper()
	rr := &RunResult{Args: cmd.Args}
	t.Logf("(dbg) Run:  %v", rr.Command())

	var outb, errb bytes.Buffer
	cmd.Stdout, rr.Stdout = &outb, &outb
	cmd.Stderr, rr.Stderr = &errb, &errb
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err == nil {
		// Reduce log spam
		if elapsed > (1 * time.Second) {
			t.Logf("(dbg) Done: %v: (%s)", rr.Command(), elapsed)
		}
	} else {
		if exitError, ok := err.(*exec.ExitError); ok {
			rr.ExitCode = exitError.ExitCode()
		}
		t.Logf("(dbg) Non-zero exit: %v: %v (%s)\n%s", rr.Command(), err, elapsed, rr.Output())
	}
	return rr, err
}

// StartSession stores the result of an cmd.Start call
type StartSession struct {
	Stdout *bufio.Reader
	Stderr *bufio.Reader
	cmd    *exec.Cmd
}

// Start starts a process in the background, streaming output
func Start(t *testing.T, cmd *exec.Cmd) (*StartSession, error) {
	t.Helper()
	t.Logf("(dbg) daemon: %v", cmd.Args)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe failed: %v %v", cmd.Args, err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe failed: %v %v", cmd.Args, err)
	}

	sr := &StartSession{Stdout: bufio.NewReader(stdoutPipe), Stderr: bufio.NewReader(stderrPipe), cmd: cmd}
	return sr, cmd.Start()
}

// Stop stops the started process
func (ss *StartSession) Stop(t *testing.T) {
	t.Helper()
	t.Logf("(dbg) stopping %s ...", ss.cmd.Args)
	if ss.cmd.Process == nil {
		t.Logf("%s has a nil Process. Maybe it's dead? How weird!", ss.cmd.Args)
		return
	}
	killProcessFamily(t, ss.cmd.Process.Pid)
	if t.Failed() {
		if ss.Stdout.Size() > 0 {
			stdout, err := ioutil.ReadAll(ss.Stdout)
			if err != nil {
				t.Logf("read stdout failed: %v", err)
			}
			t.Logf("(dbg) %s stdout:\n%s", ss.cmd.Args, stdout)
		}
		if ss.Stderr.Size() > 0 {
			stderr, err := ioutil.ReadAll(ss.Stderr)
			if err != nil {
				t.Logf("read stderr failed: %v", err)
			}
			t.Logf("(dbg) %s stderr:\n%s", ss.cmd.Args, stderr)
		}
	}
}

// Cleanup cleans up after a test run
func Cleanup(t *testing.T, profile string, cancel context.CancelFunc) {
	// No helper because it makes the call log confusing.
	if *cleanup {
		_, err := Run(t, exec.Command(Target(), "delete", "-p", profile))
		if err != nil {
			t.Logf("failed cleanup: %v", err)
		}
	} else {
		t.Logf("skipping cleanup of %s (--cleanup=false)", profile)
	}
	cancel()
}

// CleanupWithLogs cleans up after a test run, fetching logs and deleting the profile
func CleanupWithLogs(t *testing.T, profile string, cancel context.CancelFunc) {
	t.Helper()
	if t.Failed() && *postMortemLogs {
		t.Logf("%s failed, collecting logs ...", t.Name())
		rr, err := Run(t, exec.Command(Target(), "-p", profile, "logs", "--problems"))
		if err != nil {
			t.Logf("failed logs error: %v", err)
		}
		t.Logf("%s logs: %s", t.Name(), rr.Stdout)
		t.Logf("Sorry that %s failed :(", t.Name())
	}
	Cleanup(t, profile, cancel)
}

// podStatusMsg returns a human-readable pod status, for generating debug status
func podStatusMsg(pod core.Pod) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%q [%s] %s", pod.ObjectMeta.GetName(), pod.ObjectMeta.GetUID(), pod.Status.Phase))
	for i, c := range pod.Status.Conditions {
		if c.Reason != "" {
			if i == 0 {
				sb.WriteString(": ")
			} else {
				sb.WriteString(" / ")
			}
			sb.WriteString(fmt.Sprintf("%s:%s", c.Type, c.Reason))
		}
		if c.Message != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", c.Message))
		}
	}
	return sb.String()
}

// PodWait waits for pods to achieve a running state.
func PodWait(ctx context.Context, t *testing.T, profile string, ns string, selector string, timeout time.Duration) ([]string, error) {
	t.Helper()
	client, err := kapi.Client(profile)
	if err != nil {
		return nil, err
	}

	// For example: kubernetes.io/minikube-addons=gvisor
	listOpts := meta.ListOptions{LabelSelector: selector}
	minUptime := 5 * time.Second
	podStart := time.Time{}
	foundNames := map[string]bool{}
	lastMsg := ""

	start := time.Now()
	t.Logf("(dbg) waiting for pods with labels %q in namespace %q ...", selector, ns)
	f := func() (bool, error) {
		pods, err := client.CoreV1().Pods(ns).List(listOpts)
		if err != nil {
			t.Logf("WARNING: pod list for %q %q returned: %v", ns, selector, err)
			// Don't return the error upwards so that this is retried, in case the apiserver is rescheduled
			podStart = time.Time{}
			return false, nil
		}
		if len(pods.Items) == 0 {
			podStart = time.Time{}
			return false, nil
		}

		for _, pod := range pods.Items {
			foundNames[pod.ObjectMeta.Name] = true
			msg := podStatusMsg(pod)
			// Prevent spamming logs with identical messages
			if msg != lastMsg {
				t.Log(msg)
				lastMsg = msg
			}
			// Successful termination of a short-lived process, will not be restarted
			if pod.Status.Phase == core.PodSucceeded {
				return true, nil
			}
			// Long-running process state
			if pod.Status.Phase != core.PodRunning {
				if !podStart.IsZero() {
					t.Logf("WARNING: %s was running %s ago - may be unstable", selector, time.Since(podStart))
				}
				podStart = time.Time{}
				return false, nil
			}

			if podStart.IsZero() {
				podStart = time.Now()
			}

			if time.Since(podStart) > minUptime {
				return true, nil
			}
		}
		return false, nil
	}

	err = wait.PollImmediate(500*time.Millisecond, timeout, f)
	names := []string{}
	for n := range foundNames {
		names = append(names, n)
	}

	if err == nil {
		t.Logf("(dbg) pods %s up and healthy within %s", selector, time.Since(start))
		return names, nil
	}

	t.Logf("pod %q failed to start: %v", selector, err)
	showPodLogs(ctx, t, profile, ns, names)
	return names, fmt.Errorf("%s: %v", fmt.Sprintf("%s within %s", selector, timeout), err)
}

// showPodLogs logs debug info for pods
func showPodLogs(ctx context.Context, t *testing.T, profile string, ns string, names []string) {
	rr, rerr := Run(t, exec.CommandContext(ctx, "kubectl", "--context", profile, "get", "po", "-A", "--show-labels"))
	if rerr != nil {
		t.Logf("%s: %v", rr.Command(), rerr)
		// return now, because kubectl is hosed
		return
	}
	t.Logf("(dbg) %s:\n%s", rr.Command(), rr.Stdout)

	for _, name := range names {
		rr, err := Run(t, exec.CommandContext(ctx, "kubectl", "--context", profile, "describe", "po", name, "-n", ns))
		if err != nil {
			t.Logf("%s: %v", rr.Command(), err)
		} else {
			t.Logf("(dbg) %s:\n%s", rr.Command(), rr.Stdout)
		}

		rr, err = Run(t, exec.CommandContext(ctx, "kubectl", "--context", profile, "logs", name, "-n", ns))
		if err != nil {
			t.Logf("%s: %v", rr.Command(), err)
		} else {
			t.Logf("(dbg) %s:\n%s", rr.Command(), rr.Stdout)
		}
	}
}

// Status returns the minikube cluster status as a string
func Status(ctx context.Context, t *testing.T, path string, profile string) string {
	t.Helper()
	rr, err := Run(t, exec.CommandContext(ctx, path, "status", "--format={{.Host}}", "-p", profile))
	if err != nil {
		t.Logf("status error: %v (may be ok)", err)
	}
	return strings.TrimSpace(rr.Stdout.String())
}

// MaybeParallel sets that the test should run in parallel
func MaybeParallel(t *testing.T) {
	t.Helper()
	// TODO: Allow paralellized tests on "none" that do not require independent clusters
	if NoneDriver() {
		return
	}
	t.Parallel()
}

// WaitForStartSlot enforces --start-offset to avoid startup race conditions
func WaitForStartSlot(t *testing.T) {
	// Not parallel
	if NoneDriver() {
		return
	}

	wakeup := time.Now()
	startTimesMutex.Lock()
	if len(startTimes) > 0 {
		nextStart := startTimes[len(startTimes)-1].Add(*startOffset)
		// Ignore nextStart if it is in the past - to guarantee offset for next caller
		if time.Now().Before(nextStart) {
			wakeup = nextStart
		}
	}
	startTimes = append(startTimes, wakeup)
	startTimesMutex.Unlock()

	if time.Now().Before(wakeup) {
		d := time.Until(wakeup)
		t.Logf("Waiting for start slot at %s (sleeping %s)  ...", wakeup, d)
		time.Sleep(d)
	} else {
		t.Logf("No need to wait for start slot, it is already %s", time.Now())
	}
}

// killProcessFamily kills a pid and all of its children
func killProcessFamily(t *testing.T, pid int) {
	parent, err := process.NewProcess(int32(pid))
	if err != nil {
		t.Logf("unable to find parent, assuming dead: %v", err)
		return
	}
	procs := []*process.Process{}
	children, err := parent.Children()
	if err == nil {
		procs = append(procs, children...)
	}
	procs = append(procs, parent)

	for _, p := range procs {
		if err := p.Terminate(); err != nil {
			t.Logf("unable to terminate pid %d: %v", p.Pid, err)
			continue
		}
		// Allow process a chance to cleanup before instant death.
		time.Sleep(100 * time.Millisecond)
		if err := p.Kill(); err != nil {
			t.Logf("unable to kill pid %d: %v", p.Pid, err)
			continue
		}
	}
}

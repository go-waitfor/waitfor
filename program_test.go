package waitfor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func helperProgram(t *testing.T, mode, marker string) Program {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	return Program{
		Executable: executable,
		Args:       []string{"-test.run=^TestProgramHelperProcess$", "--", mode, marker},
	}
}

func TestProgramHelperProcess(t *testing.T) {
	args := os.Args
	if len(args) < 4 || args[len(args)-3] != "--" {
		return
	}
	switch args[len(args)-2] {
	case "echo":
		fmt.Fprint(os.Stdout, "hello")
		fmt.Fprint(os.Stderr, "stderr")
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, "command failure")
		os.Exit(7)
	case "stream":
		fmt.Fprint(os.Stdout, "hello")
		for {
			if _, err := os.Stat(args[len(args)-1]); err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		fmt.Fprint(os.Stderr, "stderr")
		os.Exit(0)
	case "spawn", "spawn-wait":
		child := helperProgram(t, "hold-pipe", "")
		cmd := exec.Command(child.Executable, child.Args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		require.NoError(t, cmd.Start())
		require.NoError(t, cmd.Process.Release())
		fmt.Fprint(os.Stdout, "spawned")
		if args[len(args)-2] == "spawn-wait" {
			time.Sleep(time.Minute)
		}
		os.Exit(0)
	case "hold-pipe":
		// Exit as soon as the reader closes the inherited pipe. The fallback
		// keeps a regression from hanging the test indefinitely.
		end := time.Now().Add(10 * time.Second)
		for time.Now().Before(end) {
			time.Sleep(50 * time.Millisecond)
			if _, err := fmt.Fprint(os.Stdout, "."); err != nil {
				break
			}
		}
		os.Exit(0)
	case "wait":
		if err := os.WriteFile(args[len(args)-1], []byte("started"), 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	default:
		t.Fatal("unknown helper mode")
	}
}

type outputFunc func([]byte) (int, error)

func (f outputFunc) Write(p []byte) (int, error) { return f(p) }

func TestRunToStreamsBeforeCommandExit(t *testing.T) {
	runner := testRunner(t, readyFactory)
	marker := filepath.Join(t.TempDir(), "received")
	var output bytes.Buffer
	writer := outputFunc(func(p []byte) (int, error) {
		if output.Len() == 0 {
			if err := os.WriteFile(marker, nil, 0600); err != nil {
				return 0, err
			}
		}
		return output.Write(p)
	})
	program := helperProgram(t, "stream", marker)
	program.Resources = []string{"test://ready"}
	require.NoError(t, runner.RunTo(testContext(t), program, writer))
	require.Equal(t, "hellostderr", output.String())

	program = helperProgram(t, "echo", "")
	require.NoError(t, runner.RunTo(testContext(t), program, nil))
	program.Resources = []string{"unregistered://host"}
	output.Reset()
	require.ErrorIs(t, runner.RunTo(testContext(t), program, writer), ErrResourceNotFound)
	require.Empty(t, output.String())
}

func TestProgramWaitDelay(t *testing.T) {
	runner := testRunner(t, readyFactory)
	program := helperProgram(t, "spawn", "")
	program.WaitDelay = -1
	out, err := runner.Run(testContext(t), program)
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.Nil(t, out)
	require.NoError(t, runner.Resources().Register("another", readyFactory))

	// The direct child exits successfully while its descendant holds stdout.
	program.WaitDelay = 25 * time.Millisecond
	out, err = runner.Run(testContext(t), program)
	require.ErrorIs(t, err, exec.ErrWaitDelay)
	require.Contains(t, string(out), "spawned")

	// Cancellation kills the direct child. Return while its descendant still
	// owns the write end, preserving the original command error.
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	program = helperProgram(t, "spawn-wait", "")
	program.WaitDelay = 25 * time.Millisecond
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runner.RunTo(ctx, program, outputFunc(func(p []byte) (int, error) {
			n, err := output.Write(p)
			if bytes.Contains(output.Bytes(), []byte("spawned")) {
				cancel()
			}
			return n, err
		}))
	}()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		require.Contains(t, output.String(), "spawned")
	case <-time.After(5 * time.Second):
		t.Fatal("RunTo did not release inherited pipes after cancellation")
	}
}

func TestRunOutputAndCommandErrors(t *testing.T) {
	runner := testRunner(t, readyFactory)
	program := helperProgram(t, "echo", "")
	program.Resources = []string{"test://a", "test://b"}
	output, err := runner.Run(testContext(t), program, WithMaxElapsedTime(time.Second))
	require.NoError(t, err)
	assert.Equal(t, "hellostderr", string(output))
	// An empty wait completes even with a minimal wait-only limit.
	program.Resources = nil
	output, err = runner.Run(testContext(t), program, WithMaxElapsedTime(time.Nanosecond))
	require.NoError(t, err)
	assert.Equal(t, "hellostderr", string(output))
	output, err = runner.Run(testContext(t), helperProgram(t, "fail", ""))
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	assert.Equal(t, 7, exitError.ExitCode())
	assert.Equal(t, "command failure", string(output))
	_, err = runner.Run(testContext(t), Program{Executable: filepath.Join(t.TempDir(), "missing")})
	require.Error(t, err)
}

func TestRunSkipsCommandOnReadinessFailure(t *testing.T) {
	runner, err := New()
	require.NoError(t, err)
	program := helperProgram(t, "echo", "")
	program.Resources = []string{"unknown://a"}
	output, err := runner.Run(testContext(t), program, WithAttempts(1))
	require.ErrorIs(t, err, ErrWait)
	require.ErrorIs(t, err, ErrResourceNotFound)
	assert.Nil(t, output)
}

func TestRunCancellationTerminatesStartedProcess(t *testing.T) {
	runner, err := New()
	require.NoError(t, err)
	marker := filepath.Join(t.TempDir(), "started")
	program := helperProgram(t, "wait", marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runner.Run(ctx, program); done <- err }()

	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("process exited before cancellation: %v", err)
		case <-watchdog.C:
			t.Fatal("helper did not signal startup")
		case <-poll.C:
			if _, err := os.Stat(marker); err == nil {
				cancel()
				select {
				case err := <-done:
					var exitError *exec.ExitError
					require.ErrorAs(t, err, &exitError)
					assert.False(t, exitError.Success())
				case <-time.After(5 * time.Second):
					t.Fatal("cancellation did not terminate the started process")
				}
				return
			}
		}
	}
}

package waitfor

import (
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

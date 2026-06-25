package task

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/shlex"
)

func run(args ...string) error {
	return runContext(context.Background(), args...)
}

func runContext(ctx context.Context, args ...string) error {
	_, err := RunArgs(ctx, args)
	return err
}

// RunLine splits line using shell-word rules and runs the resulting task
// arguments. A leading "task" token is ignored so callers may accept either
// "add foo" or "task add foo" from user input.
func RunLine(ctx context.Context, line string) (RunResult, error) {
	fields, err := shlex.Split(line)
	if err != nil {
		return RunResult{}, err
	}
	if len(fields) > 0 && fields[0] == "task" {
		fields = fields[1:]
	}
	return RunArgs(ctx, fields)
}

// RunShellLine runs a user-entered task command in non-interactive mode. It
// avoids Taskwarrior's recurring-task prompt by applying the same behavior as
// answering "no": modify only the addressed recurrence.
func RunShellLine(ctx context.Context, line string) (RunResult, error) {
	fields, err := shlex.Split(line)
	if err != nil {
		return RunResult{}, err
	}
	if len(fields) > 0 && fields[0] == "task" {
		fields = fields[1:]
	}
	fields = append([]string{"rc.recurrence.confirmation=no"}, fields...)
	return RunArgs(ctx, fields)
}

// RunArgs runs "task" with args and captures stdout and stderr.
func RunArgs(ctx context.Context, args []string) (RunResult, error) {
	copied := append([]string(nil), args...)
	result := RunResult{Args: copied}
	if len(copied) == 0 {
		return result, fmt.Errorf("empty task command")
	}

	if dbg.writer != nil {
		if _, err := fmt.Fprintln(dbg.writer, "task "+strings.Join(copied, " ")); err != nil {
			return result, fmt.Errorf("write debug log: %w", err)
		}
	}

	cmd := exec.CommandContext(ctx, "task", copied...)
	configureCommandContext(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, fmt.Errorf("task command: %w", ctxErr)
		}
		if strings.TrimSpace(result.Stderr) != "" {
			return result, fmt.Errorf("%w: %s", err, strings.TrimSpace(result.Stderr))
		}
		return result, err
	}
	return result, nil
}

package task

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// LoadCompletionSources returns Taskwarrior-provided completion candidates.
func LoadCompletionSources(ctx context.Context) CompletionSources {
	return CompletionSources{
		Commands: completionList(ctx, "_commands"),
		Columns:  completionList(ctx, "_columns"),
		Projects: completionList(ctx, "_projects"),
		Tags:     completionList(ctx, "_tags"),
		IDs:      completionList(ctx, "_ids"),
		UUIDs:    completionList(ctx, "_uuids"),
		UDAs:     completionList(ctx, "_udas"),
	}
}

func completionList(ctx context.Context, command string) []string {
	result, err := RunArgs(ctx, []string{command})
	if err != nil {
		return nil
	}
	return outputLines(result.Stdout)
}

func outputLines(output string) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	seen := make(map[string]struct{})
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	return lines
}

// Export retrieves tasks using `task <filter> export rc.json.array=off` and parses
// the JSON output into a slice of Task structs. Optional filter arguments are
// passed directly to the `task` command before `export`.
func Export(ctx context.Context, filters ...string) ([]Task, error) {
	args := append(filters, "export", "rc.json.array=off")
	cmd := exec.CommandContext(ctx, "task", args...)
	configureCommandContext(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("task export: %w", ctxErr)
		}
		// Include stderr output in the error message
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}

	var tasks []Task
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var t Task
		if err := json.Unmarshal(line, &t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

// RecurringSeries returns the recurring template and generated instances for
// the recurring task identified by rootUUID.
func RecurringSeries(ctx context.Context, rootUUID string) ([]Task, error) {
	if strings.TrimSpace(rootUUID) == "" {
		return nil, fmt.Errorf("empty recurring task UUID")
	}
	return Export(ctx, fmt.Sprintf("(%s or parent:%s)", rootUUID, rootUUID), "status.any:")
}

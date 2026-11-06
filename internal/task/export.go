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
	// Completion lists should be one short entry per line, but a single
	// pathological line longer than the default 64KiB scanner limit must not
	// silently truncate the result.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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

	return parseExportOutput(out)
}

// parseExportOutput decodes the newline-delimited JSON objects emitted by
// `task export rc.json.array=off`. A streaming json.Decoder is used instead
// of a line scanner so tasks whose exported JSON exceeds the 64KiB
// bufio.Scanner token limit (long descriptions, large annotation sets)
// still parse.
func parseExportOutput(out []byte) ([]Task, error) {
	var tasks []Task
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var t Task
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("parsing exported task: %w", err)
		}
		tasks = append(tasks, t)
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

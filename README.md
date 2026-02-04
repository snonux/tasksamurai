# Task Samurai

<img src="logo.png" alt="tasksamurai logo" width="250" />

Task Samurai is a fast terminal interface for [Taskwarrior](https://taskwarrior.org/) written in Go using the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework. It shows your tasks in a table and lets you manage them without leaving your keyboard.

## Why does this exist?

- I wanted to tinker with agentic coding (actually, it has been mainly vibe coded using OpenAI Codex and Claude Code CLI)
- I wanted a faster UI for Taskwarrior than other options like vit which is Python based.
- I wanted something built with Bubble Tea but never had time to deep dive into it.

## How it works

Task Samurai invokes the `task` command to read and modify tasks. The tasks are displayed in a Bubble Tea table where each row represents a task. Hotkeys trigger Taskwarrior commands such as starting, completing or annotating tasks. The UI refreshes automatically after each action so the table is always up to date.

## Hotkeys

Press `H` to view all available hotkeys.

Example: press `+`, type `Buy milk` and hit Enter to add a new task called "Buy milk".

## Screenshot

![Task Samurai screenshot](screenshot.png)

## Installation

There are two ways to install the `tasksamurai` command:

```bash
go install codeberg.org/snonux/tasksamurai/cmd/tasksamurai@latest
```

Alternatively, clone this repository and run:

```bash
mage install
```

The second method requires [mage](https://magefile.org/) to be installed.

### Usage

```bash
# Start with default pending tasks
tasksamurai

# Start with a Taskwarrior filter
tasksamurai +tag status:pending
tasksamurai project:work due:today
tasksamurai pri:H
tasksamurai -- -excludetag
tasksamurai -- -excludetag +includetag

# Any valid Taskwarrior filter can be passed as arguments
```

### Flags

- `--browser-cmd <command>`: command used to open URLs (default: firefox on Linux, open on macOS)
- `--debug-log <path>`: path to debug log file for Taskwarrior commands
- `--debug-dir <directory>`: directory for runtime debug output (goroutine dumps, profiles)
- `--disco`: start Task Samurai in disco mode, changing the theme every time a task is modified

## Debugging

If Task Samurai appears to hang or freeze, you can capture runtime diagnostics using signal handlers to help diagnose the issue.

### Signal Handlers (Unix/Linux/macOS only)

Task Samurai supports two debugging signals:

#### SIGUSR1 - Quick Goroutine Dump

Captures all goroutine stacks to a timestamped text file for quick inspection:

```bash
# Find the Task Samurai process ID
ps aux | grep tasksamurai

# Send signal to dump goroutines
kill -SIGUSR1 <pid>
```

This creates a file like `tasksamurai-goroutines-20260204-143022.txt` showing what each goroutine is doing.

#### SIGUSR2 - Full Profile Dump

Captures comprehensive profiling data for deeper analysis:

```bash
# Send signal to dump full profiles
kill -SIGUSR2 <pid>
```

This creates multiple files:
- `tasksamurai-TIMESTAMP-goroutines.txt` - Goroutine stacks (text)
- `tasksamurai-TIMESTAMP-heap.pprof` - Memory allocations
- `tasksamurai-TIMESTAMP-cpu.pprof` - CPU profile (5 second sample)
- `tasksamurai-TIMESTAMP-block.pprof` - Lock contention events

### Analyzing Profiles

Use Go's pprof tool to analyze the binary profile files:

```bash
# Interactive analysis
go tool pprof tasksamurai-TIMESTAMP-heap.pprof

# Generate visualization (requires graphviz)
go tool pprof -web tasksamurai-TIMESTAMP-cpu.pprof

# Top functions by CPU usage
go tool pprof -top tasksamurai-TIMESTAMP-cpu.pprof
```

### Specifying Output Location

By default, debug files are written to the current working directory. Use the `--debug-dir` flag to specify a different location:

```bash
tasksamurai --debug-dir=/tmp/tasksamurai-debug
```

### Example Debugging Workflow

When Task Samurai hangs:

1. **Keep the hung process running** - Don't kill it yet!
2. Find the process ID: `pgrep tasksamurai` or `ps aux | grep tasksamurai`
3. Dump goroutines: `kill -SIGUSR1 <pid>`
4. Open the generated file to see what goroutines are blocked
5. If needed, dump full profiles: `kill -SIGUSR2 <pid>`
6. Analyze with pprof to identify the bottleneck

Common issues revealed by goroutine dumps:
- External `task` command hanging (stuck in syscall)
- Waiting for terminal input (blocked on I/O)
- External editor not responding

**Note:** Signal handlers are not available on Windows. Consider using `GODEBUG` environment variables or running under a debugger instead.

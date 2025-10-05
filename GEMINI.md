# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TaskSamurai is a fast Terminal User Interface (TUI) for Taskwarrior written in Go. It provides a keyboard-driven table interface for task management with extensive hotkey support and real-time integration with the Taskwarrior CLI.

## Development Commands

```bash
# Build the application
go-task

# Run the application
go-task run

# Run all tests
go-task test

# Install to $GOPATH/bin
go-task install

# Run with debug logging
./tasksamurai --debug-log debug.log

# Run with custom browser command
./tasksamurai --browser-cmd chromium

# Run with disco mode (random theme changes)
./tasksamurai --disco
```

## Architecture

### Core Components

1. **cmd/tasksamurai/main.go**: Entry point that initializes flags, creates the UI model, and starts the Bubble Tea program.

2. **internal/task/**: Taskwarrior integration layer
   - Executes `task` commands for all CRUD operations
   - Parses JSON export format from Taskwarrior
   - Handles task filtering, sorting, and statistics calculation
   - All operations require the `task` command to be available in PATH

3. **internal/ui/**: Terminal UI implementation using Bubble Tea framework
   - Main model in `model.go` handles application state and message processing
   - `table.go` manages the task table display
   - `input.go` processes keyboard input and executes commands
   - `theme.go` handles color themes and disco mode
   - Search functionality with regex support in search methods

4. **internal/atable/**: Custom table widget implementation
   - Provides flexible table rendering for the task display
   - Handles column widths, scrolling, and cell formatting

### Key Design Patterns

- **Message-Driven Architecture**: Uses Bubble Tea's message passing for all UI updates
- **Command Pattern**: All Taskwarrior operations go through the `internal/task` package
- **MVC-like Structure**: Clear separation between data (task), view (table), and control (input handling)

### Integration Points

- **Taskwarrior CLI**: All task operations execute `task` commands via `exec.Command`
- **Terminal**: Uses ANSI escape sequences and terminal control through Bubble Tea
- **External Browser**: Opens URLs via configurable browser command

## Testing Approach

Tests are located alongside implementation files (*_test.go pattern). Key test areas:

- Task operations and JSON parsing in `internal/task/`
- UI component behavior in `internal/ui/`
- Tests create temporary directories for isolated Taskwarrior environments
- Tests check for `task` command availability and skip if not present

Run a single test file:
```bash
go test ./internal/task/task_test.go
```

Run tests for a specific package:
```bash
go test ./internal/ui/
```

## Important Implementation Notes

1. **Taskwarrior Dependency**: The application requires Taskwarrior to be installed and available in PATH. All operations fail gracefully if `task` is not available.

2. **State Management**: The UI state is managed through the main model in `internal/ui/model.go`. State updates happen through Bubble Tea messages.

3. **Error Handling**: Errors from Taskwarrior commands are displayed in the status bar at the bottom of the UI.

4. **Performance**: The table widget uses efficient rendering to handle large task lists. Only visible rows are rendered.

5. **Configuration**: User preferences like browser command and debug logging are passed via command-line flags, not configuration files.

6. **Hotkeys**: Extensive hotkey system defined in `internal/ui/input.go`. See README.md for complete reference.
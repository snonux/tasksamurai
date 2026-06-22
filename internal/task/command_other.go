//go:build !unix

package task

import "os/exec"

func configureCommandContext(*exec.Cmd) {}

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type command struct {
	Name   string
	Args   []string
	Dir    string
	Stdout io.Writer
}

type commandRunner interface {
	Run(context.Context, command) (string, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, spec command) (string, error) {
	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	var stdout bytes.Buffer
	if spec.Stdout == nil {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdout = spec.Stdout
	}
	var stderr cappedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return "", fmt.Errorf("%s failed: %w", spec.Name, err)
		}
		return "", fmt.Errorf("%s failed: %w: %s", spec.Name, err, message)
	}
	return stdout.String(), nil
}

type cappedBuffer struct {
	data []byte
}

func (buffer *cappedBuffer) Write(data []byte) (int, error) {
	const limit = 4096
	remaining := limit - len(buffer.data)
	if remaining > 0 {
		if len(data) < remaining {
			remaining = len(data)
		}
		buffer.data = append(buffer.data, data[:remaining]...)
	}
	return len(data), nil
}

func (buffer *cappedBuffer) String() string {
	return string(buffer.data)
}

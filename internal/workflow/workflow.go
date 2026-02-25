package workflow

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Definition struct {
	Command         string            `yaml:"command"`
	Args            []string          `yaml:"args"`
	AllowExtraArgs  bool              `yaml:"allow_extra_args"`
	TimeoutSeconds  int               `yaml:"timeout_seconds"`
	WorkingDir      string            `yaml:"working_dir"`
	EnvironmentVars map[string]string `yaml:"env"`
}

type fileDefinition struct {
	Workflows map[string]Definition `yaml:"workflows"`
}

type Result struct {
	Workflow string
	Output   string
}

type Runner interface {
	Run(name string, extraArgs []string) (Result, error)
	ListWorkflows() []string
}

type CommandRunner struct {
	workflows      map[string]Definition
	defaultTimeout time.Duration
}

func LoadDefinitions(path string) (map[string]Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflows file %s: %w", path, err)
	}

	var parsed fileDefinition
	if err = yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse workflows file %s: %w", path, err)
	}

	if len(parsed.Workflows) == 0 {
		return nil, errors.New("no workflows configured")
	}
	for name, workflow := range parsed.Workflows {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("workflow name cannot be empty")
		}
		if strings.TrimSpace(workflow.Command) == "" {
			return nil, fmt.Errorf("workflow %q command cannot be empty", name)
		}
	}
	return parsed.Workflows, nil
}

func NewRunner(workflows map[string]Definition, defaultTimeout time.Duration) *CommandRunner {
	if defaultTimeout <= 0 {
		defaultTimeout = 120 * time.Second
	}
	return &CommandRunner{
		workflows:      workflows,
		defaultTimeout: defaultTimeout,
	}
}

func (r *CommandRunner) Run(name string, extraArgs []string) (Result, error) {
	definition, ok := r.workflows[name]
	if !ok {
		return Result{}, fmt.Errorf("workflow %q not found", name)
	}
	if len(extraArgs) > 0 && !definition.AllowExtraArgs {
		return Result{}, fmt.Errorf("workflow %q does not allow extra args", name)
	}

	args := slices.Clone(definition.Args)
	args = append(args, extraArgs...)

	timeout := r.defaultTimeout
	if definition.TimeoutSeconds > 0 {
		timeout = time.Duration(definition.TimeoutSeconds) * time.Second
	}

	cmd := exec.Command(definition.Command, args...)
	cmd.Dir = definition.WorkingDir
	if len(definition.EnvironmentVars) > 0 {
		cmd.Env = append(os.Environ(), toEnv(definition.EnvironmentVars)...)
	}

	done := make(chan struct{})
	var output []byte
	var cmdErr error
	go func() {
		output, cmdErr = cmd.CombinedOutput()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return Result{
			Workflow: name,
			Output:   string(output),
		}, cmdErr
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		return Result{
			Workflow: name,
			Output:   string(output),
		}, fmt.Errorf("workflow %q timed out after %s", name, timeout)
	}
}

func (r *CommandRunner) ListWorkflows() []string {
	names := make([]string, 0, len(r.workflows))
	for name := range r.workflows {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func toEnv(source map[string]string) []string {
	env := make([]string, 0, len(source))
	for key, value := range source {
		env = append(env, key+"="+value)
	}
	return env
}

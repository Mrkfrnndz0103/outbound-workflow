package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type blueprint struct {
	Services []service `yaml:"services"`
}

type service struct {
	Name         string   `yaml:"name"`
	BuildCommand string   `yaml:"buildCommand"`
	StartCommand string   `yaml:"startCommand"`
	Dockerfile   string   `yaml:"dockerfilePath"`
	DockerCtx    string   `yaml:"dockerContext"`
	EnvVars      []envVar `yaml:"envVars"`
}

type envVar struct {
	Key   string `yaml:"key"`
	Value any    `yaml:"value"`
	Sync  *bool  `yaml:"sync"`
}

var (
	envKeyRe     = regexp.MustCompile(`\b(?:WF\d+_[A-Z0-9_]+|SEATALK_[A-Z0-9_]+|GOOGLE_APPLICATION_CREDENTIALS|PORT)\b`)
	cmdDirRe     = regexp.MustCompile(`\./cmd/([a-zA-Z0-9\-_]+)`)
	dockerDirRe  = regexp.MustCompile(`(?:^|[/\\])cmd[/\\]([a-zA-Z0-9\-_]+)(?:[/\\]|$)`)
	prefixKeyRe  = regexp.MustCompile(`^(WF\d+)_`)
	sharedKeySet = map[string]struct{}{
		"SEATALK_SYSTEM_WEBHOOK_URL":     {},
		"SEATALK_BASE_URL":               {},
		"SEATALK_APP_ID":                 {},
		"SEATALK_APP_SECRET":             {},
		"GOOGLE_APPLICATION_CREDENTIALS": {},
		"PORT":                           {},
	}
	missingIgnore = map[string]struct{}{
		"GOOGLE_APPLICATION_CREDENTIALS": {},
		"PORT":                           {},
	}
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "generate render env doc: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	raw, err := os.ReadFile("render.yaml")
	if err != nil {
		return err
	}
	var bp blueprint
	if err = yaml.Unmarshal(raw, &bp); err != nil {
		return err
	}

	var out bytes.Buffer
	out.WriteString("# Render Environment Variables\n\n")
	out.WriteString("Generated from `render.yaml` and workflow code env lookups.\n\n")
	out.WriteString("Regenerate with:\n\n")
	out.WriteString("```powershell\n")
	out.WriteString("go run ./scripts/generate_render_env_doc.go\n")
	out.WriteString("```\n\n")
	out.WriteString("Auto-update on local `.env` / `.env.example` / `render.yaml` changes:\n\n")
	out.WriteString("```powershell\n")
	out.WriteString("powershell -ExecutionPolicy Bypass -File ./scripts/watch_render_env_doc.ps1\n")
	out.WriteString("```\n\n")

	for _, svc := range bp.Services {
		cmdDir := inferCmdDir(svc)
		renderMap := make(map[string]envVar, len(svc.EnvVars))
		renderKeys := make([]string, 0, len(svc.EnvVars))
		for _, ev := range svc.EnvVars {
			k := strings.TrimSpace(ev.Key)
			if k == "" {
				continue
			}
			renderMap[k] = ev
			renderKeys = append(renderKeys, k)
		}
		sort.Strings(renderKeys)

		out.WriteString("## ")
		out.WriteString(svc.Name)
		out.WriteString("\n\n")
		out.WriteString("- Build command: `")
		out.WriteString(strings.TrimSpace(svc.BuildCommand))
		out.WriteString("`\n")
		if strings.TrimSpace(svc.Dockerfile) != "" {
			out.WriteString("- Dockerfile: `")
			out.WriteString(strings.TrimSpace(svc.Dockerfile))
			out.WriteString("`\n")
		}
		if strings.TrimSpace(svc.DockerCtx) != "" {
			out.WriteString("- Docker context: `")
			out.WriteString(strings.TrimSpace(svc.DockerCtx))
			out.WriteString("`\n")
		}
		if cmdDir != "" {
			out.WriteString("- Workflow source: `")
			out.WriteString(cmdDir)
			out.WriteString("`\n")
		}
		out.WriteString("\n")

		out.WriteString("### Render Vars (`render.yaml`)\n\n")
		out.WriteString("| Key | Management | Value |\n")
		out.WriteString("| --- | --- | --- |\n")
		for _, k := range renderKeys {
			ev := renderMap[k]
			mode := renderMode(ev)
			val := renderValue(ev)
			out.WriteString("| `")
			out.WriteString(k)
			out.WriteString("` | ")
			out.WriteString(mode)
			out.WriteString(" | ")
			out.WriteString(val)
			out.WriteString(" |\n")
		}
		out.WriteString("\n")

		if cmdDir == "" {
			out.WriteString("### Code Scan\n\n")
			out.WriteString("Could not infer command directory from build command.\n\n")
			continue
		}

		codeKeys, scanErr := collectCodeEnvKeys(cmdDir)
		if scanErr != nil {
			return scanErr
		}
		servicePrefixes := detectServicePrefixes(renderKeys)
		filteredCodeKeys := filterKeysForService(codeKeys, servicePrefixes)

		missing := make([]string, 0)
		for _, key := range filteredCodeKeys {
			if _, ok := renderMap[key]; ok {
				continue
			}
			if _, skip := missingIgnore[key]; skip {
				continue
			}
			missing = append(missing, key)
		}
		sort.Strings(missing)

		out.WriteString("### Code Scan (Env Keys)\n\n")
		out.WriteString("- Detected keys (prefix-filtered for this service): `")
		out.WriteString(strings.Join(filteredCodeKeys, "`, `"))
		out.WriteString("`\n")
		if len(missing) == 0 {
			out.WriteString("- Missing from `render.yaml`: none\n\n")
		} else {
			out.WriteString("- Missing from `render.yaml`:\n")
			for _, key := range missing {
				out.WriteString("  - `")
				out.WriteString(key)
				out.WriteString("`\n")
			}
			out.WriteString("\n")
		}
	}

	target := filepath.Join("docs", "render-env.md")
	return os.WriteFile(target, out.Bytes(), 0o644)
}

func inferCmdDir(s service) string {
	m := cmdDirRe.FindStringSubmatch(s.BuildCommand)
	if len(m) >= 2 {
		return filepath.ToSlash(filepath.Join("cmd", m[1]))
	}
	dm := dockerDirRe.FindStringSubmatch(strings.TrimSpace(s.Dockerfile))
	if len(dm) >= 2 {
		return filepath.ToSlash(filepath.Join("cmd", dm[1]))
	}
	return ""
}

func renderMode(ev envVar) string {
	if ev.Sync != nil && !*ev.Sync {
		return "`sync: false` (unmanaged/secret)"
	}
	if ev.Value != nil {
		return "`value` (managed)"
	}
	return "unset"
}

func renderValue(ev envVar) string {
	if ev.Value == nil {
		return "-"
	}
	raw := strings.TrimSpace(fmt.Sprint(ev.Value))
	if raw == "" {
		return "`\"\"`"
	}
	return "`" + strings.ReplaceAll(raw, "`", "'") + "`"
}

func collectCodeEnvKeys(cmdDir string) ([]string, error) {
	entries := make([]string, 0)
	err := filepath.WalkDir(cmdDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		entries = append(entries, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)

	set := map[string]struct{}{}
	for _, path := range entries {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		for _, key := range envKeyRe.FindAllString(string(raw), -1) {
			set[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func detectServicePrefixes(renderKeys []string) map[string]struct{} {
	prefixes := map[string]struct{}{}
	for _, key := range renderKeys {
		m := prefixKeyRe.FindStringSubmatch(key)
		if len(m) == 2 {
			prefixes[m[1]] = struct{}{}
		}
	}
	return prefixes
}

func filterKeysForService(keys []string, prefixes map[string]struct{}) []string {
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := sharedKeySet[key]; ok {
			filtered = append(filtered, key)
			continue
		}
		m := prefixKeyRe.FindStringSubmatch(key)
		if len(m) != 2 {
			continue
		}
		if _, ok := prefixes[m[1]]; ok {
			filtered = append(filtered, key)
		}
	}
	sort.Strings(filtered)
	return filtered
}

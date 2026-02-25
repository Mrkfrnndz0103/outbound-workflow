package bot

import "strings"

type command struct {
	name string
	args []string
}

func parseCommand(text string, prefix string) (command, bool) {
	content := strings.TrimSpace(text)
	if prefix != "" {
		if !strings.HasPrefix(content, prefix) {
			return command{}, false
		}
		content = strings.TrimSpace(strings.TrimPrefix(content, prefix))
	}
	if content == "" {
		return command{}, false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return command{}, false
	}
	return command{
		name: strings.ToLower(parts[0]),
		args: parts[1:],
	}, true
}

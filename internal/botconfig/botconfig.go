package botconfig

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

const (
	defaultTabName = "bot_config"
)

type Row struct {
	Mode          string
	Workflow      string
	TargetGroup   string
	AppID         string
	AppSecret     string
	SigningSecret string
	WebhookURL    string
	SourceRow     int
}

type OwnedGroup struct {
	GroupID string
	BotName string
}

func NormalizeTabName(tab string) string {
	trimmed := strings.TrimSpace(tab)
	if trimmed == "" {
		return defaultTabName
	}
	return trimmed
}

func LoadRowsFromSheet(ctx context.Context, sheetsSvc *sheets.Service, sheetID, tab string) ([]Row, error) {
	if sheetsSvc == nil {
		return nil, errors.New("sheets service is required")
	}
	trimmedSheetID := strings.TrimSpace(sheetID)
	if trimmedSheetID == "" {
		return nil, errors.New("sheet id is required")
	}
	rangeRef := fmt.Sprintf("%s!A2:I", NormalizeTabName(tab))
	resp, err := sheetsSvc.Spreadsheets.Values.Get(trimmedSheetID, rangeRef).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read bot config range %s: %w", rangeRef, err)
	}

	rows := make([]Row, 0, len(resp.Values))
	for idx, values := range resp.Values {
		sourceRow := idx + 2
		row := Row{
			Mode:          strings.ToLower(normalizedCell(values, 0)),
			Workflow:      strings.ToLower(normalizedCell(values, 1)),
			TargetGroup:   normalizedCell(values, 2),
			AppID:         normalizedCell(values, 5),
			AppSecret:     normalizedCell(values, 6),
			SigningSecret: normalizedCell(values, 7),
			WebhookURL:    normalizedCell(values, 8),
			SourceRow:     sourceRow,
		}
		if isRowEmpty(row) {
			continue
		}
		if row.Mode != "" && row.Mode != "bot" && row.Mode != "webhook" {
			return nil, fmt.Errorf("invalid bot_mode at row %d: %q", sourceRow, row.Mode)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func ResolveForWorkflow(rows []Row, workflowTag string) (Row, error) {
	normalizedWorkflow := strings.ToLower(strings.TrimSpace(workflowTag))
	if normalizedWorkflow == "" {
		return Row{}, errors.New("workflow tag is required")
	}
	matches := make([]Row, 0, 1)
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Workflow), normalizedWorkflow) {
			matches = append(matches, row)
		}
	}
	if len(matches) == 0 {
		return Row{}, fmt.Errorf("workflow %q not found in bot_config", normalizedWorkflow)
	}
	if len(matches) > 1 {
		return Row{}, fmt.Errorf("workflow %q has duplicate bot_config rows", normalizedWorkflow)
	}
	return matches[0], nil
}

func ValidateResolvedRow(row Row) error {
	switch row.Mode {
	case "bot":
		if strings.TrimSpace(row.TargetGroup) == "" {
			return fmt.Errorf("workflow %q row %d: target_group is required for bot mode", row.Workflow, row.SourceRow)
		}
		if strings.TrimSpace(row.AppID) == "" || strings.TrimSpace(row.AppSecret) == "" {
			return fmt.Errorf("workflow %q row %d: app_id and app_secret are required for bot mode", row.Workflow, row.SourceRow)
		}
	case "webhook":
		if strings.TrimSpace(row.WebhookURL) == "" {
			return fmt.Errorf("workflow %q row %d: webhook_url is required for webhook mode", row.Workflow, row.SourceRow)
		}
	default:
		return fmt.Errorf("workflow %q row %d: bot_mode must be bot or webhook", row.Workflow, row.SourceRow)
	}
	return nil
}

func BuildOwnedGroupRows(raw []OwnedGroup) []OwnedGroup {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]OwnedGroup, 0, len(raw))
	for _, item := range raw {
		groupID := strings.TrimSpace(item.GroupID)
		botName := strings.TrimSpace(item.BotName)
		if groupID == "" || botName == "" {
			continue
		}
		key := strings.ToLower(botName) + "\n" + groupID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, OwnedGroup{
			GroupID: groupID,
			BotName: botName,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		leftBot := strings.ToLower(out[i].BotName)
		rightBot := strings.ToLower(out[j].BotName)
		if leftBot == rightBot {
			return out[i].GroupID < out[j].GroupID
		}
		return leftBot < rightBot
	})
	return out
}

func normalizedCell(values []interface{}, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(values[idx]))
}

func isRowEmpty(row Row) bool {
	return strings.TrimSpace(row.Mode) == "" &&
		strings.TrimSpace(row.Workflow) == "" &&
		strings.TrimSpace(row.TargetGroup) == "" &&
		strings.TrimSpace(row.AppID) == "" &&
		strings.TrimSpace(row.AppSecret) == "" &&
		strings.TrimSpace(row.SigningSecret) == "" &&
		strings.TrimSpace(row.WebhookURL) == ""
}

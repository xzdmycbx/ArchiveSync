package notify

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// timeLayout is the human-readable timestamp format used in rendered text.
// Run timestamps are stored in UTC (see SPEC §4.4), so the zone suffix reflects
// whatever location the time carries.
const timeLayout = "2006-01-02 15:04:05 MST"

// statusText returns a short Chinese label for an event type / run status.
func statusText(evType, status string) string {
	s := status
	if s == "" {
		s = evType
	}
	switch s {
	case "start", "running":
		return "开始"
	case "success":
		return "成功"
	case "partial":
		return "部分成功"
	case "failure", "failed":
		return "失败"
	default:
		return s
	}
}

// subject returns a short one-line title for the event, suitable for an email
// subject or a rich-message heading.
func subject(ev Event) string {
	if ev.Title != "" {
		return ev.Title
	}
	if ev.TargetName != "" {
		return ev.TargetName
	}
	if ev.Run != nil && ev.Run.TargetName != "" {
		return ev.Run.TargetName
	}
	return "ArchiveSync 通知"
}

// humanBytes formats a byte count using binary (IEC) units, e.g. "1.5 MiB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

// plainText renders an Event into multi-line human-readable text: title,
// target, status, times, duration, size, file count, per-destination results
// and any message/error. When ev.Run is non-nil its detailed fields are used.
func plainText(ev Event) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n", subject(ev))

	target := ev.TargetName
	if target == "" && ev.Run != nil {
		target = ev.Run.TargetName
	}
	if target != "" {
		fmt.Fprintf(&b, "目标: %s\n", target)
	}

	rstatus := ""
	if ev.Run != nil {
		rstatus = string(ev.Run.Status)
	}
	fmt.Fprintf(&b, "状态: %s\n", statusText(ev.Type, rstatus))

	if ev.Run != nil {
		r := ev.Run
		if !r.StartedAt.IsZero() {
			fmt.Fprintf(&b, "开始: %s\n", r.StartedAt.Format(timeLayout))
		}
		if r.FinishedAt != nil && !r.FinishedAt.IsZero() {
			fmt.Fprintf(&b, "结束: %s\n", r.FinishedAt.Format(timeLayout))
		}
		if r.DurationMs > 0 {
			d := time.Duration(r.DurationMs) * time.Millisecond
			fmt.Fprintf(&b, "耗时: %s\n", d.Round(time.Millisecond))
		}
		if r.SizeBytes > 0 {
			fmt.Fprintf(&b, "大小: %s\n", humanBytes(r.SizeBytes))
		}
		if r.FileCount > 0 {
			fmt.Fprintf(&b, "文件数: %d\n", r.FileCount)
		}
		if r.ArchiveKey != "" {
			fmt.Fprintf(&b, "归档: %s\n", r.ArchiveKey)
		}
		if len(r.Destinations) > 0 {
			b.WriteString("目的渠道:\n")
			for _, d := range r.Destinations {
				st := "成功"
				if !d.Success {
					st = "失败"
				}
				name := d.ChannelName
				if name == "" {
					name = d.ChannelID
				}
				fmt.Fprintf(&b, "  · %s — %s", name, st)
				if d.Success && d.Pruned > 0 {
					fmt.Fprintf(&b, "（清理 %d）", d.Pruned)
				}
				if !d.Success && d.Error != "" {
					fmt.Fprintf(&b, "：%s", d.Error)
				}
				b.WriteString("\n")
			}
		}
	} else if !ev.Timestamp.IsZero() {
		fmt.Fprintf(&b, "时间: %s\n", ev.Timestamp.Format(timeLayout))
	}

	if len(ev.Fields) > 0 {
		keys := make([]string, 0, len(ev.Fields))
		for k := range ev.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %s\n", k, ev.Fields[k])
		}
	}

	msg := ev.Message
	if msg == "" && ev.Run != nil {
		msg = ev.Run.Message
	}
	if msg != "" {
		if ev.Type == "failure" {
			fmt.Fprintf(&b, "错误: %s\n", msg)
		} else {
			fmt.Fprintf(&b, "信息: %s\n", msg)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// embedColor maps an event type to a Discord embed sidebar color.
func embedColor(evType string) int {
	switch evType {
	case "success":
		return 0x22C55E // green
	case "failure":
		return 0xEF4444 // red
	case "start":
		return 0x5865F2 // blurple
	default:
		return 0x94A3B8 // slate
	}
}

// discordEmbed renders an Event as a clean Discord embed card (no emoji): a
// colored sidebar, an author line with the target, inline status fields, a
// per-destination results field and an ArchiveSync footer.
func discordEmbed(ev Event) map[string]any {
	fields := make([]map[string]any, 0, 6)
	add := func(name, val string, inline bool) {
		if val != "" {
			fields = append(fields, map[string]any{"name": name, "value": val, "inline": inline})
		}
	}

	target := ev.TargetName
	if target == "" && ev.Run != nil {
		target = ev.Run.TargetName
	}

	var desc string
	if r := ev.Run; r != nil {
		add("状态", statusText(ev.Type, string(r.Status)), true)
		if r.DurationMs > 0 {
			add("耗时", (time.Duration(r.DurationMs) * time.Millisecond).Round(time.Millisecond).String(), true)
		}
		if r.SizeBytes > 0 {
			add("大小", humanBytes(r.SizeBytes), true)
		}
		if r.FileCount > 0 {
			add("文件数", fmt.Sprintf("%d", r.FileCount), true)
		}
		if tr := ev.Fields["触发方式"]; tr != "" {
			add("触发", tr, true)
		}
		if len(r.Destinations) > 0 {
			lines := make([]string, 0, len(r.Destinations))
			for _, d := range r.Destinations {
				name := d.ChannelName
				if name == "" {
					name = d.ChannelID
				}
				st := "成功"
				if !d.Success {
					st = "失败"
				}
				line := "• " + name + " — " + st
				if d.Success && d.Pruned > 0 {
					line += fmt.Sprintf("（清理 %d 份旧备份）", d.Pruned)
				}
				if !d.Success && d.Error != "" {
					line += "：" + d.Error
				}
				lines = append(lines, line)
			}
			add("目的渠道", strings.Join(lines, "\n"), false)
		}
		if r.ArchiveKey != "" {
			desc = "`" + r.ArchiveKey + "`"
		}
		if ev.Type == "failure" && r.Message != "" {
			add("错误信息", r.Message, false)
		}
	} else {
		add("状态", statusText(ev.Type, ""), true)
	}

	if desc == "" {
		if ev.Message != "" {
			desc = ev.Message
		} else if ev.Run != nil {
			desc = ev.Run.Message
		}
	}

	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	embed := map[string]any{
		"title":     subject(ev),
		"color":     embedColor(ev.Type),
		"fields":    fields,
		"timestamp": ts.UTC().Format(time.RFC3339),
		"footer":    map[string]any{"text": "ArchiveSync"},
	}
	if target != "" {
		embed["author"] = map[string]any{"name": target}
	}
	if desc != "" {
		embed["description"] = desc
	}
	return embed
}

// 文件：describer-go/text/content.go —— 文本内容定位：链接主机、围栏语言、章节路径与合法管道表头
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package text

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var contentURL = regexp.MustCompile(`(?i)https?://[^\s<>"\x60]+`)
var contentHeading = regexp.MustCompile(`^ {0,3}(#{1,6})[\t ]+(.+)$`)
var tableDivider = regexp.MustCompile(`^:?-{3,}:?$`)

type contentList struct {
	values    []string
	seen      map[string]bool
	truncated bool
}

func (l *contentList) add(s string) {
	if s == "" {
		return
	}
	if utf8.RuneCountInString(s) > 200 {
		l.truncated = true
		return
	}
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	if l.seen[s] {
		return
	}
	if len(l.values) == 30 {
		l.truncated = true
		return
	}
	l.seen[s] = true
	l.values = append(l.values, s)
}

// contentFacts 一次扫描共享给四个 extractor；不是 Markdown 渲染器。
func contentFacts(c *textCtx) map[string]*contentList {
	if c.content != nil {
		return c.content
	}
	out := map[string]*contentList{}
	for _, k := range []string{"link-domains", "code-languages", "section-paths", "table-headers"} {
		out[k] = &contentList{}
	}
	for _, raw := range contentURL.FindAllString(c.decoded, -1) {
		raw = strings.TrimRight(raw, ").,;!?]}，。；！？）")
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			out["link-domains"].add(strings.ToLower(u.Hostname()))
		}
	}
	var stack []struct {
		level int
		title string
	}
	var fence byte
	fenceLen := 0
	previous := ""
	for _, raw := range c.lines {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimLeft(line, " ")
		indent := len(line) - len(trim)
		if indent <= 3 && len(trim) >= 3 && (trim[0] == '`' || trim[0] == '~') {
			n := 0
			for n < len(trim) && trim[n] == trim[0] {
				n++
			}
			if n >= 3 {
				rest := strings.TrimSpace(trim[n:])
				if fence != 0 {
					if trim[0] == fence && n >= fenceLen && rest == "" {
						fence = 0
					}
				} else if trim[0] != '`' || !strings.Contains(rest, "`") {
					fence, fenceLen = trim[0], n
					if words := strings.Fields(rest); len(words) > 0 {
						out["code-languages"].add(strings.ToLower(words[0]))
					}
				}
				previous = ""
				continue
			}
		}
		if fence != 0 {
			continue
		}
		if m := contentHeading.FindStringSubmatch(line); m != nil {
			title := strings.TrimSpace(m[2])
			// Optional closing hashes are only syntactic when separated by whitespace.
			end := strings.TrimRight(title, "#")
			if end != title && strings.HasSuffix(end, " ") {
				title = strings.TrimSpace(end)
			}
			level := len(m[1])
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, struct {
				level int
				title string
			}{level, title})
			parts := make([]string, len(stack))
			for i, h := range stack {
				parts[i] = h.title
			}
			out["section-paths"].add(strings.Join(parts, " / "))
		}
		cells := pipeCells(line)
		head := pipeCells(previous)
		valid := len(cells) > 0 && len(cells) == len(head)
		for _, cell := range cells {
			if !tableDivider.MatchString(cell) {
				valid = false
			}
		}
		if valid {
			for _, cell := range head {
				out["table-headers"].add(cell)
			}
		}
		previous = line
	}
	c.content = out
	return out
}

// pipeCells honors escaped pipes; a pipe is required to avoid recognizing prose.
func pipeCells(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "|") {
		return nil
	}
	if strings.HasPrefix(s, "|") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, `\|`) {
		s = s[:len(s)-1]
	}
	var cells []string
	var b strings.Builder
	escape := false
	for _, r := range s {
		if escape {
			b.WriteRune(r)
			escape = false
			continue
		}
		if r == '\\' {
			escape = true
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		} else {
			b.WriteRune(r)
		}
	}
	if escape {
		b.WriteRune('\\')
	}
	return append(cells, strings.TrimSpace(b.String()))
}

func contentExtract(key string) func(*textCtx) any {
	return func(c *textCtx) any {
		l := contentFacts(c)[key]
		if len(l.values) == 0 {
			return nil
		}
		return l.values
	}
}

func contentTruncated(key string) func(*textCtx) any {
	return func(c *textCtx) any {
		l := contentFacts(c)[key]
		if len(l.values) == 0 && !l.truncated {
			return nil
		}
		return l.truncated
	}
}

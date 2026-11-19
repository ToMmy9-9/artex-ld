// steps.go：把「复现步骤」解析到真实录制的流量报文（AntiFox 证据驱动交付）。
//
// 思路（与 AntiFox 同源，但按本仓库 traffic 包的表结构/索引适配）：
// agent 在挖洞时把每一步的**逐字**请求/响应写进 findings.reproduction；
// 报告生成时需要把每一步关联到「当时真实录制的那个包」，而不是让 AI 复述或编造。
//
// ResolveSteps 按**时间顺序**逐个解析：每一步必须落在上一步之后、任务流量时间窗内，
// 因此同一端点的多次相同请求可以被无歧义地依次对上（step1 取最早、step2 取其后…）。
//
// 判别式逐级收窄：先用 method+path 定位，再用 query / body 关键字 / 响应特征细化；
// 每个判别式只在「仍剩 ≥1 个候选」时才生效——所以 AI 转述过的字段不会把匹配打成 0。

package traffic

import (
	"regexp"
	"strings"
)

// StepRef is one reproduction step's verbatim request/response text, to be resolved
// to the real recorded exchange that cites it.
type StepRef struct {
	Request  string
	Response string
}

// ResolveSteps maps an ORDERED list of reproduction steps to real recorded exchanges,
// constrained to the task's traffic time window [sinceUnix, untilUnix] (Unix seconds).
// Returns one exchange id per step ("" when unresolved).
func (t *Traffic) ResolveSteps(sinceUnix, untilUnix int64, steps []StepRef) []string {
	out := make([]string, len(steps))
	cursor := int64(0) // previous step's exchange ts (chronological lower bound)
	for i, st := range steps {
		method, path, query, bodyToken := parseStepRequest(st.Request)
		if method == "" || path == "" {
			continue
		}
		if id, ts, ok := t.resolveStepInWindow(sinceUnix, untilUnix, cursor, method, path, query, bodyToken, st.Response); ok {
			out[i] = id
			cursor = ts
		}
	}
	return out
}

// resolveStepInWindow finds the earliest exchange matching one step, at-or-after
// cursor, inside the task's time window. Returns (id, ts, ok).
func (t *Traffic) resolveStepInWindow(sinceUnix, untilUnix, cursor int64, method, path, query, bodyToken, respText string) (string, int64, bool) {
	where := `e.method=? AND (e.url = ? OR e.url LIKE ?) AND e.ts BETWEEN ? AND ? AND e.ts >= ?`
	args := []any{strings.ToUpper(method), path, likePrefix(path), sinceUnix, untilUnix, cursor}
	// tryNarrow applies a discriminator only if it still leaves ≥1 candidate, so a
	// paraphrased/over-specific field can't zero out a match that should succeed.
	tryNarrow := func(cond string, arg any) {
		w, a := where+" AND "+cond, append(append([]any{}, args...), arg)
		if len(t.queryIDs(w, a)) >= 1 {
			where, args = w, a
		}
	}
	if t.fts && bodyToken != "" {
		tryNarrow("e.rowid IN (SELECT rowid FROM ex_fts WHERE ex_fts MATCH ?)", ftsQuote(bodyToken))
	}
	if query != "" {
		tryNarrow(`e.url LIKE ? ESCAPE '\'`, likeContains(query))
	}
	if t.fts {
		for _, disc := range respDiscriminators(respText) {
			if len(t.queryIDs(where, args)) <= 1 {
				break
			}
			tryNarrow("e.rowid IN (SELECT rowid FROM ex_fts WHERE ex_fts MATCH ?)", ftsQuote(disc))
		}
	}
	var id string
	var ts int64
	if err := t.db.QueryRow(`SELECT e.id, e.ts FROM exchanges e WHERE `+where+` ORDER BY e.ts ASC LIMIT 1`, args...).Scan(&id, &ts); err != nil {
		return "", 0, false
	}
	return id, ts, true
}

// queryIDs returns the ids of exchanges matching a WHERE clause (used to test
// whether a narrowing step still leaves candidates).
func (t *Traffic) queryIDs(where string, args []any) []string {
	rows, err := t.db.Query(`SELECT e.id FROM exchanges e WHERE `+where, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return out
		}
		out = append(out, id)
	}
	return out
}

// Reproduction-step request parsing: a step's "request" is either a verbatim HTTP
// packet or the command the agent ran (curl/shell). Both forms are resolved to the
// real recorded exchange via these patterns.
var (
	reHTTPReqLine = regexp.MustCompile(`^([A-Z]+)\s+(\S+)\s+HTTP/\S+\s*$`)
	reURLInText   = regexp.MustCompile("https?://[^\\s\"'`;)<>]+")
	reCurlMethod  = regexp.MustCompile(`(?:-X|--request)\s+([A-Za-z]+)`)
	reBodyFlag    = regexp.MustCompile(`(?:^|\s)(?:-F|--form|-d|--data|--data-raw|--data-binary)(?:\s|=)`)
	reUploadName  = regexp.MustCompile(`filename=["']?([^"'\s;,]+)`)
	reCurlData    = regexp.MustCompile(`(?:-d|--data(?:-raw|-binary)?)[ =]\s*["']?([^"'\n]+)`)
)

// parseStepRequest extracts method, url PATH (query stripped), the url QUERY, and a
// body discriminator token from a step's request text — whether it's a raw HTTP
// request packet ("METHOD /path?q PROTO\n…\n\n<body>") or a curl/shell command the
// agent executed. query (for GET-style requests) and the body token (upload filename
// / POST data / leading body slice) disambiguate same-endpoint requests; both may be
// empty when nothing distinctive is present.
func parseStepRequest(text string) (method, path, query, bodyToken string) {
	// 1) Raw HTTP request packet.
	if m := reHTTPReqLine.FindStringSubmatch(firstNonEmptyLine(text)); m != nil {
		method = strings.ToUpper(m[1])
		path, query = splitQuery(m[2])
		if i := strings.Index(text, "\n\n"); i >= 0 {
			body := text[i+2:]
			// Prefer the upload filename (strongest discriminator, and avoids a whole-body
			// FTS miss when the agent paraphrased the multipart boundary as a placeholder).
			if m := reUploadName.FindStringSubmatch(body); m != nil {
				bodyToken = m[1]
			} else {
				bodyToken = bodyPhrase(body)
			}
		}
		return method, path, query, bodyToken
	}
	// 2) A URL embedded in free text (e.g. a curl command or a prose reference).
	url := ""
	if m := reURLInText.FindString(text); m != "" {
		url = m
	}
	method = "GET"
	if m := reCurlMethod.FindStringSubmatch(text); m != nil {
		method = strings.ToUpper(m[1])
	}
	if url == "" {
		return "", "", "", ""
	}
	path, query = splitQuery(url)
	// Strip scheme+host: the recorded exchange stores the path (and sometimes the
	// full URL), so match on the path portion.
	if i := strings.Index(path, "://"); i >= 0 {
		rest := path[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			path = rest[j:]
		} else {
			path = "/"
		}
	}
	// 3) A shell/curl body payload.
	if reBodyFlag.MatchString(text) {
		if m := reCurlData.FindStringSubmatch(text); m != nil {
			bodyToken = strings.TrimSpace(m[1])
		}
	}
	if bodyToken == "" {
		if i := strings.Index(text, "\n\n"); i >= 0 {
			bodyToken = bodyPhrase(text[i+2:])
		}
	}
	return method, path, query, bodyToken
}

// firstNonEmptyLine returns the first line that has non-whitespace content.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// splitQuery splits a URL into its path and query parts.
func splitQuery(raw string) (path, query string) {
	if i := strings.Index(raw, "?"); i >= 0 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

// bodyPhrase picks a short distinctive phrase from a request body for FTS matching.
// It trims to a reasonable length so the FTS term stays selective.
func bodyPhrase(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	// Take the first line of a form body, or the whole (trimmed) body for JSON.
	if i := strings.IndexAny(body, "\r\n"); i >= 0 {
		body = strings.TrimSpace(body[:i])
	}
	if len(body) > 80 {
		body = body[:80]
	}
	return strings.TrimSpace(body)
}

// respDiscriminators extracts short distinctive tokens from a response body to
// disambiguate repeated requests to one endpoint. Only tokens long enough to be
// selective are returned.
func respDiscriminators(resp string) []string {
	var out []string
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 12 {
			continue
		}
		// Prefer JSON-ish key/value pairs and quoted values.
		if i := strings.Index(line, ":"); i > 0 && i < 40 {
			v := strings.Trim(strings.TrimSpace(line[i+1:]), `",`)
			if len(v) >= 8 && !strings.ContainsAny(v, " \t") {
				out = append(out, v)
				if len(out) >= 3 {
					return out
				}
			}
		}
	}
	return out
}

// likeContains builds a LIKE pattern matching a substring (escaping % and _).
func likeContains(s string) string {
	return "%" + escapeLike(s) + "%"
}

// likePrefix builds a LIKE pattern matching a path prefix.
func likePrefix(s string) string {
	return escapeLike(s) + "%"
}

// escapeLike escapes the LIKE wildcards in a literal.
func escapeLike(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return r.Replace(s)
}

// MetaByIDs returns the metadata of the given exchange ids, in the order they
// were requested (ids that do not exist are skipped). Used by the report pipeline
// to label each linked packet without re-reading every body.
func (t *Traffic) MetaByIDs(ids []string) ([]ExchangeMeta, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := t.db.Query(`SELECT `+exchangeMetaCols+` FROM exchanges WHERE id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]ExchangeMeta{}
	for rows.Next() {
		m, err := scanExchangeMeta(rows)
		if err != nil {
			return nil, err
		}
		byID[m.ID] = m
	}
	out := make([]ExchangeMeta, 0, len(ids))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

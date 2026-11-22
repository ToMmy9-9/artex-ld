// api_messages_test.go：守住"中文产品不给用户看英文错误"这条线。
//
// 背景（真实体验缺陷）：全仓有 250 多处 writeErr 直接写英文字面量
// （task not found / bad finding id / database unavailable / invalid JSON ...），
// 前端 toast 一律 `${(error as Error).message}`，于是中文界面上到处弹英文。
// 这一轮把这些字面量统一换成中文；这条测试负责防止以后再写回去。
//
// 三类出口分开守：
//   1. writeErr 家族的字面量参数 —— 必须带汉字
//   2. http.Error / http.NotFound —— 绕过了 writeErr，各自也会吐英文
//   3. 5xx 里的 err.Error() —— writeErr 不脱敏，拼进去就等于把驱动原文发出去
package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	// 允许跨行的 writeErr(…, "字面量") 调用；第三个参数是变量的（err.Error()、
	// errDataSourceUnavailable、fmt.Sprintf(...)）不会被匹配 —— 那些由各自的
	// 出口保证语言，见 writeDecodeErr / writeErrRedacted / writeErrInternal。
	errLiteralPattern = regexp.MustCompile(`writeErr(?:Redacted|Internal|InternalDetail)?\(\s*\w+\s*,\s*[^,()]+,\s*"((?:[^"\\]|\\.)*)"`)
	// http.Error(w, "字面量", code) 是完全绕过 writeErr 的出口。
	httpErrorPattern = regexp.MustCompile(`http\.Error\(\s*\w+\s*,\s*"((?:[^"\\]|\\.)*)"`)
	// http.NotFound 没有任何可定制的正文，它固定写 Go 的英文 "404 page not found"。
	httpNotFoundPattern = regexp.MustCompile(`\bhttp\.NotFound\(`)
	// 5xx 代码（字面量或常量写法）。
	fiveXXPattern = regexp.MustCompile(`^writeErr\(\s*\w+\s*,\s*(?:5\d\d|http\.StatusInternalServerError|http\.StatusBadGateway|http\.StatusServiceUnavailable)\s*,`)
	cjkRune       = regexp.MustCompile(`\p{Han}`)
)

// serverSources 遍历 server 包的非测试源码，交给 fn 逐文件检查。
func serverSources(t *testing.T, fn func(name, src string, offenders *[]string)) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 50 {
		t.Fatalf("只扫到 %d 个源文件，工作目录可能不对（应在 server/ 下运行）", len(files))
	}
	var offenders []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fn(name, string(raw), &offenders)
	}
	if len(offenders) > 0 {
		t.Fatalf("%s：\n  %s", t.Name(), strings.Join(offenders, "\n  "))
	}
}

// TestErrorMessagesAreChinese 扫描 server 包的非测试源码，任何 writeErr 的中文
// 字面量缺失都应该让这条测试变红。
func TestErrorMessagesAreChinese(t *testing.T) {
	checked := 0
	serverSources(t, func(name, src string, offenders *[]string) {
		for _, m := range errLiteralPattern.FindAllStringSubmatchIndex(src, -1) {
			msg := src[m[2]:m[3]]
			checked++
			if !cjkRune.MatchString(msg) && !isAllowedNonChinese(msg) {
				line := strings.Count(src[:m[0]], "\n") + 1
				*offenders = append(*offenders, name+":"+strconv.Itoa(line)+"  "+msg)
			}
		}
	})
	// 扫不到任何调用说明正则失效了，这条守卫就变成了摆设。
	if checked < 100 {
		t.Fatalf("只检查到 %d 处 writeErr 字面量，正则或目录有问题", checked)
	}
}

// TestHTTPErrorMessagesAreChinese 管住绕过 writeErr 的那些出口。
//
// 之所以单独一条：serveReportAsset 原来直接用 http.ServeFile，缺文件时吐的是
// Go 内置的 "404 page not found" —— 上一轮的守卫只扫 writeErr，完全没看见它。
func TestHTTPErrorMessagesAreChinese(t *testing.T) {
	checked := 0
	serverSources(t, func(name, src string, offenders *[]string) {
		for _, m := range httpErrorPattern.FindAllStringSubmatchIndex(src, -1) {
			msg := src[m[2]:m[3]]
			checked++
			if !cjkRune.MatchString(msg) && !isAllowedNonChinese(msg) {
				line := strings.Count(src[:m[0]], "\n") + 1
				*offenders = append(*offenders, name+":"+strconv.Itoa(line)+"  http.Error: "+msg)
			}
		}
		for _, m := range httpNotFoundPattern.FindAllStringIndex(src, -1) {
			line := strings.Count(src[:m[0]], "\n") + 1
			*offenders = append(*offenders, name+":"+strconv.Itoa(line)+
				"  http.NotFound 固定吐英文，请改用 writeErr(w, 404, \"……不存在\")")
		}
	})
	if checked == 0 {
		t.Fatal("一处 http.Error 字面量都没扫到，正则或目录有问题")
	}
}

// TestServerErrorsDoNotLeakRawError 管住 5xx 出口。
//
// writeErr 自己不脱敏：`writeErr(w, 500, err.Error())` 会把 pq:/os 的原文
// （表名、约束名、本地绝对路径）直接回给客户端。这类位置曾有 59 处。
// 5xx 要写底层原因时用 writeErrRedacted 或 writeErrInternal，它们只把
// 详情写进日志。
func TestServerErrorsDoNotLeakRawError(t *testing.T) {
	serverSources(t, func(name, src string, offenders *[]string) {
		for i, line := range strings.Split(src, "\n") {
			trimmed := strings.TrimSpace(line)
			if !fiveXXPattern.MatchString(trimmed) {
				continue
			}
			if strings.Contains(trimmed, ".Error()") || strings.Contains(trimmed, ".Err()") {
				*offenders = append(*offenders, name+":"+strconv.Itoa(i+1)+"  "+trimmed)
			}
		}
	})
}

// isAllowedNonChinese 是刻意保留的非中文文案白名单。
//
// 目前没有 —— 需要加的时候请在这里写明理由，别在 handler 里随手写英文。
func isAllowedNonChinese(string) bool { return false }

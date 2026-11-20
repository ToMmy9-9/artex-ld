// toolhint.go：目录扫描型 shell 提示（tools.kind="shell" 且 exec.dir 非空）。
//
// 与静态 shell 提示的区别：静态 shell 提示的描述文本写死在 description 里，
// 目录扫描型提示在装配 agent 时（ToolResolve）实时扫描指定目录，把当前
// 真实存在的可执行文件按一级子目录分组，渲染成一段清单注入 Bash 描述。
// 这样"往目录里丢一个工具"不需要改数据库，下次装配就生效。
//
// 安全边界（重要）：
//   - 扫描只读取目录项名称与 stat 信息，绝不读取文件内容；
//   - 目录必须由管理员在 UI 配置，模型侧的 create/update_custom_tool
//     拒绝携带 exec.dir（见 platform_tools.go），避免模型把任意路径
//     的文件名列表注入到自己的上下文里做信息收集；
//   - 平台不匹配的可执行文件（Windows 的 .exe 出现在 Linux 部署上）被过滤掉。
package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"artex/db"
)

// shellHintSpec 是目录扫描型 shell 提示的 exec 规格。
type shellHintSpec struct {
	// Dir 是要扫描的根目录（绝对路径）。为空 → 该 shell 提示是静态的。
	Dir string `json:"dir"`
	// MaxDepth 扫描的最大层数，默认 3（根目录算第 0 层）。
	MaxDepth int `json:"max_depth"`
	// MaxEntries 每个目录分组最多列出多少个文件名，默认 12。防止某个超大
	// 分组（dotnet/SharpCollection 有 200+ 个 exe）把 Bash 描述撑爆
	// （这个描述每轮 LLM 调用都会带上）。
	MaxEntries int `json:"max_entries"`
	// MaxPerGroup 覆盖 MaxEntries（每组名额的上限），留空则用 MaxEntries。
	MaxPerGroup int `json:"max_per_group"`
	// Platform 过滤目标平台："windows" / "linux" / ""（=当前运行时）。
	Platform string `json:"platform"`
	// Include 追加的可执行扩展名（在平台默认集合之外额外放行）。
	Include []string `json:"include"`
}

const (
	toolHintCacheTTL      = 60 * time.Second
	toolHintDefaultDepth  = 3
	toolHintMaxDepthHard  = 6
	toolHintMaxEntriesCap = 400
	// toolHintMaxPerGroup caps how many names one directory group may contribute
	// to the hint. Without it a group like dotnet/SharpCollection (200+ exes)
	// would blow up the Bash description, which is carried on every LLM turn.
	toolHintMaxPerGroup = 12
)

// toolHintSkipDirs are directory names never worth listing: dependency trees,
// build output, test fixtures and docs. Real tool collections carry plenty of
// these (pyenv/site-packages, sqlmap/thirdparty, wafw00f/tests) and they
// would otherwise dominate the listing with noise.
var toolHintSkipDirs = map[string]bool{
	"site-packages":   true,
	"thirdparty":      true,
	"third_party":     true,
	"node_modules":    true,
	"__pycache__":     true,
	"tests":           true,
	"test":            true,
	"docs":            true,
	"doc":             true,
	"examples":        true,
	"example":         true,
	"vendor":          true,
	"dist":            true,
	"build":           true,
	"tmp":             true,
	"temp":            true,
	"venv":            true,
	".venv":           true,
	"env":             true,
	"site-packages-bak": true,
}

// toolHintSkipFiles are file names that are never entry points.
var toolHintSkipFiles = map[string]bool{
	"__init__.py":   true,
	"__main__.py":   true,
	"setup.py":      true,
	"conftest.py":   true,
	"requirements.txt": true,
	"pipelines.txt": true,
	"pyproject.toml": true,
}

// toolHintFile 是一个被扫描到的可执行文件。
type toolHintFile struct {
	// rel 是相对根目录的路径，统一用 "/" 分隔（模型读起来更顺）。
	rel string
	// group 是一级子目录名；文件直接躺在根目录时为 ""。
	group string
	// tail 是去掉一级子目录后的剩余路径。
	tail string
}

// toolHintCacheEntry 缓存一次扫描的渲染结果。
type toolHintCacheEntry struct {
	text string
	at   time.Time
}

var toolHintCache = struct {
	mu sync.Mutex
	m  map[string]toolHintCacheEntry
}{m: map[string]toolHintCacheEntry{}}

// renderShellHint 返回注入 Bash 描述的一行提示。静态 shell 提示（无 exec.dir）
// 沿用旧格式 "- key: description"；目录扫描型则返回扫描结果清单。
//
// 返回值可能为空串（扫描失败且无静态描述），此时调用方应跳过该条。
func renderShellHint(row *db.Tool, key, desc string) string {
	spec, hasDir := parseShellHintSpec(row.Exec)
	if !hasDir {
		return "- " + key + ": " + desc
	}
	cached, ok := toolHintCacheGet(spec)
	if !ok {
		cached = scanAndRender(spec, desc)
		toolHintCachePut(spec, cached)
	}
	return cached
}

// parseShellHintSpec 解析 exec 字段，判断是否为目录扫描型提示。
func parseShellHintSpec(execRaw json.RawMessage) (shellHintSpec, bool) {
	var spec shellHintSpec
	if len(execRaw) == 0 {
		return spec, false
	}
	if err := json.Unmarshal(execRaw, &spec); err != nil {
		return shellHintSpec{}, false
	}
	if strings.TrimSpace(spec.Dir) == "" {
		return spec, false
	}
	return spec, true
}

// toolHintCacheGet 取缓存。key 由扫描参数决定，参数变了缓存自然失效。
func toolHintCacheGet(spec shellHintSpec) (string, bool) {
	key := toolHintCacheKey(spec)
	toolHintCache.mu.Lock()
	defer toolHintCache.mu.Unlock()
	e, ok := toolHintCache.m[key]
	if !ok {
		return "", false
	}
	if time.Since(e.at) > toolHintCacheTTL {
		return "", false
	}
	return e.text, true
}

func toolHintCachePut(spec shellHintSpec, text string) {
	key := toolHintCacheKey(spec)
	toolHintCache.mu.Lock()
	defer toolHintCache.mu.Unlock()
	toolHintCache.m[key] = toolHintCacheEntry{text: text, at: time.Now()}
}

func toolHintCacheKey(spec shellHintSpec) string {
	return spec.Dir + "|" + itoa(spec.MaxDepth) + "|" + itoa(spec.MaxEntries) + "|" + spec.Platform + "|" + strings.Join(spec.Include, ",")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// scanAndRender 扫描目录并渲染成清单文本。
func scanAndRender(spec shellHintSpec, fallbackDesc string) string {
	root := filepath.Clean(spec.Dir)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		if fallbackDesc != "" {
			return "- 工具目录: " + root + " (不可访问)"
		}
		return "- 工具目录: " + root + " (不可访问)"
	}

	depth := spec.MaxDepth
	if depth <= 0 {
		depth = toolHintDefaultDepth
	}
	if depth > toolHintMaxDepthHard {
		depth = toolHintMaxDepthHard
	}
	// maxEntries 是单组最多列出的文件名数量（语义：每个目录组的名额），
	// 避免某个超大分组（dotnet/SharpCollection 有 200+ 个 exe）撑爆提示。
	maxEntries := spec.MaxEntries
	if maxEntries <= 0 {
		maxEntries = toolHintMaxPerGroup
	}
	if maxEntries > toolHintMaxEntriesCap {
		maxEntries = toolHintMaxEntriesCap
	}
	perGroup := maxEntries
	if spec.MaxPerGroup > 0 {
		perGroup = spec.MaxPerGroup
	}
	platform := spec.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	allowed := toolExecExtensions(platform, spec.Include)

	files, total := scanToolFiles(root, depth, allowed)
	// 注意：不按总数截断文件列表。按总数截断会让排在后面的目录整组消失，
	// 模型就看不到那些工具了。真正的收敛靠每组上限（perGroup），
	// 它保证每个目录都被提及，只是名字多时折叠成计数。
	if total == 0 {
		return "- 工具目录: " + root + " (未发现可执行文件)"
	}
	truncated := total > len(files)

	var b strings.Builder
	b.WriteString("- 工具目录 ")
	b.WriteString(root)
	b.WriteString("  (以下命令可直接调用，共 ")
	b.WriteString(itoa(total))
	b.WriteString(" 个)")
	if truncated {
		b.WriteString("，仅列出前 ")
		b.WriteString(itoa(len(files)))
		b.WriteString(" 个")
	}
	b.WriteString(":\n")
	// 工具名叫 Bash，但 Windows 上底层是 PowerShell。不点破的话模型会按
	// bash 语法写命令（ls -la / && 管道）而全部失败。
	b.WriteString("    [命令语法: ")
	if runtime.GOOS == "windows" {
		b.WriteString("Windows PowerShell（非 bash，禁用 ls -la/&& 等 bash 语法）")
	} else {
		b.WriteString("bash -c")
	}
	b.WriteString("]\n")

	// 按一级子目录分组，组内按路径排序。
	groups := map[string][]string{}
	var order []string
	for _, f := range files {
		g := f.group
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], f.tail)
	}
	sort.Strings(order)
	for _, g := range order {
		items := groups[g]
		sort.Strings(items)
		// 一个组里可能有几百个文件（dotnet/SharpCollection 就是典型）。
		// 全列出来会让 Bash 描述膨胀到几千 token，而它每轮 LLM 都带。
		// 超出上限时只列前 N 个并给出总数，模型需要细节时可自行 ls。
		shown := items
		rest := 0
		if len(shown) > perGroup {
			rest = len(shown) - perGroup
			shown = shown[:perGroup]
		}
		if g == "" {
			b.WriteString("    根目录: ")
		} else {
			b.WriteString("    " + g + "/: ")
		}
		b.WriteString(strings.Join(shown, ", "))
		if rest > 0 {
			b.WriteString(" 等共 " + itoa(len(items)) + " 个")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolExecExtensions 返回该平台上视为"可执行工具"的文件扩展名集合。
func toolExecExtensions(platform string, extra []string) map[string]bool {
	m := map[string]bool{}
	switch platform {
	case "windows":
		for _, e := range []string{".exe", ".bat", ".cmd", ".com", ".ps1"} {
			m[e] = true
		}
	case "linux":
		for _, e := range []string{".sh", ".py"} {
			m[e] = true
		}
	default:
		for _, e := range []string{".exe", ".bat", ".cmd", ".com", ".ps1", ".sh", ".py"} {
			m[e] = true
		}
	}
	for _, e := range extra {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		if e != "" {
			m[e] = true
		}
	}
	return m
}

// scanToolFiles 递归扫描 root，返回可执行文件列表与总数（total 不受
// maxEntries 截断影响，用于告诉模型真实规模）。
//
// 深度语义：根目录算第 0 层，maxDepth=N 表示最多向下 N 层。即
// maxDepth=3 时可见 a/、a/b/、a/b/c/ 下的文件，但 a/b/c/d/ 不可见。
// 目录与文件用同一套层级判定，避免"目录被截断但深处的文件仍被收录"。
func scanToolFiles(root string, maxDepth int, allowed map[string]bool) ([]toolHintFile, int) {
	var files []toolHintFile
	total := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// 单个目录读不到就跳过，不影响整体扫描。
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil // 根本身
		}
		// depth = 该条目所在目录的层数（根为 0）。目录条目自身的层数即其父层数。
		depth := strings.Count(rel, "/")
		if d.IsDir() {
			// 该目录位于第 depth 层；进入它之后是第 depth+1 层。
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			// 跳过隐藏目录（.git/.nuclei-config 等）、依赖树与测试目录。
			if strings.HasPrefix(d.Name(), ".") || toolHintSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// 文件位于其父目录层；父层数 >= maxDepth 即超出可见范围。
		parentDepth := depth - 1
		if parentDepth >= maxDepth {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !allowed[ext] {
			return nil
		}
		// 包初始化文件与测试文件不是工具入口。
		if toolHintSkipFiles[d.Name()] {
			return nil
		}
		// test_*.py / *_test.py 这类命名同样排除。
		base := d.Name()
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			return nil
		}
		// 脚本类工具只认"入口"：入口脚本（sqlmap.py、commix.py、linpeas.sh）
		// 才算工具，其内部的库模块（lib/、core/、utils/、extra/ 下的脚本）
		// 不是可调用入口，列出来只会淹没提示（sqlmap 一个就有 180+ 个）。
		//
		// 规则：
		//   depth 1（sqlmap-1.10/sqlmap.py）→ 直接是入口；
		//   depth 2（privesc/PEASS-ng/linpeas.sh）→ 父目录须是叶子，否则它
		//         属于一棵模块树（sqlmap-1.10/extra/beep/beep.py 就是这种）；
		//   depth ≥3 → 一律视为库模块。
		if ext == ".py" || ext == ".sh" {
			if depth >= 3 {
				return nil
			}
			if depth == 2 {
				parent := filepath.Dir(path)
				entries, derr := os.ReadDir(parent)
				if derr == nil {
					for _, e := range entries {
						if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
							return nil
						}
					}
				}
			}
		}
		total++
		f := toolHintFile{rel: rel}
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			f.group = rel[:i]
			f.tail = rel[i+1:]
		} else {
			f.group = ""
			f.tail = rel
		}
		files = append(files, f)
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		if files[i].group != files[j].group {
			return files[i].group < files[j].group
		}
		return files[i].rel < files[j].rel
	})
	return files, total
}

// validateShellHintDir 校验目录扫描型 shell 提示的目录：必须是绝对路径、
// 必须存在且是目录。不允许相对路径（工作目录随任务变化，会导致同一份
// 配置在不同任务里指向不同位置）。传入的是 exec 原始 JSON 与工具类型；
// 非 shell 类型不校验，shell 类型但 exec 为空则放行（静态提示）。
func validateShellHintDir(kind string, execRaw json.RawMessage) error {
	if kind != "shell" || len(execRaw) == 0 {
		return nil
	}
	var probe struct {
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal(execRaw, &probe); err != nil {
		// 非法 JSON 不再静默放行：否则一份畸形配置会绕过全部校验，
		// 直到运行期才以难懂的方式失败。
		return errShellHintDir("exec 不是合法的 JSON: " + err.Error())
	}
	d := strings.TrimSpace(probe.Dir)
	if d == "" {
		return nil
	}
	if !filepath.IsAbs(d) {
		return errShellHintDir("工具目录必须是绝对路径")
	}
	info, err := os.Stat(d)
	if err != nil {
		return errShellHintDir("工具目录不可访问: " + err.Error())
	}
	if !info.IsDir() {
		return errShellHintDir("工具目录不是目录")
	}
	return nil
}

// errShellHintDir 是一个带中文说明的错误。
type errShellHintDir string

func (e errShellHintDir) Error() string { return string(e) }

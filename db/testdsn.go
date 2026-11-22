// testdsn.go：给测试用的库名护栏。
//
// 为什么需要它：`.workbuddy-run.ps1` 会把 ARTEX_PG_DSN 设成生产库 artex_audit，
// 这个环境变量会留在当前 shell 里。同一个 shell 里跑 `go test`，那些会写库的
// 集成测试就把 fixture 直接灌进了生产库 —— 2026-11-22 清出来的 2899 条
// （worker=test 的 bound evidence、`超长漏洞名称xxx 的摘要`、`fi 已建会话 finding`、
// task_id 为空的 host test/s）就是那么来的。测试库和生产库同名只是一字之差，
// 靠人记不住的，所以让测试自己判断该不该跑。
package db

import (
	"net/url"
	"os"
	"strings"
)

// TestEnvAllowNonTestDB 是可以绕过护栏的开关。
//
// 默认拒绝，只有明确设成 "1" 才放行：CI 或临时评测环境偶尔需要用非常规库名，
// 但那种场合一定有人手工配过环境，比"忘了设 DSN"安全得多。
const TestEnvAllowNonTestDB = "ARTEX_TEST_ALLOW_NON_TEST_DB"

// CheckTestDSN 判断 dsn 指向的数据库能不能被测试当作一次性库使用。
//
// 返回 nil 表示可以跑；返回 error 时调用方应打印原因并中止测试进程，
// 而不是降级成 skip —— skip 会让人以为"测试都过了"，而实际数据已经脏了。
//
// 规则：库名以 _test 结尾（artex_test）或直接叫 test 才算测试库。
// 这条约定是有意的，宁可逼人多写一个 _test 后缀，也不要在生产库里找 fixture。
func CheckTestDSN(dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		// 没配库：所有需要 PG 的用例自己会 skip，这里不拦。
		return nil
	}
	if os.Getenv(TestEnvAllowNonTestDB) == "1" {
		return nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return &testDSNError{reason: "无法解析连接串：" + err.Error()}
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return &testDSNError{reason: "连接串里没有库名"}
	}
	if name == "test" || strings.HasSuffix(name, "_test") {
		return nil
	}
	return &testDSNError{dbname: name}
}

type testDSNError struct {
	dbname string
	reason string
}

func (e *testDSNError) Error() string {
	var head string
	if e.dbname != "" {
		head = "拒绝在疑似生产库 " + e.dbname + " 上跑测试（库名不以 _test 结尾）"
	} else {
		head = e.reason
	}
	// 两种失败原因后面跟同一段指引：光说"不许跑"不解决问题，
	// 下一个人只会把这个检查删掉。
	return "测试库护栏：" + head + "。测试会写入并清理数据，请指向专用测试库" +
		"（例如 ARTEX_PG_DSN=postgres://.../artex_test?sslmode=disable）；" +
		"确实要用非常规库名时设 " + TestEnvAllowNonTestDB + "=1。"
}

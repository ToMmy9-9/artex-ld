package db

import (
	"strings"
	"testing"
)

// TestDSNGate 守住护栏本身的判断。
//
// 这个护栏只有在"该拦的时候真拦住"才有意义 —— 判断逻辑写宽了（比如只检查
// 库名非空）等于没加，而它的失效方式很隐蔽：测试照常通过，只是生产库又被
// 塞了一批 fixture。
func TestDSNGate(t *testing.T) {
	cases := []struct {
		name    string
		dsn     string
		allow   bool
		wantErr bool
	}{
		// 这些是正常用法，必须放行。
		{name: "专用测试库", dsn: "postgres://artex:pw@127.0.0.1:5432/artex_test?sslmode=disable"},
		{name: "库名就叫 test", dsn: "postgres://artex:pw@127.0.0.1:5432/test?sslmode=disable"},
		{name: "带 schema 参数", dsn: "postgres://artex:pw@127.0.0.1:5432/artex_test?search_path=public"},
		{name: "没配库", dsn: ""},

		// 这些是会写脏生产库的典型场景，必须拦住。
		{name: "生产库", dsn: "postgres://artex:pw@127.0.0.1:5432/artex_audit?sslmode=disable", wantErr: true},
		{name: "验证演示库", dsn: "postgres://artex:pw@127.0.0.1:5432/artex_ui?sslmode=disable", wantErr: true},
		{name: "test 只在前缀", dsn: "postgres://artex:pw@127.0.0.1:5432/test_prod?sslmode=disable", wantErr: true},
		{name: "不带 _test 后缀的普通库", dsn: "postgres://artex:pw@127.0.0.1:5432/artex?sslmode=disable", wantErr: true},
		{name: "连接串没有库名", dsn: "postgres://artex:pw@127.0.0.1:5432/", wantErr: true},

		// 逃生口：显式开关才放行，且只影响本次进程。
		{name: "显式放行生产库", dsn: "postgres://artex:pw@127.0.0.1:5432/artex_audit", allow: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.allow {
				t.Setenv(TestEnvAllowNonTestDB, "1")
			} else {
				t.Setenv(TestEnvAllowNonTestDB, "")
			}
			err := CheckTestDSN(tc.dsn)
			if tc.wantErr && err == nil {
				t.Fatalf("%q 应被拦住，实际放行", tc.dsn)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%q 应放行，实际拦住：%v", tc.dsn, err)
			}
			if err != nil {
				// 提示里必须说清怎么改，否则下一个人只会把护栏删掉。
				msg := err.Error()
				for _, need := range []string{"_test", TestEnvAllowNonTestDB} {
					if !strings.Contains(msg, need) {
						t.Fatalf("拦截提示应提到 %q，实际：%s", need, msg)
					}
				}
			}
		})
	}
}

// TestDSNGateRejectsProductionDatabaseByDefault 单独钉住最常见的那个误用：
// `.workbuddy-run.ps1` 把 ARTEX_PG_DSN 设成 artex_audit，同一个 shell 里
// 接着跑 go test 就会命中。这条用例存在的意义是让那个场景永远有测试盯着。
func TestDSNGateRejectsProductionDatabaseByDefault(t *testing.T) {
	t.Setenv(TestEnvAllowNonTestDB, "")
	prodDSN := "postgres://artex:01EDTUB7HLyFQ3ii@127.0.0.1:5432/artex_audit?sslmode=disable"
	err := CheckTestDSN(prodDSN)
	if err == nil {
		t.Fatal("生产库 DSN 必须被拦住")
	}
	if !strings.Contains(err.Error(), "artex_audit") {
		t.Fatalf("提示里应点名是哪个库，实际：%s", err.Error())
	}
}

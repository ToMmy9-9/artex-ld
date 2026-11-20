package tool

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestConsoleUTF8OnWindows 验证 PowerShell 工具输出不再以 GBK 字节返回。
// 真实任务里 Invoke-WebRequest 的中文错误经 GBK → ToValidUTF8 后变成乱码，
// 模型无法据此判断工具失败原因；withConsoleUTF8 在命令开头切换控制台编码。
func TestConsoleUTF8OnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell console encoding is Windows-only")
	}
	shell, flags := shellCmd()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, append(flags, withConsoleUTF8("Write-Output '中文测试通过'"))...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "中文测试通过") {
		t.Fatalf("UTF-8 console output not preserved: %q", out)
	}
}

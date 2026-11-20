// fsync_other.go：目录 fsync 的 POSIX 实现。
//
// 在 POSIX 上，目录项本身也要 fsync 才能真正落盘——否则掉电后文件内容可能在，
// 但目录里查不到这个名字。Windows 不支持对目录句柄 fsync（会返回
// "Access is denied"），那一边由 fsync_windows.go 提供空实现。

//go:build !windows

package evidence

import "os"

// syncDir flushes a directory's entries so a rename survives a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

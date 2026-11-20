// fsync_windows.go：目录 fsync 的 Windows 实现。
//
// Windows 不允许对目录句柄调用 FlushFileBuffers / fsync —— Open(dir) 之后的
// Sync() 稳定返回 "Access is denied"。文件的持久化由 writeBody 里对文件本身的
// f.Sync() 保证；目录项本身在 NTFS 上随文件句柄的 flush 一起提交，所以这里
// 直接返回 nil，而不是让每一次证据写入都因为一个不支持的 syscall 而失败。

//go:build windows

package evidence

// syncDir is a no-op on Windows: directory handles cannot be fsynced there.
func syncDir(string) error { return nil }

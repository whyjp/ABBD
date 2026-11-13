package model

import (
	"syscall"
	"unsafe"
)

// osLocale is the user's default locale name, such as ko-KR, from
// GetUserDefaultLocaleName, or "" when the call fails.
func osLocale() string {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	if proc.Find() != nil {
		return ""
	}
	buf := make([]uint16, 85)
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// file: internal/sysinfo/uptime_darwin.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-a3b4c5d6e7f8

//go:build darwin

package sysinfo

import (
	"syscall"
	"time"
	"unsafe"
)

// kern.boottime is a struct timeval: {sec int64, usec int32}
type timeval struct {
	Sec  int64
	Usec int32
}

func getSystemUptimePlatform() float64 {
	mib := []int32{1 /* CTL_KERN */, 21 /* KERN_BOOTTIME */}
	var tv timeval
	n := unsafe.Sizeof(tv)
	_, _, err := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&tv)),
		uintptr(unsafe.Pointer(&n)),
		0, 0,
	)
	if err != 0 {
		return 0
	}
	boot := time.Unix(tv.Sec, int64(tv.Usec)*1000)
	return time.Since(boot).Seconds()
}

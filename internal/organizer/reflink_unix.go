// file: internal/organizer/reflink_unix.go
// version: 1.0.0
// guid: 6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c

//go:build darwin || linux

package organizer

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// reflinkFilePlatform creates a CoW reflink on macOS/Linux
func (o *Organizer) reflinkFilePlatform(sourcePath, targetPath string) error {
	// Open source file
	srcFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Try platform-specific reflink
	srcFd := int(srcFile.Fd())
	dstFd := int(dstFile.Fd())

	// macOS: APFS clonefile
	// Linux: FICLONE ioctl
	var ret uintptr
	var errno syscall.Errno

	// Try Linux FICLONE first (most common)
	const FICLONE = 0x40049409
	ret, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(dstFd), FICLONE, uintptr(srcFd))
	if errno == 0 {
		return nil
	}

	// Try macOS clonefile
	const APFS_CLONE = 0xC0084A6D
	ret, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(dstFd), APFS_CLONE, uintptr(unsafe.Pointer(&srcFd)))
	if errno == 0 {
		return nil
	}

	// If both fail, return error
	if ret != 0 || errno != 0 {
		return fmt.Errorf("reflink not supported on this filesystem (errno: %v)", errno)
	}

	return nil
}

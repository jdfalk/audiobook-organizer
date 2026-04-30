// file: internal/server/deluge_import_unix.go
// version: 1.0.0
// guid: a9c2e5f7-1d3b-4087-b8a6-5f2c0e9d1b74

//go:build darwin || linux

package server

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// reflinkCopyOS attempts a reflink copy using OS-specific syscalls.
//
// On Linux it tries the FICLONE ioctl (requires btrfs, XFS, OCFS2, or ZFS).
// On macOS it tries the APFS clonefile ioctl.
// Returns an error if the filesystem does not support reflinks or the call fails;
// the caller should then fall back to io.Copy.
func reflinkCopyOS(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()

	srcFd := int(in.Fd())
	dstFd := int(out.Fd())

	// Linux: FICLONE ioctl (copy-on-write clone).
	const FICLONE = 0x40049409
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(dstFd), FICLONE, uintptr(srcFd))
	if errno == 0 {
		return nil
	}

	// macOS: APFS clonefile ioctl.
	const APFS_CLONE = 0xC0084A6D
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(dstFd), APFS_CLONE, uintptr(unsafe.Pointer(&srcFd)))
	if errno == 0 {
		return nil
	}

	// Both reflink attempts failed — remove the empty dest file so ioCopy can create a fresh one.
	out.Close()
	_ = os.Remove(dest)
	return fmt.Errorf("reflink not supported on this filesystem (errno: %v)", errno)
}

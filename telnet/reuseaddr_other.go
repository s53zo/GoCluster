//go:build !windows

package telnet

import "syscall"

// setReuseAddr enables SO_REUSEADDR on non-Windows sockets.
func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}

//go:build windows

package main

func setUmask() {
	// Windows has no umask; file permissions are handled via explicit Chmod.
}

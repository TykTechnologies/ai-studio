package ociplugins

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

const (
	// maxKeyFileSize bounds a public key file. A key ID comes from a plugin
	// command, so it must not be able to make the process read a device or
	// a huge file.
	maxKeyFileSize = 64 << 10
	// maxTrustedRootFileSize bounds a Sigstore trusted_root.json.
	maxTrustedRootFileSize = 4 << 20
)

// readRegularFile reads the regular file at path (symlinks are followed),
// refusing devices, FIFOs and other non-regular files and anything larger
// than limit bytes. The file is opened non-blocking and checked again after
// opening, so a FIFO swapped in after the first check cannot block the open
// or the read.
func readRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err = f.Stat(); err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	return data, nil
}

// ReadTrustedRootFile reads a Sigstore trusted-root override
// (OCIConfig.TrustedRootFile), bounded like ValidateTrustedRootFile.
func ReadTrustedRootFile(path string) ([]byte, error) {
	data, err := readRegularFile(path, maxTrustedRootFileSize)
	if err != nil {
		return nil, fmt.Errorf("trusted root file is not readable: %w", err)
	}
	return data, nil
}

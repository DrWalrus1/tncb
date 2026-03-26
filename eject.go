package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ejectDisc ejects the disc whose BDMV root is at bdmvRoot.
// On macOS it uses `diskutil eject`; on Linux it uses `eject`.
func ejectDisc(bdmvRoot string) error {
	// Derive the mount point: bdmvRoot is …/BDMV, so the volume is one level up.
	mountPoint := filepath.Dir(bdmvRoot)

	switch runtime.GOOS {
	case "darwin":
		return runCmd("diskutil", "eject", mountPoint)
	case "linux":
		// Try ejecting the mount point directly; fall back to /dev/sr0.
		if err := runCmd("eject", mountPoint); err != nil {
			return runCmd("eject", "/dev/sr0")
		}
		return nil
	default:
		return fmt.Errorf("eject not supported on %s", runtime.GOOS)
	}
}

func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, out)
	}
	return nil
}

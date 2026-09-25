//go:build windows

package remove

import (
	"fmt"

	"github.com/lamnguyenx/gtu/v2026/pkg/fs"
)

// TrashCommand is not supported on Windows, which has no POSIX shell to
// evaluate the command in.
func TrashCommand(command string) func(dir, item fs.Item) error {
	return func(dir, item fs.Item) error {
		return fmt.Errorf("trash command is not supported on Windows")
	}
}

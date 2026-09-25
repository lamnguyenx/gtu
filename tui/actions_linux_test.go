//go:build linux

package tui

import (
	"bytes"
	"testing"

	"github.com/lamnguyenx/gtu/v2026/internal/testapp"
	"github.com/lamnguyenx/gtu/v2026/pkg/device"
	"github.com/stretchr/testify/assert"
)

func TestShowDevicesWithError(t *testing.T) {
	app, simScreen := testapp.CreateTestAppWithSimScreen(50, 50)
	defer simScreen.Fini()

	getter := device.LinuxDevicesInfoGetter{MountsPath: "/xyzxyz"}

	ui := CreateUI(app, simScreen, &bytes.Buffer{}, false, false, false, false)
	err := ui.ListDevices(getter)

	assert.Contains(t, err.Error(), "no such file")
}

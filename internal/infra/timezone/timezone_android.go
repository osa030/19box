//go:build android

// Package timezone provides platform-specific timezone initialization.
package timezone

import (
	"os/exec"
	"strings"
	"time"
	_ "time/tzdata" // Embed timezone database for Android
)

// Init initializes time.Local to the device's timezone on Android.
// On Android, time.Local defaults to UTC. This function attempts to detect
// the actual system timezone and set time.Local accordingly.
// If detection fails, time.Local remains UTC.
func Init() {
	tzName := detectTimezone()
	if tzName == "" {
		return
	}

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return
	}

	time.Local = loc
}

// detectTimezone attempts to detect the Android system timezone
// by executing getprop persist.sys.timezone command.
func detectTimezone() string {
	// Try getprop command to get Android system timezone
	if output, err := exec.Command("getprop", "persist.sys.timezone").Output(); err == nil {
		tzName := strings.TrimSpace(string(output))
		if tzName != "" {
			return tzName
		}
	}

	return ""
}

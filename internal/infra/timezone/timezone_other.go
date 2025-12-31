//go:build !android

// Package timezone provides platform-specific timezone initialization.
package timezone

// Init is a no-op on non-Android platforms.
// On these platforms, Go's time package correctly initializes time.Local
// from the system timezone.
func Init() {
	// Nothing to do on non-Android platforms
}

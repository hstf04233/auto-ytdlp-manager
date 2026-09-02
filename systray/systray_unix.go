//go:build !windows

package systray

func StartSystray(SystrayIconContent []byte) (bool, error) {
	// Don't do anything on unix systems...
	return false, nil
}

func RemoveSystray() {
}

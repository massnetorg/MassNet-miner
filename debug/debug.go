package debug

var (
	devMode = true
)

func DevMode() bool {
	return devMode
}

func EnableDevMode() {
	devMode = true
}

func DisableDevMode() {
	devMode = false
}

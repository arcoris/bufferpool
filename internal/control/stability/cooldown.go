package stability

// CoolingDown reports whether currentWindow is still inside a cooldown period.
func CoolingDown(lastChangeWindow, currentWindow, cooldownWindows uint64) bool {
	if cooldownWindows == 0 || currentWindow < lastChangeWindow {
		return false
	}
	return currentWindow-lastChangeWindow < cooldownWindows
}

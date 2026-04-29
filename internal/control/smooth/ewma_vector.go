package smooth

// NamedEWMA associates an EWMA with a stable diagnostic name.
type NamedEWMA struct {
	// Name identifies the smoothed signal.
	Name string

	// EWMA contains the current smoothed value for Name.
	EWMA EWMA
}

package flux

// State represents the current state of a Capacitor.
type State int32

const (
	// StateLoading indicates the Capacitor is initializing and has not yet
	// processed any configuration.
	StateLoading State = iota

	// StateHealthy indicates the Capacitor has a valid configuration applied.
	StateHealthy

	// StateDegraded indicates the last configuration change failed validation
	// or application. The previous valid configuration remains active.
	StateDegraded

	// StateEmpty indicates the initial configuration load failed and no valid
	// configuration has ever been obtained. The Capacitor continues watching
	// for valid updates.
	StateEmpty
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateLoading:
		return "loading"
	case StateHealthy:
		return "healthy"
	case StateDegraded:
		return "degraded"
	case StateEmpty:
		return "empty"
	default:
		return "unknown"
	}
}

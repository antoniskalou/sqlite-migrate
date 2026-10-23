package migrate

// Direction specifies whether migrations are being applied or rolled back.
type Direction uint8

const (
	// Up applies migrations in ascending version order.
	Up Direction = iota

	// Down rolls back migrations in descending version order.
	Down
)

// String returns the string representation of the given Direction.
func (d Direction) String() string {
	switch d {
	case Up:
		return "up"
	case Down:
		return "down"
	default:
		panic("unreachable")
	}
}

func (d Direction) delta() int {
	switch d {
	case Up:
		return 1
	case Down:
		return -1
	default:
		panic("unreachable")
	}
}

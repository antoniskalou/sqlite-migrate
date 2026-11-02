package migrate

import "fmt"

// Direction specifies whether migrations are being applied or rolled back.
type Direction uint8

const (
	// Up applies migrations in ascending version order.
	Up Direction = iota + 1

	// Down rolls back migrations in descending version order.
	Down
)

const errInvalidDirection = "invalid direction: %d"

// String returns the string representation of the given Direction.
func (d Direction) String() string {
	switch d {
	case Up:
		return "up"
	case Down:
		return "down"
	default:
		return fmt.Sprintf("Direction(%d)", d)
	}
}

// MarshalText implements encoding.TextMarshaler.
//
// It returns the string representation of the direction.
func (d Direction) MarshalText() ([]byte, error) {
	switch d {
	case Up, Down:
		return []byte(d.String()), nil
	default:
		return nil, fmt.Errorf(errInvalidDirection, d)
	}
}

func (d Direction) delta() int {
	switch d {
	case Up:
		return 1
	case Down:
		return -1
	default:
		panic(fmt.Sprintf(errInvalidDirection, d))
	}
}

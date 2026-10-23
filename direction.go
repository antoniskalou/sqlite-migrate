package migrate

type Direction uint8

const (
	Up Direction = iota
	Down
)

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

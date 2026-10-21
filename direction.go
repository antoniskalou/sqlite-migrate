package migrate

type direction uint8

const (
	up direction = iota
	down
)

func (d direction) String() string {
	switch d {
	case up:
		return "up"
	case down:
		return "down"
	default:
		panic("unreachable")
	}
}

func (d direction) Delta() int {
	switch d {
	case up:
		return 1
	case down:
		return -1
	default:
		panic("unreachable")
	}
}

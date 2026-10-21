package migrate

type EventKind string

const (
	EventSkip  EventKind = "skip"
	EventApply EventKind = "apply"
	EventSQL   EventKind = "sql"
	EventDone  EventKind = "done"
)

type Event struct {
	Kind      EventKind
	File      string
	Direction string
	Version   int
	SQL       string
}

package migrate

// EventKind identifies the kind of event emitted during a migration operation.
type EventKind string

const (
	// EventSkip indicates that a migration was skipped because it was already
	// applied or was not eligible to run.
	EventSkip EventKind = "skip"

	// EventApply indicates that a migration is about to be applied.
	EventApply EventKind = "apply"

	// EventSQL indicates that a migration's SQL is being executed.
	EventSQL EventKind = "sql"

	// EventDone indicates that the migration operation has completed.
	EventDone EventKind = "done"
)

// Event describes an event emitted during a migration operation.
type Event struct {
	// Kind identifies the event type.
	Kind EventKind

	// File is the absolute path of the migration file associated with the event.
	File string

	// Direction indicates whether migrations are being applied or rolled back.
	Direction Direction

	// Version is the database version (user_version) associated with the event.
	Version int

	// SQL contains the SQL read from a migration file.
	SQL string
}

package tasks

// Task — Task struct + state machine + participants + completion token (docs/15)

type TaskID int64

type Task struct {
	ID TaskID
}

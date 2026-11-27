package route

import "slices"

type Task string

const (
	Full      Task = ""
	Measure   Task = "measure"
	Logs      Task = "logs"
	Traces    Task = "traces"
	Incident  Task = "incident"
	Inventory Task = "inventory"
	Explain   Task = "explain"
)

var Tasks = []Task{Measure, Logs, Traces, Incident, Inventory, Explain}

func Parse(raw string) (Task, bool) {
	t := Task(raw)
	return t, t == Full || slices.Contains(Tasks, t)
}

func (t Task) Name() string {
	if t == Full {
		return "full"
	}
	return string(t)
}

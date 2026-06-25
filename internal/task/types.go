package task

// DateFormat is the date format used by Taskwarrior in all date fields
// (e.g. Entry, Due, Start). All date parsing and formatting in this
// package uses this constant.
const DateFormat = "20060102T150405Z"

// Task represents a taskwarrior task as returned by `task export`.
type Annotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type Task struct {
	ID          int          `json:"id"`
	UUID        string       `json:"uuid"`
	Description string       `json:"description"`
	Project     string       `json:"project"`
	Tags        []string     `json:"tags"`
	Status      string       `json:"status"`
	Start       string       `json:"start"`
	Entry       string       `json:"entry"`
	Due         string       `json:"due"`
	Priority    string       `json:"priority"`
	Recur       string       `json:"recur"`
	Parent      string       `json:"parent"`
	RType       string       `json:"rtype"`
	Urgency     float64      `json:"urgency"`
	Annotations []Annotation `json:"annotations"`
}

// RunResult contains the captured output from a task command invocation.
type RunResult struct {
	Args   []string
	Stdout string
	Stderr string
}

// CompletionSources contains values used for Taskwarrior shell completion.
type CompletionSources struct {
	Commands []string
	Columns  []string
	Projects []string
	Tags     []string
	IDs      []string
	UUIDs    []string
	UDAs     []string
}

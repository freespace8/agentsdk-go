package workflow

// Workflow models a high-level orchestrator abstraction.
type Workflow interface {
	Run() error
}

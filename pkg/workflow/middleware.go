package workflow

// Middleware allows step interception.
type Middleware interface {
	BeforeStep(name string) error
	AfterStep(name string) error
}

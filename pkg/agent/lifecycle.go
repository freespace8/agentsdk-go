package agent

// LifecycleHook allows pluggable start/stop hooks around agent execution.
type LifecycleHook interface {
	OnStart(ctx RunContext) error
	OnStop(ctx RunContext) error
}

// LifecycleHooks is a helper slice that executes hooks sequentially.
type LifecycleHooks []LifecycleHook

// RunStart executes all start hooks until one fails.
func (hooks LifecycleHooks) RunStart(ctx RunContext) error {
	for _, hook := range hooks {
		if err := hook.OnStart(ctx); err != nil {
			return err
		}
	}
	return nil
}

// RunStop executes all stop hooks until one fails.
func (hooks LifecycleHooks) RunStop(ctx RunContext) error {
	for _, hook := range hooks {
		if err := hook.OnStop(ctx); err != nil {
			return err
		}
	}
	return nil
}

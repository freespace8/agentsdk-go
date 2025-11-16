package toolbuiltin

// GlobTool looks up files via glob patterns.
type GlobTool struct{}

func (g GlobTool) Name() string { return "glob" }

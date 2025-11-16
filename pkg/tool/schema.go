package tool

// JSONSchema captures the subset of JSON Schema we require for tool validation.
type JSONSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required"`
}

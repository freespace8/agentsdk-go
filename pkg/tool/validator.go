package tool

import (
	"encoding/json"
	"fmt"
	"math"
)

// Validator validates tool parameters before execution.
type Validator interface {
	Validate(params map[string]interface{}, schema *JSONSchema) error
}

// DefaultValidator implements a minimal JSON Schema validator covering
// required fields and primitive type checks.
type DefaultValidator struct{}

// Validate ensures that params satisfy the provided schema.
func (DefaultValidator) Validate(params map[string]interface{}, schema *JSONSchema) error {
	if schema == nil {
		return nil
	}

	if params == nil {
		params = map[string]interface{}{}
	}

	for _, field := range schema.Required {
		if _, exists := params[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	if len(schema.Properties) == 0 {
		return nil
	}

	for key, value := range params {
		propDef, ok := schema.Properties[key]
		if !ok {
			continue
		}

		expectedType := extractExpectedType(propDef)
		if expectedType == "" {
			continue
		}

		if err := validateType(value, expectedType); err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
	}

	return nil
}

func extractExpectedType(definition interface{}) string {
	switch def := definition.(type) {
	case map[string]interface{}:
		if value, ok := def["type"].(string); ok {
			return value
		}
	case *JSONSchema:
		return def.Type
	}
	return ""
}

func validateType(value interface{}, expected string) error {
	switch expected {
	case "string":
		if _, ok := value.(string); ok {
			return nil
		}
	case "number":
		if isNumber(value) {
			return nil
		}
	case "integer":
		if isInteger(value) {
			return nil
		}
	case "boolean":
		if _, ok := value.(bool); ok {
			return nil
		}
	case "object":
		if value == nil {
			break
		}
		if _, ok := value.(map[string]interface{}); ok {
			return nil
		}
	case "array":
		if _, ok := value.([]interface{}); ok {
			return nil
		}
	case "null":
		if value == nil {
			return nil
		}
	default:
		return fmt.Errorf("unsupported schema type %q", expected)
	}
	return fmt.Errorf("expected %s but got %T", expected, value)
}

func isNumber(value interface{}) bool {
	switch v := value.(type) {
	case float32, float64:
		return true
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case json.Number:
		_, err := v.Float64()
		return err == nil
	}
	return false
}

func isInteger(value interface{}) bool {
	switch v := value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return math.Trunc(float64(v)) == float64(v)
	case float64:
		return math.Trunc(v) == v
	case json.Number:
		_, err := v.Int64()
		return err == nil
	}
	return false
}

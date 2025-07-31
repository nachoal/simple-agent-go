package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Generator converts Go structs to JSON schemas
type Generator struct {
	// Definitions stores schema definitions for reuse
	Definitions map[string]interface{}
}

// NewGenerator creates a new schema generator
func NewGenerator() *Generator {
	return &Generator{
		Definitions: make(map[string]interface{}),
	}
}

// Generate creates a JSON schema from a Go type
func (g *Generator) Generate(v interface{}) (map[string]interface{}, error) {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", t.Kind())
	}

	return g.generateObject(t), nil
}

// GenerateFunctionSchema creates an OpenAI-compatible function schema
func (g *Generator) GenerateFunctionSchema(name, description string, params interface{}) map[string]interface{} {
	schema, _ := g.Generate(params)
	
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        name,
			"description": description,
			"parameters":  schema,
		},
	}
}

func (g *Generator) generateObject(t reflect.Type) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}

	properties := schema["properties"].(map[string]interface{})
	required := []string{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		
		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		fieldName := getFieldName(field, jsonTag)
		if fieldName == "" {
			continue
		}

		// Check if field is required
		schemaTag := field.Tag.Get("schema")
		if strings.Contains(schemaTag, "required") || !strings.Contains(jsonTag, "omitempty") {
			required = append(required, fieldName)
		}

		// Generate field schema
		fieldSchema := g.generateFieldSchema(field)
		
		// Add description if present
		if desc := field.Tag.Get("description"); desc != "" {
			fieldSchema["description"] = desc
		}

		// Parse schema tag for additional constraints
		g.parseSchemaTag(schemaTag, fieldSchema)

		properties[fieldName] = fieldSchema
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func (g *Generator) generateFieldSchema(field reflect.StructField) map[string]interface{} {
	schema := make(map[string]interface{})

	switch field.Type.Kind() {
	case reflect.String:
		schema["type"] = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		schema["type"] = "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
		schema["minimum"] = 0
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Slice:
		schema["type"] = "array"
		if field.Type.Elem().Kind() == reflect.Struct {
			schema["items"] = g.generateObject(field.Type.Elem())
		} else {
			schema["items"] = g.generateFieldSchema(reflect.StructField{Type: field.Type.Elem()})
		}
	case reflect.Map:
		schema["type"] = "object"
		if field.Type.Elem().Kind() != reflect.Interface {
			schema["additionalProperties"] = g.generateFieldSchema(reflect.StructField{Type: field.Type.Elem()})
		}
	case reflect.Struct:
		// Handle time.Time specially
		if field.Type.String() == "time.Time" {
			schema["type"] = "string"
			schema["format"] = "date-time"
		} else {
			return g.generateObject(field.Type)
		}
	case reflect.Ptr:
		// For pointers, generate schema for the underlying type
		elemField := reflect.StructField{
			Name: field.Name,
			Type: field.Type.Elem(),
			Tag:  field.Tag,
		}
		return g.generateFieldSchema(elemField)
	default:
		schema["type"] = "string" // Default to string for unknown types
	}

	return schema
}

func (g *Generator) parseSchemaTag(tag string, schema map[string]interface{}) {
	if tag == "" {
		return
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		
		// Handle enum values
		if strings.HasPrefix(part, "enum:") {
			values := strings.Split(part[5:], "|")
			schema["enum"] = values
			continue
		}

		// Handle min/max values
		if strings.HasPrefix(part, "min:") {
			var min interface{}
			if err := json.Unmarshal([]byte(part[4:]), &min); err == nil {
				schema["minimum"] = min
			}
			continue
		}

		if strings.HasPrefix(part, "max:") {
			var max interface{}
			if err := json.Unmarshal([]byte(part[4:]), &max); err == nil {
				schema["maximum"] = max
			}
			continue
		}

		// Handle pattern
		if strings.HasPrefix(part, "pattern:") {
			schema["pattern"] = part[8:]
			continue
		}

		// Handle format
		if strings.HasPrefix(part, "format:") {
			schema["format"] = part[7:]
			continue
		}

		// Handle default value
		if strings.HasPrefix(part, "default:") {
			var def interface{}
			if err := json.Unmarshal([]byte(part[8:]), &def); err == nil {
				schema["default"] = def
			} else {
				// Try as string if JSON parsing fails
				schema["default"] = part[8:]
			}
			continue
		}
	}
}

func getFieldName(field reflect.StructField, jsonTag string) string {
	if jsonTag == "" {
		return field.Name
	}

	parts := strings.Split(jsonTag, ",")
	name := strings.TrimSpace(parts[0])
	
	if name == "" {
		return field.Name
	}

	return name
}
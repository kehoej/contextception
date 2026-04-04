// Command gen-schema generates JSON Schema files from Contextception's Go types.
//
// Usage:
//
//	go run ./cmd/gen-schema
//
// This writes protocol/analysis-schema.json and protocol/change-schema.json.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/kehoej/contextception/schema"
)

// jsonSchema represents a JSON Schema object (draft 2020-12).
type jsonSchema struct {
	Schema      string                 `json:"$schema,omitempty"`
	ID          string                 `json:"$id,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Type        interface{}            `json:"type,omitempty"`
	Properties  map[string]*jsonSchema `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Items       *jsonSchema            `json:"items,omitempty"`
	Defs        map[string]*jsonSchema `json:"$defs,omitempty"`
	Ref         string                 `json:"$ref,omitempty"`
	OneOf       []*jsonSchema          `json:"oneOf,omitempty"`

	// Numeric constraints
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`

	// String constraints
	Enum []string `json:"enum,omitempty"`

	// Map support
	AdditionalProperties *jsonSchema `json:"additionalProperties,omitempty"`
}

func main() {
	outDir := "protocol"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal("create output dir: %v", err)
	}

	// Generate AnalysisOutput schema
	analysisSchema := generateSchema(
		reflect.TypeOf(schema.AnalysisOutput{}),
		"https://contextception.dev/schema/analysis-v3.2.json",
		"AnalysisOutput",
		"Contextception analysis output (schema v3.2). Describes the context bundle for a single or multi-file analysis.",
	)
	writeJSON(filepath.Join(outDir, "analysis-schema.json"), analysisSchema)

	// Generate ChangeReport schema
	changeSchema := generateSchema(
		reflect.TypeOf(schema.ChangeReport{}),
		"https://contextception.dev/schema/change-v1.0.json",
		"ChangeReport",
		"Contextception change report (schema v1.0). Describes the impact analysis for a branch diff.",
	)
	writeJSON(filepath.Join(outDir, "change-schema.json"), changeSchema)

	fmt.Println("Generated protocol/analysis-schema.json")
	fmt.Println("Generated protocol/change-schema.json")
}

func generateSchema(t reflect.Type, id, title, description string) *jsonSchema {
	defs := make(map[string]*jsonSchema)
	root := typeToSchema(t, defs)
	root.Schema = "https://json-schema.org/draft/2020-12/schema"
	root.ID = id
	root.Title = title
	root.Description = description
	if len(defs) > 0 {
		root.Defs = defs
	}
	return root
}

func typeToSchema(t reflect.Type, defs map[string]*jsonSchema) *jsonSchema {
	// Unwrap pointer
	if t.Kind() == reflect.Ptr {
		return typeToSchema(t.Elem(), defs)
	}

	switch t.Kind() {
	case reflect.String:
		return &jsonSchema{Type: "string"}
	case reflect.Bool:
		return &jsonSchema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &jsonSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &jsonSchema{Type: "number"}
	case reflect.Slice:
		elem := t.Elem()
		if elem.Kind() == reflect.Slice {
			// [][]string → array of arrays
			return &jsonSchema{
				Type:  "array",
				Items: typeToSchema(elem, defs),
			}
		}
		return &jsonSchema{
			Type:  "array",
			Items: typeToSchema(elem, defs),
		}
	case reflect.Map:
		return &jsonSchema{
			Type:                 "object",
			AdditionalProperties: typeToSchema(t.Elem(), defs),
		}
	case reflect.Struct:
		return structToSchema(t, defs)
	default:
		return &jsonSchema{Type: "string"}
	}
}

func structToSchema(t reflect.Type, defs map[string]*jsonSchema) *jsonSchema {
	// Check if already in defs to handle reuse
	name := t.Name()
	if name != "" {
		if _, exists := defs[name]; exists {
			return &jsonSchema{Ref: "#/$defs/" + name}
		}
		// Reserve the slot to prevent infinite recursion
		defs[name] = nil
	}

	props := make(map[string]*jsonSchema)
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}

		jsonName, opts := parseTag(tag)
		omitempty := strings.Contains(opts, "omitempty")

		fieldSchema := typeToSchema(field.Type, defs)

		// If the field is a pointer and omitempty, wrap with oneOf to allow null
		if field.Type.Kind() == reflect.Ptr && omitempty && fieldSchema.Ref != "" {
			fieldSchema = &jsonSchema{
				OneOf: []*jsonSchema{
					{Ref: fieldSchema.Ref},
					{Type: "null"},
				},
			}
		}

		props[jsonName] = fieldSchema

		if !omitempty {
			required = append(required, jsonName)
		}
	}

	s := &jsonSchema{
		Type:       "object",
		Properties: props,
	}
	if len(required) > 0 {
		s.Required = required
	}

	if name != "" {
		defs[name] = s
		return &jsonSchema{Ref: "#/$defs/" + name}
	}
	return s
}

func parseTag(tag string) (name string, opts string) {
	parts := strings.SplitN(tag, ",", 2)
	name = parts[0]
	if len(parts) > 1 {
		opts = parts[1]
	}
	return
}

func writeJSON(path string, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fatal("marshal JSON: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal("write %s: %v", path, err)
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "gen-schema: "+format+"\n", args...)
	os.Exit(1)
}

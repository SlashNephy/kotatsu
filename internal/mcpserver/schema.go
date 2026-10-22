package mcpserver

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

const (
	schemaObject = "object"
	schemaString = "string"
	// keyType は、JSON Schema の型を書くキー。
	keyType = "type"
)

// maxSchemaDepth は、入力の型を JSON Schema にするときにたどる深さの上限。入力の型が自分を含んでも止まるようにする。
const maxSchemaDepth = 8

// inputSchema は、GraphQL の操作の変数から、ツールの入力の JSON Schema を作る。
// 必須の変数 (null を許さず、既定値もないもの) は required にする。
func inputSchema(schema *ast.Schema, query string) (map[string]any, error) {
	doc, errs := gqlparser.LoadQueryWithRules(schema, query, nil)
	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid tool query: %w", errs)
	}
	if len(doc.Operations) != 1 {
		return nil, fmt.Errorf("a tool query must have exactly one operation, got %d", len(doc.Operations))
	}
	properties := make(map[string]any)
	required := make([]string, 0)
	for _, variable := range doc.Operations[0].VariableDefinitions {
		properties[variable.Variable] = typeSchema(schema, variable.Type, 0)
		if variable.Type.NonNull && variable.DefaultValue == nil {
			required = append(required, variable.Variable)
		}
	}

	return map[string]any{keyType: schemaObject, "properties": properties, "required": required, "additionalProperties": false}, nil
}

// typeSchema は、GraphQL の型を JSON Schema にする。
func typeSchema(schema *ast.Schema, t *ast.Type, depth int) map[string]any {
	if t.Elem != nil {
		return map[string]any{keyType: "array", "items": typeSchema(schema, t.Elem, depth)}
	}
	definition := schema.Types[t.NamedType]
	result := make(map[string]any)
	if definition != nil && !definition.BuiltIn && definition.Description != "" {
		result["description"] = strings.TrimSpace(definition.Description)
	}
	switch t.NamedType {
	case "String", "ID":
		result[keyType] = schemaString
	case "Int", "Int64":
		result[keyType] = "integer"
	case "Float":
		result[keyType] = "number"
	case "Boolean":
		result[keyType] = "boolean"
	case "Time":
		result[keyType] = schemaString
		result["format"] = "date-time"
	default:
		switch {
		case definition == nil:
		case definition.Kind == ast.Enum:
			values := make([]string, 0, len(definition.EnumValues))
			for _, value := range definition.EnumValues {
				values = append(values, value.Name)
			}
			result[keyType] = schemaString
			result["enum"] = values
		case definition.Kind == ast.InputObject && depth < maxSchemaDepth:
			properties := make(map[string]any)
			required := make([]string, 0)
			for _, field := range definition.Fields {
				property := typeSchema(schema, field.Type, depth+1)
				if field.Description != "" {
					property["description"] = strings.TrimSpace(field.Description)
				}
				properties[field.Name] = property
				if field.Type.NonNull && field.DefaultValue == nil {
					required = append(required, field.Name)
				}
			}
			result[keyType] = schemaObject
			result["properties"] = properties
			result["required"] = required
			result["additionalProperties"] = false
		}
	}

	return result
}

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// FieldAnnotation represents doc-gen annotations for a field
type FieldAnnotation struct {
	Exclude  bool
	Truncate bool
	Link     string
}

// TypeMapping maps JSON field names to Go type names
type TypeMapping struct {
	GoTypeName string // The Go type that contains this field (e.g., "APMFeatureConfig")
}

// ParseDocGenAnnotations parses +doc-gen: annotations from Go source files.
// Returns:
// - annotations: map of "StructName.jsonFieldName" -> FieldAnnotation
// - typeMap: map of "StructName.jsonFieldName" -> TypeMapping (for nested types)
func ParseDocGenAnnotations(filePath string) (map[string]FieldAnnotation, map[string]TypeMapping, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse file: %w", err)
	}

	annotations := make(map[string]FieldAnnotation)
	typeMap := make(map[string]TypeMapping)

	// Walk the AST to find struct declarations
	ast.Inspect(file, func(n ast.Node) bool {
		// Look for type declarations
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		// Only process struct types
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		structName := typeSpec.Name.Name

		// Iterate through struct fields
		for _, field := range structType.Fields.List {
			// Skip fields without names (embedded fields)
			if len(field.Names) == 0 {
				continue
			}

			// Extract JSON tag
			var jsonName string
			if field.Tag != nil {
				jsonName = extractJSONTag(field.Tag.Value)
			}
			if jsonName == "" {
				// If no json tag, skip this field
				continue
			}

			// Key format: "StructName.jsonFieldName"
			key := fmt.Sprintf("%s.%s", structName, jsonName)

			// Parse annotations from field comments
			if field.Doc != nil {
				annotation := parseAnnotationsFromCommentGroup(field.Doc)
				if annotation.Exclude || annotation.Truncate || annotation.Link != "" {
					annotations[key] = annotation
				}
			}

			// Get the type name for this field and store in typeMap
			fieldType := getFieldTypeName(field.Type)
			if fieldType != "" {
				typeMap[key] = TypeMapping{GoTypeName: fieldType}
			}
		}

		return true
	})

	return annotations, typeMap, nil
}

// parseAnnotationsFromCommentGroup extracts +doc-gen: annotations from comments
func parseAnnotationsFromCommentGroup(cg *ast.CommentGroup) FieldAnnotation {
	annotation := FieldAnnotation{}

	for _, comment := range cg.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))

		// Check for +doc-gen:exclude
		if strings.HasPrefix(text, "+doc-gen:exclude") {
			annotation.Exclude = true
		}

		// Check for +doc-gen:truncate
		if strings.HasPrefix(text, "+doc-gen:truncate") {
			annotation.Truncate = true
		}

		// Check for +doc-gen:link=<URL>
		if strings.HasPrefix(text, "+doc-gen:link=") {
			link := strings.TrimPrefix(text, "+doc-gen:link=")
			annotation.Link = strings.TrimSpace(link)
		}
	}

	return annotation
}

// extractJSONTag extracts the JSON field name from a struct tag
// e.g., `json:"enabled,omitempty"` returns "enabled"
func extractJSONTag(tag string) string {
	// Remove backticks
	tag = strings.Trim(tag, "`")

	// Look for json:"fieldname"
	if !strings.Contains(tag, "json:") {
		return ""
	}

	// Extract the json tag value
	parts := strings.Split(tag, "json:")
	if len(parts) < 2 {
		return ""
	}

	// Get the quoted value
	jsonPart := strings.TrimSpace(parts[1])
	jsonPart = strings.Trim(jsonPart, `"`)

	// Handle comma-separated options (e.g., "fieldname,omitempty")
	if idx := strings.Index(jsonPart, ","); idx != -1 {
		jsonPart = jsonPart[:idx]
	}

	return jsonPart
}

// getFieldTypeName extracts the type name from a field type expression
// Handles *Type, []Type, map[K]V, and plain Type
func getFieldTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// Simple type like "bool" or "APMFeatureConfig"
		return t.Name
	case *ast.StarExpr:
		// Pointer type like "*APMFeatureConfig"
		return getFieldTypeName(t.X)
	case *ast.ArrayType:
		// Array/slice type like "[]string"
		return getFieldTypeName(t.Elt)
	case *ast.MapType:
		// Map type - we care about the value type
		return getFieldTypeName(t.Value)
	case *ast.SelectorExpr:
		// Qualified type like "corev1.EnvVar"
		if ident, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", ident.Name, t.Sel.Name)
		}
	}
	return ""
}
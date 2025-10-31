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

// structDef represents a struct definition
type structDef struct {
	fields []fieldDef
}

// fieldDef represents a field in a struct
type fieldDef struct {
	jsonName   string
	typeName   string
	annotation FieldAnnotation
}

// ParseDocGenAnnotations parses +doc-gen: annotations from Go source files.
// Returns a map of path-based keys (e.g., "features.cspm.customBenchmarks") to FieldAnnotation.
// The function traces through type definitions starting from DatadogAgentSpec to build full paths.
func ParseDocGenAnnotations(filePath string) (map[string]FieldAnnotation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	// First pass: collect all struct definitions with their fields and annotations
	structDefs := make(map[string]*structDef)

	ast.Inspect(file, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		structName := typeSpec.Name.Name
		fields := make([]fieldDef, 0)

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				continue
			}

			var jsonName string
			if field.Tag != nil {
				jsonName = extractJSONTag(field.Tag.Value)
			}
			if jsonName == "" {
				continue
			}

			var annotation FieldAnnotation
			if field.Doc != nil {
				annotation = parseAnnotationsFromCommentGroup(field.Doc)
			}

			fieldType := getFieldTypeName(field.Type)

			fields = append(fields, fieldDef{
				jsonName:   jsonName,
				typeName:   fieldType,
				annotation: annotation,
			})
		}

		structDefs[structName] = &structDef{fields: fields}
		return true
	})

	// Second pass: build path-based annotations starting from DatadogAgentSpec
	annotations := make(map[string]FieldAnnotation)
	buildPaths([]string{}, "DatadogAgentSpec", structDefs, annotations)

	return annotations, nil
}

// buildPaths recursively builds path-based annotation keys
func buildPaths(path []string, currentType string, structDefs map[string]*structDef, annotations map[string]FieldAnnotation) {
	// Stop if this is an external type (contains package prefix like "corev1.EnvVar")
	if strings.Contains(currentType, ".") {
		return
	}

	structDef, exists := structDefs[currentType]
	if !exists {
		return
	}

	for _, field := range structDef.fields {
		fieldPath := append(path, field.jsonName)
		pathKey := strings.Join(fieldPath, ".")

		// Store annotation if it exists
		if field.annotation.Exclude || field.annotation.Truncate || field.annotation.Link != "" {
			annotations[pathKey] = field.annotation
		}

		// Recurse into nested types (unless annotated to stop)
		shouldRecurse := field.typeName != "" &&
						!field.annotation.Exclude &&
						!field.annotation.Truncate &&
						field.annotation.Link == ""

		if shouldRecurse {
			buildPaths(fieldPath, field.typeName, structDefs, annotations)
		}
	}
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
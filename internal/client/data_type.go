// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const maxPaimonFieldID = 1<<31 - 1

type structuredDataType struct {
	Type    string             `json:"type"`
	Element any                `json:"element,omitempty"`
	Key     any                `json:"key,omitempty"`
	Value   any                `json:"value,omitempty"`
	Fields  *[]structuredField `json:"fields,omitempty"`
	Length  int                `json:"length,omitempty"`
}

type structuredField struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Type         any     `json:"type"`
	Description  *string `json:"description,omitempty"`
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// MarshalJSON coordinates nested ROW field IDs across the complete schema.
// Paimon requires nested fields to carry IDs when their parent field has one.
func (s Schema) MarshalJSON() ([]byte, error) {
	type schemaJSON struct {
		Fields        []structuredField `json:"fields"`
		PartitionKeys []string          `json:"partitionKeys"`
		PrimaryKeys   []string          `json:"primaryKeys"`
		Options       map[string]string `json:"options"`
		Comment       *string           `json:"comment,omitempty"`
	}

	nextFieldID := -1
	usedFieldIDs := make(map[int]struct{}, len(s.Fields))
	for _, field := range s.Fields {
		if field.ID < 0 || field.ID > maxPaimonFieldID {
			return nil, fmt.Errorf("Paimon field %q ID must be between 0 and %d", field.Name, maxPaimonFieldID)
		}
		if _, duplicate := usedFieldIDs[field.ID]; duplicate {
			return nil, fmt.Errorf("Paimon field ID %d is duplicated", field.ID)
		}
		usedFieldIDs[field.ID] = struct{}{}
		if field.ID > nextFieldID {
			nextFieldID = field.ID
		}
	}
	fields := make([]structuredField, 0, len(s.Fields))
	for _, field := range s.Fields {
		encodedType, err := encodeDataType(string(field.Type), &nextFieldID)
		if err != nil {
			return nil, fmt.Errorf("encode Paimon field %q type: %w", field.Name, err)
		}
		fields = append(fields, structuredField{
			ID:           field.ID,
			Name:         field.Name,
			Type:         encodedType,
			Description:  field.Description,
			DefaultValue: field.DefaultValue,
		})
	}

	return json.Marshal(schemaJSON{
		Fields:        fields,
		PartitionKeys: nonNilSlice(s.PartitionKeys),
		PrimaryKeys:   nonNilSlice(s.PrimaryKeys),
		Options:       nonNilMap(s.Options),
		Comment:       s.Comment,
	})
}

func encodeDataType(input string, nextFieldID *int) (any, error) {
	typeName, notNull := stripNotNull(input)
	root, body, composite, err := compositeTypeParts(typeName)
	if err != nil {
		return nil, err
	}
	if !composite {
		return strings.TrimSpace(input), nil
	}

	serializedRoot := root
	if notNull {
		serializedRoot += " NOT NULL"
	}
	parts, err := splitTopLevel(body, ',')
	if err != nil {
		return nil, fmt.Errorf("invalid %s type %q: %w", root, input, err)
	}

	switch root {
	case "ARRAY", "MULTISET":
		if len(parts) != 1 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid %s type %q: expected one element type", root, input)
		}
		element, err := encodeDataType(parts[0], nextFieldID)
		if err != nil {
			return nil, err
		}

		return structuredDataType{Type: serializedRoot, Element: element}, nil
	case "MAP":
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid MAP type %q: expected key and value types", input)
		}
		key, err := encodeDataType(parts[0], nextFieldID)
		if err != nil {
			return nil, err
		}
		value, err := encodeDataType(parts[1], nextFieldID)
		if err != nil {
			return nil, err
		}

		return structuredDataType{Type: serializedRoot, Key: key, Value: value}, nil
	case "VECTOR":
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid VECTOR type %q: expected element type and length", input)
		}
		element, err := encodeDataType(parts[0], nextFieldID)
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || length <= 0 {
			return nil, fmt.Errorf("invalid VECTOR type %q: length must be a positive integer", input)
		}

		return structuredDataType{Type: serializedRoot, Element: element, Length: length}, nil
	case "ROW":
		fields := make([]structuredField, 0, len(parts))
		if len(parts) == 1 && parts[0] == "" {
			return structuredDataType{Type: serializedRoot, Fields: &fields}, nil
		}
		for _, part := range parts {
			name, fieldType, description, defaultValue, err := parseRowField(part)
			if err != nil {
				return nil, fmt.Errorf("invalid ROW type %q: %w", input, err)
			}
			if *nextFieldID >= maxPaimonFieldID {
				return nil, fmt.Errorf("Paimon nested field IDs exceed %d", maxPaimonFieldID)
			}
			*nextFieldID = *nextFieldID + 1
			fieldID := *nextFieldID
			encodedType, err := encodeDataType(fieldType, nextFieldID)
			if err != nil {
				return nil, err
			}
			fields = append(fields, structuredField{
				ID:           fieldID,
				Name:         name,
				Type:         encodedType,
				Description:  description,
				DefaultValue: defaultValue,
			})
		}

		return structuredDataType{Type: serializedRoot, Fields: &fields}, nil
	default:
		return nil, fmt.Errorf("unsupported composite Paimon type %q", root)
	}
}

func stripNotNull(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	const suffix = " NOT NULL"
	if len(trimmed) >= len(suffix) && strings.EqualFold(trimmed[len(trimmed)-len(suffix):], suffix) {
		return strings.TrimSpace(trimmed[:len(trimmed)-len(suffix)]), true
	}

	return trimmed, false
}

func compositeTypeParts(input string) (string, string, bool, error) {
	trimmed := strings.TrimSpace(input)
	opening := strings.IndexByte(trimmed, '<')
	leading := trimmed
	if opening >= 0 {
		leading = strings.TrimSpace(trimmed[:opening])
	}
	root := strings.ToUpper(leading)
	isCompositeRoot := root == "ARRAY" || root == "MAP" || root == "MULTISET" || root == "ROW" || root == "VECTOR"
	if !isCompositeRoot {
		return "", "", false, nil
	}
	if opening < 0 {
		return "", "", false, fmt.Errorf("invalid %s type %q: missing angle brackets", root, input)
	}

	closing, err := matchingAngleBracket(trimmed, opening)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid %s type %q: %w", root, input, err)
	}
	if strings.TrimSpace(trimmed[closing+1:]) != "" {
		return "", "", false, fmt.Errorf("invalid %s type %q: unexpected trailing content", root, input)
	}

	return root, trimmed[opening+1 : closing], true, nil
}

func matchingAngleBracket(input string, opening int) (int, error) {
	depth := 0
	parenDepth, squareDepth, braceDepth := 0, 0, 0
	inString := false
	inIdentifier := false
	for index := opening; index < len(input); index++ {
		switch input[index] {
		case '\'':
			if inIdentifier {
				continue
			}
			if inString && index+1 < len(input) && input[index+1] == '\'' {
				index++

				continue
			}
			inString = !inString
		case '`':
			if inString {
				continue
			}
			if inIdentifier && index+1 < len(input) && input[index+1] == '`' {
				index++

				continue
			}
			inIdentifier = !inIdentifier
		case '<':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				depth++
			}
		case '>':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				depth--
				if depth == 0 {
					return index, nil
				}
				if depth < 0 {
					return -1, fmt.Errorf("unexpected closing angle bracket")
				}
			}
		case '(':
			if !inString && !inIdentifier {
				parenDepth++
			}
		case ')':
			if !inString && !inIdentifier {
				parenDepth--
			}
		case '[':
			if !inString && !inIdentifier {
				squareDepth++
			}
		case ']':
			if !inString && !inIdentifier {
				squareDepth--
			}
		case '{':
			if !inString && !inIdentifier {
				braceDepth++
			}
		case '}':
			if !inString && !inIdentifier {
				braceDepth--
			}
		}
		if parenDepth < 0 || squareDepth < 0 || braceDepth < 0 {
			return -1, fmt.Errorf("unbalanced delimiters")
		}
	}
	if inString || inIdentifier || parenDepth != 0 || squareDepth != 0 || braceDepth != 0 {
		return -1, fmt.Errorf("unbalanced delimiters or quotes")
	}

	return -1, fmt.Errorf("missing closing angle bracket")
}

func splitTopLevel(input string, separator byte) ([]string, error) {
	parts := make([]string, 0, 2)
	start := 0
	angleDepth, parenDepth, squareDepth, braceDepth := 0, 0, 0, 0
	inString := false
	inIdentifier := false
	for index := 0; index < len(input); index++ {
		character := input[index]
		switch character {
		case '\'':
			if inIdentifier {
				continue
			}
			if inString && index+1 < len(input) && input[index+1] == '\'' {
				index++

				continue
			}
			inString = !inString
		case '`':
			if inString {
				continue
			}
			if inIdentifier && index+1 < len(input) && input[index+1] == '`' {
				index++

				continue
			}
			inIdentifier = !inIdentifier
		case '<':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				angleDepth++
			}
		case '>':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				angleDepth--
			}
		case '(':
			if !inString && !inIdentifier {
				parenDepth++
			}
		case ')':
			if !inString && !inIdentifier {
				parenDepth--
			}
		case '[':
			if !inString && !inIdentifier {
				squareDepth++
			}
		case ']':
			if !inString && !inIdentifier {
				squareDepth--
			}
		case '{':
			if !inString && !inIdentifier {
				braceDepth++
			}
		case '}':
			if !inString && !inIdentifier {
				braceDepth--
			}
		default:
			if character == separator && !inString && !inIdentifier && angleDepth == 0 && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:index]))
				start = index + 1
			}
		}
		if angleDepth < 0 || parenDepth < 0 || squareDepth < 0 || braceDepth < 0 {
			return nil, fmt.Errorf("unbalanced delimiters")
		}
	}
	if inString || inIdentifier || angleDepth != 0 || parenDepth != 0 || squareDepth != 0 || braceDepth != 0 {
		return nil, fmt.Errorf("unbalanced delimiters or quotes")
	}
	parts = append(parts, strings.TrimSpace(input[start:]))

	return parts, nil
}

func parseRowField(input string) (string, string, *string, *string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", nil, nil, fmt.Errorf("empty field declaration")
	}

	name, remainder, err := consumeRowFieldName(trimmed)
	if err != nil {
		return "", "", nil, nil, err
	}
	if strings.TrimSpace(remainder) == "" {
		return "", "", nil, nil, fmt.Errorf("field %q is missing a type", name)
	}

	commentPosition := findTopLevelKeyword(remainder, "COMMENT")
	defaultPosition := findTopLevelKeyword(remainder, "DEFAULT")
	typeEnd := len(remainder)
	if commentPosition >= 0 && commentPosition < typeEnd {
		typeEnd = commentPosition
	}
	if defaultPosition >= 0 && defaultPosition < typeEnd {
		typeEnd = defaultPosition
	}
	fieldType := strings.TrimSpace(remainder[:typeEnd])
	if fieldType == "" {
		return "", "", nil, nil, fmt.Errorf("field %q is missing a type", name)
	}

	var description, defaultValue *string
	if commentPosition >= 0 {
		if defaultPosition >= 0 && defaultPosition < commentPosition {
			return "", "", nil, nil, fmt.Errorf("field %q COMMENT must precede DEFAULT", name)
		}
		commentEnd := len(remainder)
		if defaultPosition >= 0 {
			commentEnd = defaultPosition
		}
		commentText := strings.TrimSpace(remainder[commentPosition+len("COMMENT") : commentEnd])
		decoded, err := decodeSQLString(commentText)
		if err != nil {
			return "", "", nil, nil, fmt.Errorf("field %q has invalid COMMENT: %w", name, err)
		}
		description = &decoded
	}
	if defaultPosition >= 0 {
		value := strings.TrimSpace(remainder[defaultPosition+len("DEFAULT"):])
		if value == "" {
			return "", "", nil, nil, fmt.Errorf("field %q has an empty DEFAULT", name)
		}
		defaultValue = &value
	}

	return name, fieldType, description, defaultValue, nil
}

func consumeRowFieldName(input string) (string, string, error) {
	if input[0] != '`' {
		index := strings.IndexFunc(input, unicode.IsSpace)
		if index <= 0 {
			return "", "", fmt.Errorf("expected a field name followed by a type")
		}

		return input[:index], input[index:], nil
	}

	var name strings.Builder
	for index := 1; index < len(input); index++ {
		if input[index] != '`' {
			name.WriteByte(input[index])

			continue
		}
		if index+1 < len(input) && input[index+1] == '`' {
			name.WriteByte('`')
			index++

			continue
		}
		if index+1 < len(input) && !unicode.IsSpace(rune(input[index+1])) {
			return "", "", fmt.Errorf("expected whitespace after quoted field name")
		}

		return name.String(), input[index+1:], nil
	}

	return "", "", fmt.Errorf("unterminated quoted field name")
}

func findTopLevelKeyword(input, keyword string) int {
	angleDepth, parenDepth, squareDepth, braceDepth := 0, 0, 0, 0
	inString := false
	inIdentifier := false
	for index := 0; index < len(input); index++ {
		switch input[index] {
		case '\'':
			if inIdentifier {
				continue
			}
			if inString && index+1 < len(input) && input[index+1] == '\'' {
				index++

				continue
			}
			inString = !inString
		case '`':
			if inString {
				continue
			}
			if inIdentifier && index+1 < len(input) && input[index+1] == '`' {
				index++

				continue
			}
			inIdentifier = !inIdentifier
		case '<':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				angleDepth++
			}
		case '>':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				angleDepth--
			}
		case '(':
			if !inString && !inIdentifier {
				parenDepth++
			}
		case ')':
			if !inString && !inIdentifier {
				parenDepth--
			}
		case '[':
			if !inString && !inIdentifier {
				squareDepth++
			}
		case ']':
			if !inString && !inIdentifier {
				squareDepth--
			}
		case '{':
			if !inString && !inIdentifier {
				braceDepth++
			}
		case '}':
			if !inString && !inIdentifier {
				braceDepth--
			}
		default:
			if !inString && !inIdentifier && angleDepth == 0 && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && hasKeywordAt(input, index, keyword) {
				return index
			}
		}
	}

	return -1
}

func hasKeywordAt(input string, index int, keyword string) bool {
	if index+len(keyword) > len(input) || !strings.EqualFold(input[index:index+len(keyword)], keyword) {
		return false
	}
	if index > 0 && !unicode.IsSpace(rune(input[index-1])) {
		return false
	}
	end := index + len(keyword)

	return end == len(input) || unicode.IsSpace(rune(input[end]))
}

func decodeSQLString(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) < 2 || trimmed[0] != '\'' || trimmed[len(trimmed)-1] != '\'' {
		return "", fmt.Errorf("expected a single-quoted string")
	}
	contents := trimmed[1 : len(trimmed)-1]
	for index := 0; index < len(contents); index++ {
		if contents[index] != '\'' {
			continue
		}
		if index+1 >= len(contents) || contents[index+1] != '\'' {
			return "", fmt.Errorf("unescaped single quote")
		}
		index++
	}

	return strings.ReplaceAll(contents, "''", "'"), nil
}

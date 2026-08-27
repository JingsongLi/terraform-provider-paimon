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
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Paimon reserves IDs at and above SpecialFields.SYSTEM_FIELD_ID_START.
const maxPaimonFieldID = (1 << 30) - 2

var (
	sizedAtomicTypePattern  = regexp.MustCompile(`(?i)^(CHAR|VARCHAR|BINARY|VARBINARY)\s*(?:\(\s*([0-9]+)\s*\))?$`)
	decimalTypePattern      = regexp.MustCompile(`(?i)^(DECIMAL|DEC|NUMERIC)\s*(?:\(\s*([0-9]+)\s*(?:,\s*([0-9]+)\s*)?\))?$`)
	timeTypePattern         = regexp.MustCompile(`(?i)^TIME\s*(?:\(\s*([0-9]+)\s*\))?(?:\s+WITHOUT\s+TIME\s+ZONE)?$`)
	timestampTypePattern    = regexp.MustCompile(`(?i)^TIMESTAMP\s*(?:\(\s*([0-9]+)\s*\))?(?:\s+WITHOUT\s+TIME\s+ZONE)?$`)
	timestampLTZPattern     = regexp.MustCompile(`(?i)^(?:TIMESTAMP_LTZ\s*(?:\(\s*([0-9]+)\s*\))?|TIMESTAMP\s*(?:\(\s*([0-9]+)\s*\))?\s+WITH\s+LOCAL\s+TIME\s+ZONE)$`)
	notNullSuffixPattern    = regexp.MustCompile(`(?i)\s+NOT\s+NULL\s*$`)
	nullSuffixPattern       = regexp.MustCompile(`(?i)\s+NULL\s*$`)
	suffixCollectionPattern = regexp.MustCompile(`(?i)\s+(ARRAY|MULTISET)\s*$`)
)

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
		if matches := suffixCollectionPattern.FindStringSubmatchIndex(typeName); matches != nil {
			elementType := strings.TrimSpace(typeName[:matches[0]])
			if elementType == "" {
				return nil, fmt.Errorf("invalid suffix collection type %q: missing element type", input)
			}
			element, err := encodeDataType(elementType, nextFieldID)
			if err != nil {
				return nil, err
			}
			root := strings.ToUpper(typeName[matches[2]:matches[3]])
			if notNull {
				root += " NOT NULL"
			}

			return structuredDataType{Type: root, Element: element}, nil
		}

		return canonicalAtomicDataType(input), nil
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

// EquivalentDataTypes reports whether two SQL spellings encode to the same
// language-neutral Paimon REST data type.
func EquivalentDataTypes(left, right DataType) bool {
	canonicalLeft, err := canonicalDataType(left)
	if err != nil {
		return false
	}
	canonicalRight, err := canonicalDataType(right)

	return err == nil && canonicalLeft == canonicalRight
}

func canonicalDataType(value DataType) (DataType, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var canonical DataType
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return "", err
	}

	return canonical, nil
}

func canonicalAtomicDataType(input string) string {
	typeName, notNull := stripNotNull(input)
	canonical, recognized := canonicalAtomicTypeName(typeName)
	if !recognized {
		return strings.TrimSpace(input)
	}
	if notNull {
		canonical += " NOT NULL"
	}

	return canonical
}

func canonicalAtomicTypeName(input string) (string, bool) {
	if matches := sizedAtomicTypePattern.FindStringSubmatch(strings.TrimSpace(input)); matches != nil {
		length := 1
		if matches[2] != "" {
			var valid bool
			length, valid = boundedInteger(matches[2], 1, 1<<31-1)
			if !valid {
				return "", false
			}
		}
		root := strings.ToUpper(matches[1])
		if root == "VARCHAR" && length == 1<<31-1 {
			return "STRING", true
		}
		if root == "VARBINARY" && length == 1<<31-1 {
			return "BYTES", true
		}

		return fmt.Sprintf("%s(%d)", root, length), true
	}
	if matches := decimalTypePattern.FindStringSubmatch(strings.TrimSpace(input)); matches != nil {
		precision, scale := 10, 0
		if matches[2] != "" {
			var valid bool
			precision, valid = boundedInteger(matches[2], 1, 38)
			if !valid {
				return "", false
			}
		}
		if matches[3] != "" {
			var valid bool
			scale, valid = boundedInteger(matches[3], 0, precision)
			if !valid {
				return "", false
			}
		}

		return fmt.Sprintf("DECIMAL(%d, %d)", precision, scale), true
	}
	if matches := timestampLTZPattern.FindStringSubmatch(strings.TrimSpace(input)); matches != nil {
		precisionText := matches[1]
		if precisionText == "" {
			precisionText = matches[2]
		}
		precision := 6
		if precisionText != "" {
			var valid bool
			precision, valid = boundedInteger(precisionText, 0, 9)
			if !valid {
				return "", false
			}
		}

		return fmt.Sprintf("TIMESTAMP(%d) WITH LOCAL TIME ZONE", precision), true
	}
	if matches := timestampTypePattern.FindStringSubmatch(strings.TrimSpace(input)); matches != nil {
		precision := 6
		if matches[1] != "" {
			var valid bool
			precision, valid = boundedInteger(matches[1], 0, 9)
			if !valid {
				return "", false
			}
		}

		return fmt.Sprintf("TIMESTAMP(%d)", precision), true
	}
	if matches := timeTypePattern.FindStringSubmatch(strings.TrimSpace(input)); matches != nil {
		precision := 0
		if matches[1] != "" {
			var valid bool
			precision, valid = boundedInteger(matches[1], 0, 9)
			if !valid {
				return "", false
			}
		}

		return fmt.Sprintf("TIME(%d)", precision), true
	}
	if parameters, matched := geospatialTypeParameters(input, "GEOMETRY"); matched {
		if len(parameters) != 1 {
			return "", false
		}
		crs, valid := canonicalCRS(parameters[0])
		if !valid {
			return "", false
		}

		return "GEOMETRY(" + crs + ")", true
	}
	if parameters, matched := geospatialTypeParameters(input, "GEOGRAPHY"); matched {
		if len(parameters) < 1 || len(parameters) > 2 {
			return "", false
		}
		crs, valid := canonicalCRS(parameters[0])
		if !valid {
			return "", false
		}
		algorithm := "SPHERICAL"
		if len(parameters) == 2 {
			parsed, valid := geospatialParameter(parameters[1])
			if !valid {
				return "", false
			}
			algorithm = strings.ToUpper(parsed)
		}
		switch algorithm {
		case "SPHERICAL", "VINCENTY", "THOMAS", "ANDOYER", "KARNEY":
		default:
			return "", false
		}

		return "GEOGRAPHY(" + crs + ", " + strings.ToLower(algorithm) + ")", true
	}

	normalized := strings.ToUpper(strings.Join(strings.Fields(input), " "))
	switch normalized {
	case "STRING", "BOOLEAN", "TINYINT", "SMALLINT", "BIGINT", "FLOAT", "DOUBLE", "DATE", "VARIANT", "BLOB", "BYTES":
		return normalized, true
	case "INT", "INTEGER":
		return "INT", true
	case "DOUBLE PRECISION":
		return "DOUBLE", true
	case "GEOMETRY":
		return "GEOMETRY(OGC:CRS84)", true
	case "GEOGRAPHY":
		return "GEOGRAPHY(OGC:CRS84, spherical)", true
	default:
		return "", false
	}
}

func geospatialTypeParameters(input, root string) ([]string, bool) {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) <= len(root) || !strings.EqualFold(trimmed[:len(root)], root) {
		return nil, false
	}
	remainder := strings.TrimSpace(trimmed[len(root):])
	if len(remainder) < 2 || remainder[0] != '(' || remainder[len(remainder)-1] != ')' {
		return nil, false
	}
	parameters, err := splitTopLevel(remainder[1:len(remainder)-1], ',')
	if err != nil {
		return nil, false
	}

	return parameters, true
}

func canonicalCRS(input string) (string, bool) {
	value, valid := geospatialParameter(input)
	if !valid {
		return "", false
	}
	value = strings.ToUpper(value)
	if value[0] >= '0' && value[0] <= '9' || strings.ContainsAny(value, " \t\r\n<>() ,.'`") {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'", true
	}

	return value, true
}

func geospatialParameter(input string) (string, bool) {
	value := strings.TrimSpace(input)
	if strings.HasPrefix(value, "'") {
		decoded, err := decodeSQLString(value)
		if err != nil {
			return "", false
		}

		return decoded, decoded != ""
	}
	if value == "" || strings.ContainsAny(value, " \t\r\n<>() ,.'`") {
		return "", false
	}

	return value, true
}

func boundedInteger(input string, minimum, maximum int) (int, bool) {
	value, err := strconv.Atoi(input)
	if err != nil || value < minimum || value > maximum {
		return 0, false
	}

	return value, true
}

func stripNotNull(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if suffix := notNullSuffixPattern.FindStringIndex(trimmed); suffix != nil {
		return strings.TrimSpace(trimmed[:suffix[0]]), true
	}
	if suffix := nullSuffixPattern.FindStringIndex(trimmed); suffix != nil {
		return strings.TrimSpace(trimmed[:suffix[0]]), false
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
	defaultAtDepth := make(map[int]bool)
	inString := false
	inIdentifier := false
	for index := opening; index < len(input); index++ {
		if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && hasKeywordAt(input, index, "DEFAULT") {
			defaultAtDepth[depth] = true
		}
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
			if !inString && !inIdentifier && !defaultAtDepth[depth] && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && isCompositeAngleOpening(input, index) {
				depth++
			}
		case '>':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && isStructuralAngleClosing(input, index) {
				delete(defaultAtDepth, depth)
				depth--
				if depth == 0 {
					return index, nil
				}
				if depth < 0 {
					return -1, errors.New("unexpected closing angle bracket")
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
		case ',':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
				delete(defaultAtDepth, depth)
			}
		}
		if parenDepth < 0 || squareDepth < 0 || braceDepth < 0 {
			return -1, errors.New("unbalanced delimiters")
		}
	}
	if inString || inIdentifier || parenDepth != 0 || squareDepth != 0 || braceDepth != 0 {
		return -1, errors.New("unbalanced delimiters or quotes")
	}

	return -1, errors.New("missing closing angle bracket")
}

func splitTopLevel(input string, separator byte) ([]string, error) {
	parts := make([]string, 0, 2)
	start := 0
	angleDepth, parenDepth, squareDepth, braceDepth := 0, 0, 0, 0
	defaultAtDepth := make(map[int]bool)
	inString := false
	inIdentifier := false
	for index := 0; index < len(input); index++ {
		character := input[index]
		if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && hasKeywordAt(input, index, "DEFAULT") {
			defaultAtDepth[angleDepth] = true
		}
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
			if !inString && !inIdentifier && !defaultAtDepth[angleDepth] && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && isCompositeAngleOpening(input, index) {
				angleDepth++
			}
		case '>':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && isStructuralAngleClosing(input, index) {
				delete(defaultAtDepth, angleDepth)
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
		}
		if angleDepth < 0 || parenDepth < 0 || squareDepth < 0 || braceDepth < 0 {
			return nil, errors.New("unbalanced delimiters")
		}
		if character == separator && !inString && !inIdentifier && angleDepth == 0 && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 {
			delete(defaultAtDepth, angleDepth)
			parts = append(parts, strings.TrimSpace(input[start:index]))
			start = index + 1
		}
	}
	if inString || inIdentifier || angleDepth != 0 || parenDepth != 0 || squareDepth != 0 || braceDepth != 0 {
		return nil, errors.New("unbalanced delimiters or quotes")
	}
	parts = append(parts, strings.TrimSpace(input[start:]))

	return parts, nil
}

func parseRowField(input string) (string, string, *string, *string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", nil, nil, errors.New("empty field declaration")
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
			return "", "", errors.New("expected a field name followed by a type")
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
			return "", "", errors.New("expected whitespace after quoted field name")
		}

		return name.String(), input[index+1:], nil
	}

	return "", "", errors.New("unterminated quoted field name")
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
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && isCompositeAngleOpening(input, index) {
				angleDepth++
			}
		case '>':
			if !inString && !inIdentifier && parenDepth == 0 && squareDepth == 0 && braceDepth == 0 && isStructuralAngleClosing(input, index) {
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

func isCompositeAngleOpening(input string, index int) bool {
	if index < 0 || index >= len(input) || input[index] != '<' {
		return false
	}
	end := index
	for end > 0 && unicode.IsSpace(rune(input[end-1])) {
		end--
	}
	start := end
	for start > 0 {
		character := input[start-1]
		if character != '_' && (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') {
			break
		}
		start--
	}

	switch strings.ToUpper(input[start:end]) {
	case "ARRAY", "MAP", "MULTISET", "ROW", "VECTOR":
		return true
	default:
		return false
	}
}

func isStructuralAngleClosing(input string, index int) bool {
	if index < 0 || index >= len(input) || input[index] != '>' {
		return false
	}
	next := index + 1
	for next < len(input) && unicode.IsSpace(rune(input[next])) {
		next++
	}
	if next == len(input) {
		return true
	}
	switch input[next] {
	case ',', '>', ')', ']', '}':
		return true
	}

	return hasKeywordAt(input, next, "NOT") || hasKeywordAt(input, next, "COMMENT") || hasKeywordAt(input, next, "DEFAULT")
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
		return "", errors.New("expected a single-quoted string")
	}
	contents := trimmed[1 : len(trimmed)-1]
	for index := 0; index < len(contents); index++ {
		if contents[index] != '\'' {
			continue
		}
		if index+1 >= len(contents) || contents[index+1] != '\'' {
			return "", errors.New("unescaped single quote")
		}
		index++
	}

	return strings.ReplaceAll(contents, "''", "'"), nil
}

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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type ConfigResponse struct {
	Defaults  map[string]string `json:"defaults"`
	Overrides map[string]string `json:"overrides"`
}

type Database struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Location  string            `json:"location"`
	Options   map[string]string `json:"options"`
	Owner     string            `json:"owner"`
	CreatedAt int64             `json:"createdAt"`
	CreatedBy string            `json:"createdBy"`
	UpdatedAt int64             `json:"updatedAt"`
	UpdatedBy string            `json:"updatedBy"`
}

type createDatabaseRequest struct {
	Name    string            `json:"name"`
	Options map[string]string `json:"options"`
}

type alterDatabaseRequest struct {
	Removals []string          `json:"removals"`
	Updates  map[string]string `json:"updates"`
}

type Identifier struct {
	Database string `json:"database"`
	Object   string `json:"object"`
}

type Schema struct {
	Fields        []Field           `json:"fields"`
	PartitionKeys []string          `json:"partitionKeys"`
	PrimaryKeys   []string          `json:"primaryKeys"`
	Options       map[string]string `json:"options"`
	Comment       *string           `json:"comment,omitempty"`
}

type Field struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Type         DataType `json:"type"`
	Description  *string  `json:"description,omitempty"`
	DefaultValue *string  `json:"defaultValue,omitempty"`
}

type Table struct {
	ID         string `json:"id"`
	Database   string `json:"database"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsExternal bool   `json:"isExternal"`
	SchemaID   int64  `json:"schemaId"`
	Schema     Schema `json:"schema"`
	Owner      string `json:"owner"`
	CreatedAt  int64  `json:"createdAt"`
	CreatedBy  string `json:"createdBy"`
	UpdatedAt  int64  `json:"updatedAt"`
	UpdatedBy  string `json:"updatedBy"`
}

type createTableRequest struct {
	Identifier Identifier `json:"identifier"`
	Schema     Schema     `json:"schema"`
}

type SchemaChange map[string]any

type alterTableRequest struct {
	Changes []SchemaChange `json:"changes"`
}

// DataType is Paimon's language-neutral REST representation of a data type.
// Atomic types use SQL strings. ARRAY, MAP, MULTISET, ROW and VECTOR use the
// structured JSON form required by Paimon's REST type parser.
type DataType string

func (t DataType) MarshalJSON() ([]byte, error) {
	nextFieldID := -1
	value, err := encodeDataType(string(t), &nextFieldID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(value)
}

func (t *DataType) UnmarshalJSON(data []byte) error {
	var primitive string
	if err := json.Unmarshal(data, &primitive); err == nil {
		*t = DataType(primitive)

		return nil
	}

	var structured struct {
		Type    string          `json:"type"`
		Element json.RawMessage `json:"element"`
		Key     json.RawMessage `json:"key"`
		Value   json.RawMessage `json:"value"`
		Fields  []Field         `json:"fields"`
		Length  int             `json:"length"`
	}
	if err := json.Unmarshal(data, &structured); err != nil {
		return fmt.Errorf("decode Paimon data type: %w", err)
	}

	typeName := strings.TrimSpace(structured.Type)
	notNull := strings.HasSuffix(strings.ToUpper(typeName), " NOT NULL")
	root := strings.TrimSpace(strings.TrimSuffix(strings.ToUpper(typeName), " NOT NULL"))
	var sqlType string

	switch root {
	case "ARRAY", "MULTISET":
		element, err := decodeNestedType(structured.Element)
		if err != nil {
			return err
		}
		sqlType = fmt.Sprintf("%s<%s>", root, element)
	case "MAP":
		key, err := decodeNestedType(structured.Key)
		if err != nil {
			return err
		}
		value, err := decodeNestedType(structured.Value)
		if err != nil {
			return err
		}
		sqlType = fmt.Sprintf("MAP<%s, %s>", key, value)
	case "ROW":
		fields := make([]string, 0, len(structured.Fields))
		for _, field := range structured.Fields {
			part := quoteIdentifier(field.Name) + " " + string(field.Type)
			if field.Description != nil && *field.Description != "" {
				part += " COMMENT '" + strings.ReplaceAll(*field.Description, "'", "''") + "'"
			}
			if field.DefaultValue != nil {
				part += " DEFAULT " + *field.DefaultValue
			}
			fields = append(fields, part)
		}
		sqlType = "ROW<" + strings.Join(fields, ", ") + ">"
	case "VECTOR":
		element, err := decodeNestedType(structured.Element)
		if err != nil {
			return err
		}
		sqlType = fmt.Sprintf("VECTOR<%s, %s>", element, strconv.Itoa(structured.Length))
	default:
		return fmt.Errorf("unsupported structured Paimon data type %q", structured.Type)
	}

	if notNull {
		sqlType += " NOT NULL"
	}
	*t = DataType(sqlType)

	return nil
}

func decodeNestedType(data json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return "", errors.New("structured Paimon data type is missing a nested type")
	}
	var nested DataType
	if err := json.Unmarshal(data, &nested); err != nil {
		return "", err
	}

	return string(nested), nil
}

func quoteIdentifier(value string) string {
	if value != "" {
		valid := true
		for i, r := range value {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
				valid = false

				break
			}
		}
		if valid {
			return value
		}
	}

	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

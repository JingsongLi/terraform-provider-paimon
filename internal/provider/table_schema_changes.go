// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information regarding
// copyright ownership. The ASF licenses this file to You under the Apache
// License, Version 2.0 (the "License"); you may not use this file except in
// compliance with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/apache/terraform-provider-paimon/internal/client"
)

func assignTemporaryIDsToNewFields(before []client.Field, planned []tableFieldModel, after []client.Field) error {
	if len(planned) != len(after) {
		return errors.New("the planned field models and converted schema have different lengths")
	}

	used := make(map[int]struct{})
	next := 0
	reserve := func(id int) {
		used[id] = struct{}{}
		if id >= next {
			next = id + 1
		}
	}
	for _, field := range before {
		reserve(field.ID)
		for _, nestedID := range field.NestedFieldIDs {
			reserve(nestedID)
		}
	}
	for index, field := range planned {
		if !field.ID.IsNull() && !field.ID.IsUnknown() {
			reserve(int(field.ID.ValueInt64()))
		}
		for _, nestedID := range after[index].NestedFieldIDs {
			reserve(nestedID)
		}
	}

	for index, field := range planned {
		if !field.ID.IsNull() && !field.ID.IsUnknown() {
			continue
		}
		for {
			if _, exists := used[next]; !exists {
				break
			}
			next++
		}
		if next > maxPaimonFieldID {
			return errors.New("no Paimon field IDs remain for a newly added field")
		}
		after[index].ID = next
		reserve(next)
	}

	return nil
}

func tableFieldSchemaChanges(before, after []client.Field, addBeforePartition bool, partitionKeys []string) ([]client.SchemaChange, error) {
	beforeByID, err := fieldsByID(before)
	if err != nil {
		return nil, err
	}
	afterByID, err := fieldsByID(after)
	if err != nil {
		return nil, err
	}

	changes := make([]client.SchemaChange, 0)
	for _, previous := range before {
		planned, exists := afterByID[previous.ID]
		if !exists {
			continue
		}
		previousType, previousNullable := splitFieldType(previous.Type)
		plannedType, plannedNullable := splitFieldType(planned.Type)
		path := []string{previous.Name}
		if !client.EquivalentDataTypes(previousType, plannedType) {
			plannedTypeField := planned
			plannedTypeField.Type = plannedType
			changes = append(changes, client.SchemaChange{
				"action":          "updateColumnType",
				"fieldNames":      path,
				"newDataType":     schemaChangeDataType(plannedTypeField, before, previous.ID),
				"keepNullability": true,
			})
		}
		if previousNullable != plannedNullable {
			changes = append(changes, client.SchemaChange{
				"action":         "updateColumnNullability",
				"fieldNames":     path,
				"newNullability": plannedNullable,
			})
		}
		if !stringPointersEqual(previous.Description, planned.Description) {
			changes = append(changes, client.SchemaChange{
				"action":     "updateColumnComment",
				"fieldNames": path,
				"newComment": planned.Description,
			})
		}
		if !stringPointersEqual(previous.DefaultValue, planned.DefaultValue) {
			changes = append(changes, client.SchemaChange{
				"action":          "updateColumnDefaultValue",
				"fieldNames":      path,
				"newDefaultValue": planned.DefaultValue,
			})
		}
	}

	for _, previous := range before {
		if _, exists := afterByID[previous.ID]; !exists {
			changes = append(changes, client.SchemaChange{"action": "dropColumn", "fieldNames": []string{previous.Name}})
		}
	}

	retainedNames := make(map[string]int)
	pendingRenames := make(map[string]client.Field)
	for _, previous := range before {
		if planned, exists := afterByID[previous.ID]; exists {
			retainedNames[previous.Name] = planned.ID
			if previous.Name != planned.Name {
				pendingRenames[previous.Name] = planned
			}
		}
	}
	for len(pendingRenames) > 0 {
		progress := false
		for _, previous := range before {
			planned, pending := pendingRenames[previous.Name]
			if !pending {
				continue
			}
			if conflictingID, conflict := retainedNames[planned.Name]; conflict && conflictingID != planned.ID {
				continue
			}
			changes = append(changes, client.SchemaChange{
				"action":     "renameColumn",
				"fieldNames": []string{previous.Name},
				"newName":    planned.Name,
			})
			delete(retainedNames, previous.Name)
			retainedNames[planned.Name] = planned.ID
			delete(pendingRenames, previous.Name)
			progress = true
		}
		if !progress {
			return nil, errors.New("cannot apply a cycle of table field renames in one apply; use an intermediate name")
		}
	}

	for _, planned := range after {
		if _, exists := beforeByID[planned.ID]; exists {
			continue
		}
		changes = append(changes, client.SchemaChange{
			"action":     "addColumn",
			"fieldNames": []string{planned.Name},
			"dataType":   schemaChangeDataType(planned, before, -1),
			"comment":    planned.Description,
		})
		if planned.DefaultValue != nil {
			changes = append(changes, client.SchemaChange{
				"action":          "updateColumnDefaultValue",
				"fieldNames":      []string{planned.Name},
				"newDefaultValue": planned.DefaultValue,
			})
		}
	}

	currentOrder := make([]string, 0, len(after))
	for _, previous := range before {
		if planned, exists := afterByID[previous.ID]; exists {
			currentOrder = append(currentOrder, planned.Name)
		}
	}
	partitionKeySet := make(map[string]struct{}, len(partitionKeys))
	for _, name := range partitionKeys {
		partitionKeySet[name] = struct{}{}
	}
	for _, planned := range after {
		if _, exists := beforeByID[planned.ID]; !exists {
			insertAt := len(currentOrder)
			if addBeforePartition {
				for index, name := range currentOrder {
					if _, partitionKey := partitionKeySet[name]; partitionKey {
						insertAt = index

						break
					}
				}
			}
			currentOrder = insertString(currentOrder, insertAt, planned.Name)
		}
	}
	for index, planned := range after {
		currentIndex := slices.Index(currentOrder, planned.Name)
		if currentIndex == index {
			continue
		}
		changes = append(changes, columnPositionChange(after, index))
		currentOrder = moveString(currentOrder, currentIndex, index)
	}

	return changes, nil
}

func columnPositionChange(fields []client.Field, index int) client.SchemaChange {
	move := client.SchemaChange{"fieldName": fields[index].Name, "type": "FIRST"}
	if index > 0 {
		move["type"] = "AFTER"
		move["referenceFieldName"] = fields[index-1].Name
	}

	return client.SchemaChange{"action": "updateColumnPosition", "move": move}
}

func fieldsByID(fields []client.Field) (map[int]client.Field, error) {
	result := make(map[int]client.Field, len(fields))
	names := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, duplicate := result[field.ID]; duplicate {
			return nil, fmt.Errorf("field ID %d appears more than once", field.ID)
		}
		if _, duplicate := names[field.Name]; duplicate {
			return nil, fmt.Errorf("field name %q appears more than once", field.Name)
		}
		result[field.ID] = field
		names[field.Name] = struct{}{}
	}

	return result, nil
}

func splitFieldType(value client.DataType) (client.DataType, bool) {
	typeName := strings.TrimSpace(string(value))
	if strings.HasSuffix(strings.ToUpper(typeName), " NOT NULL") {
		return client.DataType(strings.TrimSpace(typeName[:len(typeName)-len(" NOT NULL")])), false
	}

	return client.DataType(typeName), true
}

func schemaChangeDataType(field client.Field, existing []client.Field, excludeNestedFieldID int) client.SchemaChangeDataType {
	used := make([]int, 0)
	for _, current := range existing {
		used = append(used, current.ID)
		if current.ID == excludeNestedFieldID {
			continue
		}
		for _, nestedID := range current.NestedFieldIDs {
			used = append(used, nestedID)
		}
	}

	return client.SchemaChangeDataType{Type: field.Type, NestedFieldIDs: field.NestedFieldIDs, UsedFieldIDs: used}
}

func stringPointersEqual(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func moveString(values []string, from, to int) []string {
	value := values[from]
	values = append(values[:from], values[from+1:]...)
	values = append(values, "")
	copy(values[to+1:], values[to:])
	values[to] = value

	return values
}

func insertString(values []string, index int, value string) []string {
	values = append(values, "")
	copy(values[index+1:], values[index:])
	values[index] = value

	return values
}

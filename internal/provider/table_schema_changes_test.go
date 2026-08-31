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
	"encoding/json"
	"testing"

	"github.com/apache/terraform-provider-paimon/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableFieldSchemaChanges(t *testing.T) {
	oldDescription := "old"
	newDescription := "new"
	newDefault := "1"
	before := []client.Field{
		{ID: 0, Name: "alpha", Type: "BIGINT NOT NULL", Description: &oldDescription},
		{ID: 1, Name: "drop_me", Type: "STRING"},
		{ID: 2, Name: "move_first", Type: "STRING"},
	}
	after := []client.Field{
		{ID: 2, Name: "move_first", Type: "STRING"},
		{ID: 0, Name: "beta", Type: "DOUBLE", Description: &newDescription, DefaultValue: &newDefault},
		{ID: 3, Name: "added", Type: "ROW<value STRING>", NestedFieldIDs: map[string]int{"/fields/value": 4}},
	}

	changes, err := tableFieldSchemaChanges(before, after, false, nil)
	require.NoError(t, err)
	actions := make([]string, 0, len(changes))
	for _, change := range changes {
		actions = append(actions, change["action"].(string))
	}
	assert.Equal(t, []string{
		"updateColumnType",
		"updateColumnNullability",
		"updateColumnComment",
		"updateColumnDefaultValue",
		"dropColumn",
		"renameColumn",
		"addColumn",
		"updateColumnPosition",
	}, actions)
	assert.Equal(t, []string{"alpha"}, changes[0]["fieldNames"])
	assert.Equal(t, "beta", changes[5]["newName"])
	assert.Equal(t, client.SchemaChange{"fieldName": "move_first", "type": "FIRST"}, changes[7]["move"])

	encoded, err := json.Marshal(changes[6]["dataType"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"ROW","fields":[{"id":4,"name":"value","type":"STRING"}]}`, string(encoded))
}

func TestStabilizePlannedFieldIdentitiesAcrossInsertionAndReordering(t *testing.T) {
	state := []tableFieldModel{
		tableFieldForTest("id", types.Int64Value(0)),
		tableFieldForTest("event_time", types.Int64Value(1)),
	}
	state[1].NestedFieldIDs = types.MapValueMust(types.Int64Type, map[string]attr.Value{"/fields/value": types.Int64Value(2)})
	configured := []tableFieldModel{
		tableFieldForTest("event_time", types.Int64Null()),
		tableFieldForTest("id", types.Int64Null()),
		tableFieldForTest("payload", types.Int64Null()),
	}
	planned := []tableFieldModel{
		tableFieldForTest("event_time", types.Int64Unknown()),
		tableFieldForTest("id", types.Int64Unknown()),
		tableFieldForTest("payload", types.Int64Unknown()),
	}

	stabilizePlannedFieldIdentities(configured, state, planned)

	assert.Equal(t, int64(1), planned[0].ID.ValueInt64())
	assert.Equal(t, int64(0), planned[1].ID.ValueInt64())
	assert.True(t, planned[2].ID.IsUnknown())
	assert.True(t, planned[0].NestedFieldIDs.Equal(state[1].NestedFieldIDs))
}

func TestStabilizePlannedFieldIdentitiesUsesExplicitIDForRename(t *testing.T) {
	state := []tableFieldModel{tableFieldForTest("old_name", types.Int64Value(7))}
	configured := []tableFieldModel{tableFieldForTest("new_name", types.Int64Value(7))}
	planned := []tableFieldModel{tableFieldForTest("new_name", types.Int64Value(7))}

	stabilizePlannedFieldIdentities(configured, state, planned)

	assert.Equal(t, int64(7), planned[0].ID.ValueInt64())
	assert.True(t, planned[0].NestedFieldIDs.Equal(state[0].NestedFieldIDs))
}

func TestNewFieldDoesNotReuseDroppedFieldID(t *testing.T) {
	before := []client.Field{
		{ID: 0, Name: "a", Type: "STRING"},
		{ID: 1, Name: "b", Type: "STRING"},
	}
	planned := []tableFieldModel{
		tableFieldForTest("b", types.Int64Value(1)),
		tableFieldForTest("c", types.Int64Unknown()),
	}
	after := []client.Field{
		{ID: 1, Name: "b", Type: "STRING"},
		{ID: 0, Name: "c", Type: "STRING"},
	}

	require.NoError(t, assignTemporaryIDsToNewFields(before, planned, after))
	assert.Equal(t, 2, after[1].ID)
	changes, err := tableFieldSchemaChanges(before, after, false, nil)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	assert.Equal(t, "dropColumn", changes[0]["action"])
	assert.Equal(t, []string{"a"}, changes[0]["fieldNames"])
	assert.Equal(t, "addColumn", changes[1]["action"])
	assert.Equal(t, []string{"c"}, changes[1]["fieldNames"])
}

func TestTableFieldSchemaChangesOrdersRenameChain(t *testing.T) {
	changes, err := tableFieldSchemaChanges(
		[]client.Field{{ID: 0, Name: "alpha", Type: "STRING"}, {ID: 1, Name: "beta", Type: "STRING"}},
		[]client.Field{{ID: 0, Name: "beta", Type: "STRING"}, {ID: 1, Name: "gamma", Type: "STRING"}},
		false,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	assert.Equal(t, []string{"beta"}, changes[0]["fieldNames"])
	assert.Equal(t, "gamma", changes[0]["newName"])
	assert.Equal(t, []string{"alpha"}, changes[1]["fieldNames"])
	assert.Equal(t, "beta", changes[1]["newName"])
}

func TestTableFieldSchemaChangesRejectsRenameCycle(t *testing.T) {
	_, err := tableFieldSchemaChanges(
		[]client.Field{{ID: 0, Name: "left", Type: "STRING"}, {ID: 1, Name: "right", Type: "STRING"}},
		[]client.Field{{ID: 0, Name: "right", Type: "STRING"}, {ID: 1, Name: "left", Type: "STRING"}},
		false,
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intermediate name")
}

func TestTableIDRoundTrip(t *testing.T) {
	id := tableID("analytics.prod", "events.v1")
	assert.Equal(t, "database=analytics.prod&table=events.v1", id)
	database, table, err := parseTableID(id)
	require.NoError(t, err)
	assert.Equal(t, "analytics.prod", database)
	assert.Equal(t, "events.v1", table)

	database, table, err = parseTableID("analytics.events")
	require.NoError(t, err)
	assert.Equal(t, "analytics", database)
	assert.Equal(t, "events", table)
}

func TestMutationReconciliationMatching(t *testing.T) {
	expected := client.Schema{
		Fields:  []client.Field{{ID: 1, Name: "payload", Type: "ROW<value STRING>"}},
		Options: map[string]string{"bucket": "2"},
	}
	observed := &client.Table{Schema: client.Schema{
		Fields: []client.Field{{
			ID:             1,
			Name:           "payload",
			Type:           "ROW<value STRING>",
			NestedFieldIDs: map[string]int{"/fields/value": 2},
		}},
		Options: map[string]string{"bucket": "2", "server-only": "kept"},
	}}
	assert.False(t, tableMatchesPlannedSchema(observed, expected, true, true))
	assert.True(t, tableMatchesPlannedSchema(observed, expected, false, false))

	expected.Fields[0].NestedFieldIDs = map[string]int{"/fields/value": 3}
	assert.False(t, tableMatchesPlannedSchema(observed, expected, true, false))

	database := &client.Database{Name: "analytics", Options: map[string]string{"owner": "data", "server-only": "kept"}}
	assert.False(t, databaseMatchesOptions(database, "analytics", map[string]string{"owner": "data"}, true))
	assert.True(t, databaseMatchesOptions(database, "analytics", map[string]string{"owner": "data"}, false))
}

func TestCompositeFieldTypeReplacementUsesStableIDs(t *testing.T) {
	before := []tableFieldModel{
		tableFieldForTest("first", types.Int64Value(1)),
		tableFieldForTest("payload", types.Int64Value(2)),
	}
	before[1].Type = types.StringValue("ROW<value STRING>")
	after := []tableFieldModel{before[1], before[0]}
	assert.False(t, compositeFieldTypesRequireReplace(before, after), "reordering must not replace a table")

	after[0].Type = types.StringValue("ROW<value BIGINT>")
	assert.True(t, compositeFieldTypesRequireReplace(before, after))

	after[0].Type = types.StringValue("STRING")
	before[1].Type = types.StringValue("INT")
	assert.False(t, compositeFieldTypesRequireReplace(before, after), "atomic casts use SchemaChange")
}

func TestKeyFieldTypeChangeRequiresReplacement(t *testing.T) {
	before := []tableFieldModel{
		tableFieldForTest("id", types.Int64Value(0)),
		tableFieldForTest("payload", types.Int64Value(1)),
	}
	before[0].Type = types.StringValue("BIGINT")
	after := append([]tableFieldModel(nil), before...)
	after[0].Type = types.StringValue("INT")

	assert.True(t, keyFieldTypesRequireReplace(before, after, []string{"id"}))
	assert.False(t, keyFieldTypesRequireReplace(before, after, []string{"payload"}))
}

func TestNewNonNullableFieldRequiresReplacement(t *testing.T) {
	before := []tableFieldModel{tableFieldForTest("id", types.Int64Value(0))}
	added := tableFieldForTest("required_value", types.Int64Unknown())
	added.Nullable = types.BoolValue(false)
	after := []tableFieldModel{before[0], added}

	assert.True(t, newNonNullableFieldsRequireReplace(before, after))
	after[1].Nullable = types.BoolValue(true)
	assert.False(t, newNonNullableFieldsRequireReplace(before, after))
	after[1].Nullable = types.BoolUnknown()
	after[1].Type = types.StringValue("STRING NOT NULL")
	assert.True(t, newNonNullableFieldsRequireReplace(before, after))
}

func TestTableFieldSchemaChangesAccountsForAddBeforePartition(t *testing.T) {
	before := []client.Field{
		{ID: 0, Name: "id", Type: "BIGINT"},
		{ID: 1, Name: "day", Type: "DATE"},
	}
	after := append(append([]client.Field(nil), before...), client.Field{ID: 2, Name: "payload", Type: "STRING"})

	changes, err := tableFieldSchemaChanges(before, after, true, []string{"day"})
	require.NoError(t, err)
	require.Len(t, changes, 2)
	assert.Equal(t, "addColumn", changes[0]["action"])
	assert.Equal(t, client.SchemaChange{
		"action": "updateColumnPosition",
		"move": client.SchemaChange{
			"fieldName":          "payload",
			"type":               "AFTER",
			"referenceFieldName": "day",
		},
	}, changes[1])

	changes, err = tableFieldSchemaChanges(before, after, false, []string{"day"})
	require.NoError(t, err)
	require.Len(t, changes, 1)
}

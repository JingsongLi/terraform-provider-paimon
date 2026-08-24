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

package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/apache/terraform-provider-paimon/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type tableFieldModel struct {
	ID           types.Int64  `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	Nullable     types.Bool   `tfsdk:"nullable"`
	Description  types.String `tfsdk:"description"`
	DefaultValue types.String `tfsdk:"default_value"`
}

// Paimon reserves IDs at and above SpecialFields.SYSTEM_FIELD_ID_START.
const maxPaimonFieldID = (1 << 30) - 2

func tableFieldAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":            types.Int64Type,
		"name":          types.StringType,
		"type":          types.StringType,
		"nullable":      types.BoolType,
		"description":   types.StringType,
		"default_value": types.StringType,
	}
}

func tableResourceAttributes() map[string]rschema.Attribute {
	return map[string]rschema.Attribute{
		"id": rschema.StringAttribute{
			Description: "Terraform identifier in database.table form.",
			Computed:    true,
		},
		"catalog_id": rschema.StringAttribute{Description: "Server-assigned table identifier.", Computed: true},
		"database": rschema.StringAttribute{
			Description:   "Database containing the table.",
			Required:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"name": rschema.StringAttribute{
			Description:   "Table name.",
			Required:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"fields": rschema.ListNestedAttribute{
			Description:   "Ordered table fields. Field, primary-key and partition-key changes replace the table in this initial provider version.",
			Required:      true,
			PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
			NestedObject:  rschema.NestedAttributeObject{Attributes: tableFieldResourceAttributes()},
		},
		"partition_keys": rschema.ListAttribute{
			Description:   "Ordered partition key field names.",
			Optional:      true,
			Computed:      true,
			ElementType:   types.StringType,
			PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
		},
		"primary_keys": rschema.ListAttribute{
			Description:   "Ordered primary key field names.",
			Optional:      true,
			Computed:      true,
			ElementType:   types.StringType,
			PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
		},
		"options": rschema.MapAttribute{
			Description:   "Table options managed by Terraform. Options not declared here are preserved. Paimon options that are immutable after creation cause replacement when changed.",
			Optional:      true,
			ElementType:   types.StringType,
			Validators:    []validator.Map{reservedTableOptionsValidator{}},
			PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplaceIf(immutableTableOptionsRequiresReplace, "replaces the table when an immutable Paimon option changes", "replaces the table when an immutable Paimon option changes")},
		},
		"server_options": rschema.MapAttribute{
			Description: "All table options returned by the REST Catalog.",
			Computed:    true,
			ElementType: types.StringType,
		},
		"comment":     rschema.StringAttribute{Description: "Table comment.", Optional: true},
		"schema_id":   rschema.Int64Attribute{Description: "Current server schema identifier.", Computed: true},
		"path":        rschema.StringAttribute{Description: "Table storage path returned by the server.", Computed: true},
		"is_external": rschema.BoolAttribute{Description: "Whether the table is external.", Computed: true},
		"owner":       rschema.StringAttribute{Description: "Table owner returned by the server.", Computed: true},
		"created_at":  rschema.Int64Attribute{Description: "Creation timestamp in milliseconds since the Unix epoch.", Computed: true},
		"created_by":  rschema.StringAttribute{Description: "Principal that created the table.", Computed: true},
		"updated_at":  rschema.Int64Attribute{Description: "Last update timestamp in milliseconds since the Unix epoch.", Computed: true},
		"updated_by":  rschema.StringAttribute{Description: "Principal that last updated the table.", Computed: true},
	}
}

func tableFieldResourceAttributes() map[string]rschema.Attribute {
	return map[string]rschema.Attribute{
		"id": rschema.Int64Attribute{
			Description: "Stable Paimon field ID between 0 and " + strconv.Itoa(maxPaimonFieldID) + ". The next available ID is assigned when omitted.",
			Optional:    true,
			Computed:    true,
		},
		"name": rschema.StringAttribute{Description: "Field name.", Required: true},
		"type": rschema.StringAttribute{
			Description: "Canonical Paimon SQL data type, for example BIGINT, STRING or DECIMAL(12, 2). Configure top-level nullability with nullable.",
			Required:    true,
			Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"nullable": rschema.BoolAttribute{
			Description: "Whether the field accepts null values. Defaults to true when omitted.",
			Optional:    true,
			Computed:    true,
		},
		"description":   rschema.StringAttribute{Description: "Field description.", Optional: true},
		"default_value": rschema.StringAttribute{Description: "Field default expression.", Optional: true},
	}
}

func tableDataSourceAttributes() map[string]dschema.Attribute {
	return map[string]dschema.Attribute{
		"id":         dschema.StringAttribute{Description: "Terraform identifier in database.table form.", Computed: true},
		"catalog_id": dschema.StringAttribute{Description: "Server-assigned table identifier.", Computed: true},
		"database":   dschema.StringAttribute{Description: "Database containing the table.", Required: true},
		"name":       dschema.StringAttribute{Description: "Table name.", Required: true},
		"fields": dschema.ListNestedAttribute{
			Description:  "Ordered table fields.",
			Computed:     true,
			NestedObject: dschema.NestedAttributeObject{Attributes: tableFieldDataSourceAttributes()},
		},
		"partition_keys": dschema.ListAttribute{Description: "Ordered partition key field names.", Computed: true, ElementType: types.StringType},
		"primary_keys":   dschema.ListAttribute{Description: "Ordered primary key field names.", Computed: true, ElementType: types.StringType},
		"options":        dschema.MapAttribute{Description: "All table options returned by the REST Catalog.", Computed: true, ElementType: types.StringType},
		"comment":        dschema.StringAttribute{Description: "Table comment.", Computed: true},
		"schema_id":      dschema.Int64Attribute{Description: "Current server schema identifier.", Computed: true},
		"path":           dschema.StringAttribute{Description: "Table storage path returned by the server.", Computed: true},
		"is_external":    dschema.BoolAttribute{Description: "Whether the table is external.", Computed: true},
		"owner":          dschema.StringAttribute{Description: "Table owner returned by the server.", Computed: true},
		"created_at":     dschema.Int64Attribute{Description: "Creation timestamp in milliseconds since the Unix epoch.", Computed: true},
		"created_by":     dschema.StringAttribute{Description: "Principal that created the table.", Computed: true},
		"updated_at":     dschema.Int64Attribute{Description: "Last update timestamp in milliseconds since the Unix epoch.", Computed: true},
		"updated_by":     dschema.StringAttribute{Description: "Principal that last updated the table.", Computed: true},
	}
}

func tableFieldDataSourceAttributes() map[string]dschema.Attribute {
	return map[string]dschema.Attribute{
		"id":            dschema.Int64Attribute{Description: "Stable field ID.", Computed: true},
		"name":          dschema.StringAttribute{Description: "Field name.", Computed: true},
		"type":          dschema.StringAttribute{Description: "Canonical Paimon SQL data type.", Computed: true},
		"nullable":      dschema.BoolAttribute{Description: "Whether the field accepts null values.", Computed: true},
		"description":   dschema.StringAttribute{Description: "Field description.", Computed: true},
		"default_value": dschema.StringAttribute{Description: "Field default expression.", Computed: true},
	}
}

func schemaFromResourceModel(ctx context.Context, model *tableResourceModel, diags *diag.Diagnostics) client.Schema {
	partitionKeys := stringListFromValue(ctx, model.PartitionKeys, diags)
	primaryKeys := stringListFromValue(ctx, model.PrimaryKeys, diags)
	options := mapFromValue(ctx, model.Options, diags)
	if diags.HasError() {
		return client.Schema{}
	}
	if _, exists := options["primary-key"]; exists {
		diags.AddError("Reserved table option", "Configure primary_keys instead of the primary-key table option.")
	}
	if _, exists := options["partition"]; exists {
		diags.AddError("Reserved table option", "Configure partition_keys instead of the partition table option.")
	}
	primaryKeyNullable := false
	if configured, exists := options["primary-key.nullable"]; exists {
		parsed, err := strconv.ParseBool(configured)
		if err != nil {
			diags.AddError("Invalid primary-key.nullable option", "Expected a boolean value, got: "+configured)
		} else {
			primaryKeyNullable = parsed
		}
	}
	primaryKeySet := make(map[string]struct{}, len(primaryKeys))
	for _, key := range primaryKeys {
		primaryKeySet[key] = struct{}{}
	}

	var fieldModels []tableFieldModel
	diags.Append(model.Fields.ElementsAs(ctx, &fieldModels, false)...)
	fieldIDs := allocateFieldIDs(fieldModels, diags)
	fields := make([]client.Field, 0, len(fieldModels))
	fieldNames := make(map[string]struct{}, len(fieldModels))
	for index, field := range fieldModels {
		typeName := strings.TrimSpace(field.Type.ValueString())
		hasNotNullSuffix := strings.HasSuffix(strings.ToUpper(typeName), " NOT NULL")
		_, isPrimaryKey := primaryKeySet[field.Name.ValueString()]
		nullable := !isPrimaryKey || primaryKeyNullable
		if (field.Nullable.IsNull() || field.Nullable.IsUnknown()) && hasNotNullSuffix {
			nullable = false
		}
		if !field.Nullable.IsNull() && !field.Nullable.IsUnknown() {
			nullable = field.Nullable.ValueBool()
			if isPrimaryKey && nullable != primaryKeyNullable {
				diags.AddError(
					"Conflicting primary key nullability",
					"Field "+field.Name.ValueString()+" is a primary key. Its nullable value must match the primary-key.nullable table option (false by default).",
				)

				continue
			}
		}
		if hasNotNullSuffix {
			if nullable {
				diags.AddError("Conflicting field nullability", "Field "+field.Name.ValueString()+" includes NOT NULL in type while nullable is true. Remove NOT NULL and configure nullable instead.")

				continue
			}
		} else if !nullable {
			typeName += " NOT NULL"
		}
		fields = append(fields, client.Field{
			ID:           fieldIDs[index],
			Name:         field.Name.ValueString(),
			Type:         client.DataType(typeName),
			Description:  optionalStringPointer(field.Description),
			DefaultValue: optionalStringPointer(field.DefaultValue),
		})
		if _, duplicate := fieldNames[field.Name.ValueString()]; duplicate {
			diags.AddError("Duplicate Paimon field", "Field names must be unique: "+field.Name.ValueString())
		}
		fieldNames[field.Name.ValueString()] = struct{}{}
	}
	validateKeyFields("partition_keys", partitionKeys, fieldNames, diags)
	validateKeyFields("primary_keys", primaryKeys, fieldNames, diags)

	return client.Schema{
		Fields:        fields,
		PartitionKeys: partitionKeys,
		PrimaryKeys:   primaryKeys,
		Options:       options,
		Comment:       optionalStringPointer(model.Comment),
	}
}

func allocateFieldIDs(fields []tableFieldModel, diags *diag.Diagnostics) []int {
	ids := make([]int, len(fields))
	used := make(map[int]int, len(fields))
	for index, field := range fields {
		if field.ID.IsNull() || field.ID.IsUnknown() {
			continue
		}
		configured := field.ID.ValueInt64()
		if configured < 0 || configured > maxPaimonFieldID {
			diags.AddError("Invalid Paimon field ID", "Field "+field.Name.ValueString()+" must have a field ID between 0 and "+strconv.Itoa(maxPaimonFieldID)+".")

			continue
		}
		id := int(configured)
		if previous, duplicate := used[id]; duplicate {
			diags.AddError("Duplicate Paimon field ID", "Fields "+fields[previous].Name.ValueString()+" and "+field.Name.ValueString()+" use the same field ID: "+strconv.Itoa(id))

			continue
		}
		ids[index] = id
		used[id] = index
	}

	next := 0
	for index, field := range fields {
		if !field.ID.IsNull() && !field.ID.IsUnknown() {
			continue
		}
		for {
			if _, exists := used[next]; !exists {
				break
			}
			next++
		}
		ids[index] = next
		used[next] = index
		next++
	}

	return ids
}

func validateKeyFields(attribute string, keys []string, fields map[string]struct{}, diags *diag.Diagnostics) {
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, duplicate := seen[key]; duplicate {
			diags.AddError("Duplicate Paimon key field", attribute+" contains the field more than once: "+key)
		}
		seen[key] = struct{}{}
		if _, exists := fields[key]; !exists {
			diags.AddError("Unknown Paimon key field", attribute+" references a field that is not declared: "+key)
		}
	}
}

func fieldsValueFromRemote(ctx context.Context, fields []client.Field, diags *diag.Diagnostics) types.List {
	return fieldsValueFromModels(ctx, fieldModelsFromRemote(fields), diags)
}

func resourceFieldsValueFromRemote(ctx context.Context, managed types.List, fields []client.Field, diags *diag.Diagnostics) types.List {
	models := fieldModelsFromRemote(fields)
	if managed.IsNull() || managed.IsUnknown() {
		return fieldsValueFromModels(ctx, models, diags)
	}
	var managedModels []tableFieldModel
	newDiags := managed.ElementsAs(ctx, &managedModels, false)
	if newDiags.HasError() || len(managedModels) != len(models) {
		return fieldsValueFromModels(ctx, models, diags)
	}
	for index := range models {
		if managedModels[index].Name.IsNull() || managedModels[index].Name.IsUnknown() || managedModels[index].Type.IsNull() || managedModels[index].Type.IsUnknown() {
			continue
		}
		if managedModels[index].Name.ValueString() != models[index].Name.ValueString() {
			continue
		}
		if client.EquivalentDataTypes(
			client.DataType(managedModels[index].Type.ValueString()),
			client.DataType(models[index].Type.ValueString()),
		) {
			models[index].Type = managedModels[index].Type
		}
	}

	return fieldsValueFromModels(ctx, models, diags)
}

func fieldModelsFromRemote(fields []client.Field) []tableFieldModel {
	models := make([]tableFieldModel, 0, len(fields))
	for _, field := range fields {
		typeName := strings.TrimSpace(string(field.Type))
		nullable := true
		if strings.HasSuffix(strings.ToUpper(typeName), " NOT NULL") {
			nullable = false
			typeName = strings.TrimSpace(typeName[:len(typeName)-len(" NOT NULL")])
		}
		models = append(models, tableFieldModel{
			ID:           types.Int64Value(int64(field.ID)),
			Name:         types.StringValue(field.Name),
			Type:         types.StringValue(typeName),
			Nullable:     types.BoolValue(nullable),
			Description:  stringValueFromPointer(field.Description),
			DefaultValue: stringValueFromPointer(field.DefaultValue),
		})
	}

	return models
}

func fieldsValueFromModels(ctx context.Context, models []tableFieldModel, diags *diag.Diagnostics) types.List {
	value, newDiags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: tableFieldAttrTypes()}, models)
	diags.Append(newDiags...)

	return value
}

func optionalStringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueString()

	return &result
}

func stringValueFromPointer(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}

	return types.StringValue(*value)
}

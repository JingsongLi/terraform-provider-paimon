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
	"fmt"
	"sort"
	"strings"

	"github.com/apache/terraform-provider-paimon/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &tableResource{}
	_ resource.ResourceWithImportState = &tableResource{}
)

type tableResource struct {
	client *client.Client
}

type tableResourceModel struct {
	ID            types.String `tfsdk:"id"`
	CatalogID     types.String `tfsdk:"catalog_id"`
	Database      types.String `tfsdk:"database"`
	Name          types.String `tfsdk:"name"`
	Fields        types.List   `tfsdk:"fields"`
	PartitionKeys types.List   `tfsdk:"partition_keys"`
	PrimaryKeys   types.List   `tfsdk:"primary_keys"`
	Options       types.Map    `tfsdk:"options"`
	ServerOptions types.Map    `tfsdk:"server_options"`
	Comment       types.String `tfsdk:"comment"`
	SchemaID      types.Int64  `tfsdk:"schema_id"`
	Path          types.String `tfsdk:"path"`
	IsExternal    types.Bool   `tfsdk:"is_external"`
	Owner         types.String `tfsdk:"owner"`
	CreatedAt     types.Int64  `tfsdk:"created_at"`
	CreatedBy     types.String `tfsdk:"created_by"`
	UpdatedAt     types.Int64  `tfsdk:"updated_at"`
	UpdatedBy     types.String `tfsdk:"updated_by"`
}

func NewTableResource() resource.Resource {
	return &tableResource{}
}

func (r *tableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table"
}

func (r *tableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a table in a Paimon REST Catalog. Dropping this resource calls the catalog's managed-table drop API and can remove table data.",
		Attributes:  tableResourceAttributes(),
	}
}

func (r *tableResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientFromProviderData(req.ProviderData, &r.client, &resp.Diagnostics, "paimon_table resource")
}

func (r *tableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tableSchema := schemaFromResourceModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateTable(ctx, plan.Database.ValueString(), plan.Name.ValueString(), tableSchema); err != nil {
		resp.Diagnostics.AddError("Unable to create Paimon table", err.Error())

		return
	}
	r.readIntoState(ctx, &plan, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *tableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	table, err := r.client.GetTable(ctx, state.Database.ValueString(), state.Name.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)

		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Paimon table", err.Error())

		return
	}
	setTableResourceModel(ctx, &state, table, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *tableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan tableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	before := mapFromValue(ctx, state.Options, &resp.Diagnostics)
	after := mapFromValue(ctx, plan.Options, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	removals, updates := diffTableOptions(before, after)
	sort.Strings(removals)
	updateKeys := make([]string, 0, len(updates))
	for key := range updates {
		updateKeys = append(updateKeys, key)
	}
	sort.Strings(updateKeys)

	changes := make([]client.SchemaChange, 0, len(removals)+len(updates)+1)
	for _, key := range removals {
		changes = append(changes, client.SchemaChange{"action": "removeOption", "key": key})
	}
	for _, key := range updateKeys {
		changes = append(changes, client.SchemaChange{"action": "setOption", "key": key, "value": updates[key]})
	}
	if !state.Comment.Equal(plan.Comment) {
		changes = append(changes, client.SchemaChange{"action": "updateComment", "comment": optionalStringPointer(plan.Comment)})
	}

	if len(changes) > 0 {
		if err := r.client.AlterTable(ctx, plan.Database.ValueString(), plan.Name.ValueString(), changes); err != nil {
			resp.Diagnostics.AddError("Unable to update Paimon table", err.Error())

			return
		}
	}
	r.readIntoState(ctx, &plan, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *tableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DropTable(ctx, state.Database.ValueString(), state.Name.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to drop Paimon table", err.Error())
	}
}

func (r *tableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	separator := strings.LastIndex(req.ID, ".")
	if separator <= 0 || separator == len(req.ID)-1 {
		resp.Diagnostics.AddError("Invalid Paimon table import identifier", "Expected an identifier in database.table form, got: "+req.ID)

		return
	}
	database, name := req.ID[:separator], req.ID[separator+1:]
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database"), database)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

func (r *tableResource) readIntoState(ctx context.Context, model *tableResourceModel, diags *diag.Diagnostics) {
	table, err := r.client.GetTable(ctx, model.Database.ValueString(), model.Name.ValueString())
	if err != nil {
		diags.AddError("Unable to read Paimon table after change", err.Error())

		return
	}
	setTableResourceModel(ctx, model, table, diags)
}

func setTableResourceModel(ctx context.Context, model *tableResourceModel, table *client.Table, diags *diag.Diagnostics) {
	database := table.Database
	if database == "" {
		database = model.Database.ValueString()
	}
	name := table.Name
	if name == "" {
		name = model.Name.ValueString()
	}
	model.ID = types.StringValue(fmt.Sprintf("%s.%s", database, name))
	model.CatalogID = types.StringValue(table.ID)
	model.Database = types.StringValue(database)
	model.Name = types.StringValue(name)
	model.Fields = resourceFieldsValueFromRemote(ctx, model.Fields, table.Schema.Fields, diags)
	model.PartitionKeys = stringListValue(ctx, table.Schema.PartitionKeys, diags)
	model.PrimaryKeys = stringListValue(ctx, table.Schema.PrimaryKeys, diags)
	model.Options = syncManagedTableOptions(ctx, model.Options, table.Schema.Options, diags)
	model.ServerOptions = stringMapValue(ctx, table.Schema.Options, diags)
	model.Comment = stringValueFromPointer(table.Schema.Comment)
	model.SchemaID = types.Int64Value(table.SchemaID)
	model.Path = types.StringValue(table.Path)
	model.IsExternal = types.BoolValue(table.IsExternal)
	model.Owner = types.StringValue(table.Owner)
	model.CreatedAt = types.Int64Value(table.CreatedAt)
	model.CreatedBy = types.StringValue(table.CreatedBy)
	model.UpdatedAt = types.Int64Value(table.UpdatedAt)
	model.UpdatedBy = types.StringValue(table.UpdatedBy)
}

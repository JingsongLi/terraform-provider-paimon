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

	"github.com/apache/terraform-provider-paimon/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &databaseResource{}
	_ resource.ResourceWithImportState = &databaseResource{}
)

type databaseResource struct {
	client *client.Client
}

type databaseResourceModel struct {
	ID            types.String `tfsdk:"id"`
	CatalogID     types.String `tfsdk:"catalog_id"`
	Name          types.String `tfsdk:"name"`
	Options       types.Map    `tfsdk:"options"`
	ServerOptions types.Map    `tfsdk:"server_options"`
	Location      types.String `tfsdk:"location"`
	Owner         types.String `tfsdk:"owner"`
	CreatedAt     types.Int64  `tfsdk:"created_at"`
	CreatedBy     types.String `tfsdk:"created_by"`
	UpdatedAt     types.Int64  `tfsdk:"updated_at"`
	UpdatedBy     types.String `tfsdk:"updated_by"`
}

func NewDatabaseResource() resource.Resource {
	return &databaseResource{}
}

func (r *databaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *databaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a database in a Paimon REST Catalog.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Terraform identifier, equal to the database name.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"catalog_id": schema.StringAttribute{Description: "Server-assigned database identifier.", Computed: true},
			"name": schema.StringAttribute{
				Description:   "Database name.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"options": schema.MapAttribute{
				Description: "Database options managed by Terraform. Options not declared here are preserved.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"server_options": schema.MapAttribute{
				Description: "All database options returned by the REST Catalog.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"location":   schema.StringAttribute{Description: "Database location returned by the server.", Computed: true},
			"owner":      schema.StringAttribute{Description: "Database owner returned by the server.", Computed: true},
			"created_at": schema.Int64Attribute{Description: "Creation timestamp in milliseconds since the Unix epoch.", Computed: true},
			"created_by": schema.StringAttribute{Description: "Principal that created the database.", Computed: true},
			"updated_at": schema.Int64Attribute{Description: "Last update timestamp in milliseconds since the Unix epoch.", Computed: true},
			"updated_by": schema.StringAttribute{Description: "Principal that last updated the database.", Computed: true},
		},
	}
}

func (r *databaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientFromProviderData(req.ProviderData, &r.client, &resp.Diagnostics, "paimon_database resource")
}

func (r *databaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	options := mapFromValue(ctx, plan.Options, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateDatabase(ctx, plan.Name.ValueString(), options); err != nil {
		resp.Diagnostics.AddError("Unable to create Paimon database", err.Error())

		return
	}
	r.readIntoState(ctx, &plan, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *databaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	database, err := r.client.GetDatabase(ctx, state.Name.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)

		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Paimon database", err.Error())

		return
	}
	setDatabaseResourceModel(ctx, &state, database, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *databaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan databaseResourceModel
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
	removals, updates := diffOptions(before, after)
	if len(removals) > 0 || len(updates) > 0 {
		if err := r.client.AlterDatabase(ctx, plan.Name.ValueString(), removals, updates); err != nil {
			resp.Diagnostics.AddError("Unable to update Paimon database", err.Error())

			return
		}
	}
	r.readIntoState(ctx, &plan, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *databaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DropDatabase(ctx, state.Name.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to drop Paimon database", err.Error())
	}
}

func (r *databaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *databaseResource) readIntoState(ctx context.Context, model *databaseResourceModel, diags *diag.Diagnostics) {
	database, err := r.client.GetDatabase(ctx, model.Name.ValueString())
	if err != nil {
		diags.AddError("Unable to read Paimon database after change", err.Error())

		return
	}
	setDatabaseResourceModel(ctx, model, database, diags)
}

func setDatabaseResourceModel(ctx context.Context, model *databaseResourceModel, database *client.Database, diags *diag.Diagnostics) {
	model.ID = types.StringValue(database.Name)
	model.CatalogID = types.StringValue(database.ID)
	model.Name = types.StringValue(database.Name)
	model.Options = syncManagedOptions(ctx, model.Options, database.Options, diags)
	model.ServerOptions = stringMapValue(ctx, database.Options, diags)
	model.Location = types.StringValue(database.Location)
	model.Owner = types.StringValue(database.Owner)
	model.CreatedAt = types.Int64Value(database.CreatedAt)
	model.CreatedBy = types.StringValue(database.CreatedBy)
	model.UpdatedAt = types.Int64Value(database.UpdatedAt)
	model.UpdatedBy = types.StringValue(database.UpdatedBy)
}

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
	"testing"

	"github.com/apache/terraform-provider-paimon/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	assert.NotNil(t, New("test")())
}

func TestDiffOptions(t *testing.T) {
	removals, updates := diffOptions(
		map[string]string{"remove": "old", "change": "old", "same": "value"},
		map[string]string{"change": "new", "same": "value", "add": "value"},
	)
	assert.ElementsMatch(t, []string{"remove"}, removals)
	assert.Equal(t, map[string]string{"change": "new", "add": "value"}, updates)
}

func TestSchemasHaveValidFrameworkImplementations(t *testing.T) {
	ctx := context.Background()
	p := &paimonProvider{version: "test"}

	var providerResponse frameworkprovider.SchemaResponse
	p.Schema(ctx, frameworkprovider.SchemaRequest{}, &providerResponse)
	require.False(t, providerResponse.Diagnostics.HasError())
	require.False(t, providerResponse.Schema.ValidateImplementation(ctx).HasError())

	for _, factory := range p.Resources(ctx) {
		var response resource.SchemaResponse
		factory().Schema(ctx, resource.SchemaRequest{}, &response)
		require.False(t, response.Diagnostics.HasError())
		diagnostics := response.Schema.ValidateImplementation(ctx)
		require.False(t, diagnostics.HasError(), diagnostics.Errors())
	}

	for _, factory := range p.DataSources(ctx) {
		var response datasource.SchemaResponse
		factory().Schema(ctx, datasource.SchemaRequest{}, &response)
		require.False(t, response.Diagnostics.HasError())
		diagnostics := response.Schema.ValidateImplementation(ctx)
		require.False(t, diagnostics.HasError(), diagnostics.Errors())
	}
}

func TestProviderDLFCredentialAttributesAreSensitive(t *testing.T) {
	ctx := context.Background()
	p := &paimonProvider{version: "test"}
	var response frameworkprovider.SchemaResponse
	p.Schema(ctx, frameworkprovider.SchemaRequest{}, &response)
	require.False(t, response.Diagnostics.HasError())

	for _, name := range []string{
		"dlf_access_key_id",
		"dlf_access_key_secret",
		"dlf_security_token",
	} {
		attribute, ok := response.Schema.Attributes[name].(providerschema.StringAttribute)
		require.True(t, ok, "attribute %s should be a string", name)
		assert.True(t, attribute.Sensitive, "attribute %s should be sensitive", name)
	}
}

func TestHasDLFConfiguration(t *testing.T) {
	assert.False(t, hasDLFConfiguration(paimonProviderModel{}))
	assert.True(t, hasDLFConfiguration(paimonProviderModel{
		DLFTokenLoader: types.StringValue(client.DLFTokenLoaderECS),
	}))
}

func TestSchemaFromResourceModelNormalizesPrimaryKeyNullability(t *testing.T) {
	ctx := context.Background()
	fields, diagnostics := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: tableFieldAttrTypes()}, []tableFieldModel{
		{
			ID:           types.Int64Unknown(),
			Name:         types.StringValue("id"),
			Type:         types.StringValue("BIGINT"),
			Nullable:     types.BoolUnknown(),
			Description:  types.StringNull(),
			DefaultValue: types.StringNull(),
		},
	})
	require.False(t, diagnostics.HasError(), diagnostics.Errors())
	primaryKeys, diagnostics := types.ListValueFrom(ctx, types.StringType, []string{"id"})
	require.False(t, diagnostics.HasError(), diagnostics.Errors())

	model := tableResourceModel{
		Fields:        fields,
		PartitionKeys: types.ListUnknown(types.StringType),
		PrimaryKeys:   primaryKeys,
		Options:       types.MapNull(types.StringType),
		Comment:       types.StringNull(),
	}
	tableSchema := schemaFromResourceModel(ctx, &model, &diagnostics)
	require.False(t, diagnostics.HasError(), diagnostics.Errors())
	require.Len(t, tableSchema.Fields, 1)
	assert.Equal(t, client.DataType("BIGINT NOT NULL"), tableSchema.Fields[0].Type)
}

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

	"github.com/apache/terraform-provider-paimon/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func clientFromProviderData(data any, target **client.Client, diags *diag.Diagnostics, kind string) {
	if data == nil {
		return
	}
	api, ok := data.(*client.Client)
	if !ok {
		diags.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client while configuring %s, got %T. Please report this issue to the provider developers.", kind, data),
		)

		return
	}
	*target = api
}

func mapFromValue(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	result := make(map[string]string)
	if value.IsNull() || value.IsUnknown() {
		return result
	}
	diags.Append(value.ElementsAs(ctx, &result, false)...)

	return result
}

func stringListFromValue(ctx context.Context, value types.List, diags *diag.Diagnostics) []string {
	result := make([]string, 0)
	if value.IsNull() || value.IsUnknown() {
		return result
	}
	diags.Append(value.ElementsAs(ctx, &result, false)...)

	return result
}

func stringMapValue(ctx context.Context, value map[string]string, diags *diag.Diagnostics) types.Map {
	if value == nil {
		value = map[string]string{}
	}
	result, newDiags := types.MapValueFrom(ctx, types.StringType, value)
	diags.Append(newDiags...)

	return result
}

func stringListValue(ctx context.Context, value []string, diags *diag.Diagnostics) types.List {
	if value == nil {
		value = []string{}
	}
	result, newDiags := types.ListValueFrom(ctx, types.StringType, value)
	diags.Append(newDiags...)

	return result
}

func syncManagedOptions(ctx context.Context, managed types.Map, remote map[string]string, diags *diag.Diagnostics) types.Map {
	if managed.IsNull() || managed.IsUnknown() {
		return managed
	}
	current := mapFromValue(ctx, managed, diags)
	if diags.HasError() {
		return managed
	}
	updated := make(map[string]string)
	for key := range current {
		if value, ok := remote[key]; ok {
			updated[key] = value
		}
	}

	return stringMapValue(ctx, updated, diags)
}

func diffOptions(before, after map[string]string) ([]string, map[string]string) {
	removals := make([]string, 0)
	updates := make(map[string]string)
	for key := range before {
		if _, ok := after[key]; !ok {
			removals = append(removals, key)
		}
	}
	for key, value := range after {
		if previous, ok := before[key]; !ok || previous != value {
			updates[key] = value
		}
	}

	return removals, updates
}

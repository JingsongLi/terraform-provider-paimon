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
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var (
	_ resource.ResourceWithUpgradeState = &databaseResource{}
	_ resource.ResourceWithUpgradeState = &tableResource{}
)

func (r *databaseResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{0: {StateUpgrader: upgradeAPIState(false)}}
}

func (r *tableResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{0: {StateUpgrader: upgradeAPIState(true)}}
}

// Rename version-zero metadata without acquiring ownership of imported keys.
// primary_keys keeps the same stored type; HCL now manages it through options.
func upgradeAPIState(table bool) func(context.Context, resource.UpgradeStateRequest, *resource.UpgradeStateResponse) {
	return func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
		var values map[string]json.RawMessage
		if req.RawState == nil || json.Unmarshal(req.RawState.JSON, &values) != nil || values == nil {
			resp.Diagnostics.AddError("Unable to upgrade Paimon state", "Expected a JSON resource state object. The previous state has not been changed.")

			return
		}
		renames := map[string]string{"catalog_id": "server_id"}
		if table {
			renames["allow_destructive_changes"] = "allow_replacement"
		}
		for oldName, newName := range renames {
			if value, exists := values[oldName]; exists {
				if _, duplicate := values[newName]; duplicate {
					resp.Diagnostics.AddError("Unable to upgrade Paimon state", "State contains both "+oldName+" and "+newName+"; resolve the conflicting attributes before upgrading.")

					return
				}
				values[newName] = value
				delete(values, oldName)
			}
		}
		if table {
			if _, exists := values["allow_replacement"]; !exists {
				// Version-zero snapshots can predate the replacement guard.
				values["allow_replacement"] = json.RawMessage("false")
			}
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			resp.Diagnostics.AddError("Unable to upgrade Paimon state", "Could not encode the upgraded state.")

			return
		}
		raw := tfprotov6.RawState{JSON: encoded}
		typ := resp.State.Schema.Type().TerraformType(ctx)
		value, err := raw.Unmarshal(typ)
		if err != nil {
			resp.Diagnostics.AddError("Unable to upgrade Paimon state", "The upgraded attributes do not match the current schema. The previous state has not been changed.")

			return
		}
		dynamic, err := tfprotov6.NewDynamicValue(typ, value)
		if err != nil {
			resp.Diagnostics.AddError("Unable to upgrade Paimon state", "Could not encode the current resource state.")

			return
		}
		resp.DynamicValue = &dynamic
	}
}

<!--
  Licensed to the Apache Software Foundation (ASF) under one
  or more contributor license agreements. See the NOTICE file
  distributed with this work for additional information
  regarding copyright ownership. The ASF licenses this file
  to you under the Apache License, Version 2.0 (the
  "License"); you may not use this file except in compliance
  with the License. You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing,
  software distributed under the License is distributed on an
  "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
  KIND, either express or implied. See the License for the
  specific language governing permissions and limitations
  under the License.
-->

# API contract and migration

The core Terraform API consists of `paimon_database`, `paimon_table`, and their
single-object data sources. Keep the ordered `fields` list, string-valued
`options`, explicit `partition_keys`, and computed server metadata. Primary
keys are configured through Paimon's Java `primary-key` option; `primary_keys`
is the normalized read-only result. Terraform `id` identifies the managed
object; `server_id` is the database/table UUID returned by the server, not a
Catalog identifier. `options` manages declared keys and preserves unmanaged
keys; `server_options` exposes the raw server map.

## Reference contracts

The review used these exact source revisions. They are source compatibility
references, not a minimum version or certification for deployed servers.

| Reference | Contract used |
| --- | --- |
| [Paimon 2.0 final, 604e6d5](https://github.com/apache/paimon/tree/604e6d5e131c74a8d127333a2a3ad6d0319732bf) | Schema primary-key normalization, field identity, SchemaChange and constant default conversion |
| [Paimon main, 475be56](https://github.com/apache/paimon/tree/475be566fef490ad147deec4ee15344c25c0352d) | The same core contract plus experimental permission/policy models and reference REST server policy canonicalization |
| [terraform-provider-iceberg, c7d15b4](https://github.com/apache/terraform-provider-iceberg/tree/c7d15b42799dd116ba51cf6ed9c43997f41ad2b2) | Comparison of Terraform resource/data-source boundaries, schema evolution and ownership; Iceberg-specific partition transforms are not Paimon partition keys |

Core names and ownership behavior can be maintained as the stable API baseline.
Future list data sources, nested schema evolution and additional Paimon objects
can be additive. Table identity changes currently require replacement;
`allow_replacement = true` explicitly permits plans that can delete data.
This setting does not protect explicit destroy or resource removal. Supported
in-place changes remain governed by the deployed Catalog's validation.

`paimon_permission`, `paimon_row_filter`, and `paimon_column_mask` remain
experimental because the Java management APIs and serialized Predicate/Transform
AST are experimental. Reference policy canonicalization is implemented in the
Java test REST server; it is not proof that every external REST/DLF deployment
behaves identically. Pin the provider and server revisions, run the real catalog
suite, and validate query enforcement before accepting a deployment. Content
changes to filters and masks remain non-atomic and require a maintenance window
with `allow_non_atomic_update = true`.

## Migrating pre-stable HCL

| Previous configuration/reference | Current configuration/reference |
| --- | --- |
| `primary_keys = ["id", "tenant"]` | `options = { "primary-key" = "id,tenant" }` (merge into existing options) |
| `.catalog_id` on resources or data sources | `.server_id` |
| `allow_destructive_changes = true` | `allow_replacement = true` |

Resource schema version 1 automatically renames version-0 state attributes
`catalog_id` and `allow_destructive_changes`. Field IDs, normalized keys, and
managed options are preserved; no remote mutation happens during migration.
HCL files and output references must still be updated. Data sources are read
again using the new schema. State cannot tell whether old optional/computed
primary keys were explicitly configured or only observed during import, so the
migration does not silently add managed options. Move any intended key
configuration into `options` yourself; adopting matching keys causes no ALTER
or replacement.

Before upgrading, retain a state backup and inspect the new plan. An omitted
unmanaged primary key remains inherited. Once `options["primary-key"]` is
managed, removing it requests no primary keys and is protected by the default
replacement guard. Do not approve a replacement merely to complete migration.
The HCL changes above are intentionally completed before a stable API release.

See [production validation](production-readiness.md) for CLI, deployed-service,
query-engine and signed Registry checks that source review cannot establish.

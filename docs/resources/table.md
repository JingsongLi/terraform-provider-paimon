---
page_title: "paimon_table Resource - Paimon"
subcategory: ""
description: |-
  Creates, evolves, and manages a table in an Apache Paimon REST Catalog.
---

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

# `paimon_table` resource

Creates and manages a Paimon table.

```hcl
resource "paimon_table" "example" {
  database = "analytics"
  name     = "events"

  fields = [
    {
      name     = "id"
      type     = "BIGINT"
      nullable = false
    },
    {
      name = "payload"
      type = "STRING"
    }
  ]

  primary_keys = ["id"]
  options = {
    bucket = "4"
  }
}
```

Each field supports `id`, `name`, `type`, `nullable`, `description`, and
`default_value`. `nested_field_ids` is a computed map that preserves the stable
IDs of nested ROW fields. Top-level field IDs must be unique integers from 0
through 1073741822; the next available ID is assigned when omitted. Use
canonical Paimon SQL type strings such as `INT`, `BIGINT`, `STRING`,
`DECIMAL(12, 2)`, `ARRAY<STRING>`, or `ROW<item STRING>`.

Supported top-level field additions, drops, renames, atomic type/nullability
changes, comments, defaults, and position changes use Paimon's in-place
`SchemaChange` API. Stable field IDs distinguish a rename from a drop plus add.
Rename cycles require an intermediate name. Adding a non-nullable field,
changing the type of a retained partition/primary-key field, or changing an
existing composite `ROW`, `ARRAY`, `MAP`, `MULTISET`, or `VECTOR` shape replaces
the table because Paimon's schema-evolution API cannot safely apply those
changes in place. `database`, `name`, `partition_keys`, and `primary_keys` also
remain replacement attributes.
Configure keys with `primary_keys` and `partition_keys`; the
normalized Paimon options `primary-key` and `partition` are rejected in
`options` because the server removes them from its options map.

Mutable `options` and `comment` update in place. Changing or removing an option
that Paimon defines as immutable, such as `merge-engine`, `bucket-key`, `type`,
or `primary-key.nullable`, replaces the table. Unmanaged server options are
preserved and exposed through `server_options`.

Dropping a managed table can delete its data. Use `prevent_destroy` where
appropriate:

```hcl
lifecycle {
  prevent_destroy = true
}
```

Import with the unambiguous URL-query identity. The legacy `database.table`
form remains accepted when names do not contain dots:

```bash
terraform import paimon_table.example 'database=analytics&table=events'
```

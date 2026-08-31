---
page_title: "paimon_permission Resource - Paimon"
subcategory: ""
description: |-
  Manages a direct Apache Paimon catalog permission assignment.
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

# `paimon_permission` resource

Manages one direct permission assignment. Paimon's REST management API is
experimental, and the configured principal must already exist in the server's
principal namespace.

```hcl
resource "paimon_permission" "read_events" {
  resource_type = "TABLE"
  database      = "analytics"
  table         = "events"
  access        = "SELECT"
  principal     = "role:analyst"
  expire_time   = "2026-12-31T23:59:59Z"
}
```

The assignment identity is `resource_type` plus its locator, `access`, and
`principal`. Changing an identity attribute replaces the resource. Changing
`expire_time`, `column_names`, or `excluded_column_names` uses Paimon's
grant-or-replace operation.

Resource locators have these exact shapes:

| `resource_type` | Locator attributes |
| --- | --- |
| `CATALOG`, `CATALOG_ALL` | none |
| `DATABASE`, `DATABASE_ALL` | `database` |
| `TABLE`, `COLUMN` | `database`, `table` |
| `FUNCTION` | `database`, `function` |
| `VIEW` | `database`, `view` |

Supported accesses follow the server contract:

| Resource type | Accesses |
| --- | --- |
| `CATALOG` | `ALL`, `ALTER`, `DROP`, `GRANT`, `CREATEDATABASE` |
| `CATALOG_ALL` | `ALL`, `DESCRIBE`, `ALTER`, `DROP`, `GRANT`, `CREATETABLE`, `CREATEVIEW`, `CREATEFUNCTION`, `LIST`, `SELECT`, `UPDATE` |
| `DATABASE` | `ALL`, `DESCRIBE`, `ALTER`, `DROP`, `GRANT`, `CREATETABLE`, `CREATEVIEW`, `CREATEFUNCTION`, `LIST` |
| `DATABASE_ALL` | `ALL`, `SELECT`, `UPDATE`, `ALTER`, `DROP`, `GRANT` |
| `TABLE` | `ALL`, `SELECT`, `UPDATE`, `ALTER`, `DROP`, `GRANT` |
| `COLUMN` | `SELECT` |
| `VIEW`, `FUNCTION` | `ALL`, `SELECT`, `ALTER`, `DROP`, `GRANT` |

A `COLUMN` assignment must set exactly one non-empty column range:

```hcl
resource "paimon_permission" "limited_event_columns" {
  resource_type = "COLUMN"
  database      = "analytics"
  table         = "events"
  access        = "SELECT"
  principal     = "role:analyst"
  column_names  = ["event_id", "event_time"]
}
```

`column_names` is an allowlist and denies columns added later.
`excluded_column_names` is a denylist and allows columns added later. The table
must enable `query-auth.enabled=true` before a column assignment is granted.

`expire_time` is optional. It must be a UTC ISO-8601 instant with a `Z` suffix.
Fractional seconds may contain up to nine digits, but the parsed value must
resolve exactly to milliseconds: `.123000Z` is valid and `.123456Z` is not.
Equivalent spellings are normalized to upper-case `T`/`Z` with no more than
three fractional digits before they are sent to the server. Expiry is an
exclusive upper bound evaluated by the server clock.

Import with the URL-query identity printed in `id`:

```bash
terraform import paimon_permission.read_events \
  'resource_type=TABLE&database=analytics&table=events&access=SELECT&principal=role%3Aanalyst'
```

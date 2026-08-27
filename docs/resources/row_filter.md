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

# `paimon_row_filter` resource

Manages the one row filter a principal may have on a table. A data policy only
restricts an already-authorized read; it does not grant `SELECT`. The table must
exist with `query-auth.enabled=true`, and the principal must exist on the
server.

```hcl
resource "paimon_row_filter" "analyst_region" {
  database  = "analytics"
  table     = "events"
  principal = "role:analyst"
  predicate = jsonencode({
    kind = "LEAF"
    transform = {
      name = "FIELD_REF"
      fieldRef = {
        index = 0
        name  = "region"
        type  = "STRING"
      }
    }
    function = "EQUAL"
    literals = ["APAC"]
  })
}
```

`predicate` must be the JSON serialization of one Paimon `Predicate` and must
not exceed 60 KiB in UTF-8. The server validates field references and returns a
canonical representation. JSON formatting and object-key ordering alone do not
replace the remote policy; an in-place apply only updates the stored
representation.

Paimon currently has create and drop operations but no policy update operation.
Changing `predicate` therefore drops and recreates the policy. If creation
fails, the provider attempts to restore the previous policy and reports whether
restoration succeeded. There can still be a brief interval without the filter;
schedule sensitive changes accordingly.

Import with the URL-query identity printed in `id`:

```bash
terraform import paimon_row_filter.analyst_region \
  'database=analytics&table=events&principal=role%3Aanalyst'
```

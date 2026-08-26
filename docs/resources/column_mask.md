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

# `paimon_column_mask` resource

Manages the one mask a principal may have on a table column. A data policy only
restricts an already-authorized read; it does not grant `SELECT`. The table must
exist with `query-auth.enabled=true`, and the principal must exist on the
server.

```hcl
resource "paimon_column_mask" "analyst_email" {
  database  = "analytics"
  table     = "events"
  principal = "role:analyst"
  column    = "email"
  transform = jsonencode({
    name = "CONCAT"
    inputs = ["***@***.com"]
  })
}
```

This example replaces every email value with the constant `***@***.com`; it
does not include the original column value in the transform output.

`transform` must be the JSON serialization of one Paimon `Transform` and must
not exceed 60 KiB in UTF-8. The server validates field references and the
result type against the table schema, then returns a canonical representation.
JSON formatting and object-key ordering alone do not replace the remote policy;
an in-place apply only updates the stored representation.

Paimon currently has create and drop operations but no policy update operation.
Changing `transform` therefore drops and recreates the policy. If creation
fails, the provider attempts to restore the previous policy and reports whether
restoration succeeded. There can still be a brief interval without the mask;
schedule sensitive changes accordingly.

Import with the URL-query identity printed in `id`:

```bash
terraform import paimon_column_mask.analyst_email \
  'database=analytics&table=events&principal=role%3Aanalyst&column=email'
```

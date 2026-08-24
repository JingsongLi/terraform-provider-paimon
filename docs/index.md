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

# Paimon provider

The Paimon provider manages catalog metadata through the Apache Paimon REST
Catalog API.

## Configuration

```hcl
provider "paimon" {
  uri       = "https://catalog.example.com"
  warehouse = "production"

  token_provider = "bear"
  token          = var.paimon_token
}
```

### Arguments

- `uri` (required): REST Catalog base URI. A base path is supported.
- `warehouse` (optional): sent as the `warehouse` query parameter to
  `/v1/config`.
- `token_provider` (optional): `bear` for Bearer authentication or `dlf` for
  Alibaba Cloud DLF AK/STS signing. It is inferred when omitted.
- `token` (optional, sensitive): token used by the `bear` provider.

The provider first calls `/v1/config`, merges server defaults, client values,
and server overrides in that order, and then uses the resulting `prefix` for
catalog operations.

## Resources and data sources

- [Database resource](resources/database.md)
- [Table resource](resources/table.md)
- [Database data source](data-sources/database.md)
- [Table data source](data-sources/table.md)

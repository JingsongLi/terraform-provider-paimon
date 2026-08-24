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

# Paimon Terraform Provider

This project integrates Terraform and OpenTofu with an Apache Paimon REST
Catalog. It follows the provider shape established by
[`apache/terraform-provider-iceberg`](https://github.com/apache/terraform-provider-iceberg),
but talks directly to Paimon's language-neutral REST Catalog API instead of
embedding a JVM client.

> This repository is an initial implementation. The `apache/paimon` Terraform
> Registry address becomes installable after the first provider release is
> published and registered.

## Supported objects

Resources:

- `paimon_database` creates, reads, updates, imports, and drops databases.
- `paimon_table` creates, reads, imports, updates options/comments, and drops
  managed tables.

Data sources:

- `paimon_database` reads a database and its server metadata.
- `paimon_table` reads a table schema, keys, options, and server metadata.

## Example

```hcl
terraform {
  required_providers {
    paimon = {
      source = "apache/paimon"
    }
  }
}

provider "paimon" {
  uri       = "http://localhost:8080"
  warehouse = "default"

  token_provider = "bear"
  token          = var.paimon_token
}

resource "paimon_database" "analytics" {
  name = "analytics"
  options = {
    owner = "data-platform"
  }
}

resource "paimon_table" "events" {
  database = paimon_database.analytics.name
  name     = "events"

  fields = [
    {
      name     = "event_id"
      type     = "BIGINT"
      nullable = false
    },
    {
      name = "event_time"
      type = "TIMESTAMP(3)"
    },
    {
      name = "payload"
      type = "STRING"
    }
  ]

  primary_keys   = ["event_id"]
  partition_keys = []
  options = {
    "bucket" = "4"
  }
  comment = "Events managed by Terraform"
}
```

## Lifecycle and safety

Only the REST metastore is in scope. Filesystem, Hive, and JDBC catalogs are
not accessed directly because Terraform needs a stable remote control-plane
contract; Paimon's REST OpenAPI provides that contract.

## Import

```bash
terraform import paimon_database.analytics analytics
terraform import paimon_table.events analytics.events
```

## Development

Go 1.25 or newer is required.

```bash
make fmt
make test
make build
```

See [`docs/index.md`](docs/index.md) for the full provider configuration and
resource notes. The source comparison and implementation tradeoffs are recorded
in [`docs/design.md`](docs/design.md).

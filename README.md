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

Provider capabilities:

- Paimon `/v1/config` discovery and server-supplied catalog prefix
- optional warehouse selection
- Bearer token authentication
- Alibaba Cloud DLF AK/STS signing with default and OpenAPI algorithms
- automatic STS refresh from a rotating local token file or ECS RAM role
- custom HTTP headers
- preservation of server options not managed by Terraform
- nested Paimon type decoding from the REST wire format

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

For an Alibaba Cloud DLF REST endpoint, static AK/STS authentication can be
configured as follows. Both `/v1/config` and catalog operations are signed.

```hcl
provider "paimon" {
  uri            = "https://dlf.cn-hangzhou.aliyuncs.com"
  token_provider = "dlf"

  dlf_access_key_id     = var.dlf_access_key_id
  dlf_access_key_secret = var.dlf_access_key_secret
  dlf_security_token    = var.dlf_security_token
}
```

For renewable STS credentials, use a rotating JSON token file or an ECS RAM
role instead of static values. The provider reloads dynamic credentials when
they have less than one hour remaining:

```hcl
provider "paimon" {
  uri              = "https://dlf.cn-hangzhou.aliyuncs.com"
  token_provider   = "dlf"
  dlf_token_loader = "local_file"
  dlf_token_path   = "/run/secrets/dlf-sts.json"
}
```

See [`docs/index.md`](docs/index.md) for the token JSON format, ECS setup, and
signing-algorithm selection.

Paimon primary key fields are non-null by default. To use nullable primary
keys, set `options["primary-key.nullable"] = "true"` and set the matching
field's `nullable` attribute to `true`.

## Lifecycle and safety

In this initial version, `fields`, `partition_keys`, and `primary_keys` are
immutable Terraform attributes. Changing one produces a table replacement.
Paimon's managed-table drop operation can delete table data, so inspect plans
carefully and use Terraform's `prevent_destroy` lifecycle rule for important
tables. Table `options` and `comment` update in place through Paimon
`SchemaChange` requests.

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

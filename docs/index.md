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
  headers        = {
    "X-Tenant" = "analytics"
  }
}
```

### Arguments

- `uri` (required): REST Catalog base URI. A base path is supported.
- `warehouse` (optional): sent as the `warehouse` query parameter to
  `/v1/config`.
- `token_provider` (optional): `bear` for Bearer authentication or `dlf` for
  Alibaba Cloud DLF AK/STS signing. It is inferred when omitted.
- `token` (optional, sensitive): token used by the `bear` provider.
- `dlf_region` (optional): region used by the default DLF signer. Standard DLF
  endpoint hostnames are parsed when this is omitted.
- `dlf_signing_algorithm` (optional): `default` or `openapi`. Endpoints whose
  hostname contains `dlfnext` select `openapi`; all others select `default`.
- `dlf_access_key_id` and `dlf_access_key_secret` (optional, sensitive): a
  static Alibaba Cloud access key pair.
- `dlf_security_token` (optional, sensitive): STS token used with the static
  access key pair.
- `dlf_token_loader` (optional): dynamic credential source, either
  `local_file` or `ecs`.
- `dlf_token_path` (optional): rotating AK/STS JSON file. Setting a path
  implies the `local_file` loader.
- `dlf_ecs_metadata_url` (optional): ECS metadata endpoint override. The
  default is
  `http://100.100.100.200/latest/meta-data/Ram/security-credentials/`.
- `dlf_ecs_role_name` (optional): RAM role name. The provider discovers it
  from the ECS metadata endpoint when omitted.
- `prefix` (optional): client catalog prefix. A prefix in the server's config
  `overrides` takes precedence.
- `headers` (optional, sensitive): additional request headers.

The provider first calls `/v1/config`, merges server defaults, client values,
and server overrides in that order, and then uses the resulting `prefix` for
catalog operations.

## DLF AK/STS authentication

Exactly one DLF credential source must be configured: static AK/STS, a local
token file, or ECS metadata. DLF authentication signs the initial
`/v1/config` request as well as every subsequent catalog request. Generated
signature headers take precedence over entries with the same names in
`headers`.

### Static AK or STS

```hcl
provider "paimon" {
  uri            = "https://dlf.cn-hangzhou.aliyuncs.com"
  token_provider = "dlf"

  dlf_access_key_id     = var.dlf_access_key_id
  dlf_access_key_secret = var.dlf_access_key_secret
  dlf_security_token    = var.dlf_security_token # omit for long-lived AK
}
```

Static credentials are not refreshed. Use a dynamic source for renewable STS
credentials.

### Rotating local token file

```hcl
provider "paimon" {
  uri              = "https://dlf.cn-hangzhou.aliyuncs.com"
  token_provider   = "dlf"
  dlf_token_loader = "local_file"
  dlf_token_path   = "/run/secrets/dlf-sts.json"
}
```

The file must contain the same field names as Paimon's `DLFToken` JSON:

```json
{
  "AccessKeyId": "STS....",
  "AccessKeySecret": "...",
  "SecurityToken": "...",
  "Expiration": "2026-08-20T12:00:00Z"
}
```

Replace the file atomically and restrict its permissions. The provider caches
the parsed token and reloads the file before a request when the credential has
less than one hour remaining. A read or parse failure is retried up to five
times; token contents are never included in the resulting error.

### ECS RAM role

```hcl
provider "paimon" {
  uri              = "https://dlf.cn-hangzhou.aliyuncs.com"
  token_provider   = "dlf"
  dlf_token_loader = "ecs"

  # Optional. Otherwise it is discovered from ECS metadata.
  dlf_ecs_role_name = "paimon-terraform-role"
}
```

The ECS loader obtains temporary credentials from the instance metadata
service and refreshes them using the same one-hour safety window. Use
`dlf_ecs_metadata_url` only for a compatible metadata service or local testing.

## Resources and data sources

- [Database resource](resources/database.md)
- [Table resource](resources/table.md)
- [Database data source](data-sources/database.md)
- [Table data source](data-sources/table.md)

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

# Design assessment

This implementation was designed after reviewing Apache
`terraform-provider-iceberg` main at commit
`186d4f48b9e98386b044e1367d6fdf8d47a3ff8d` and Apache Paimon master at commit
`d187f675d04ce5924beee04d0ba9d821dca79de1` on 2026-08-20.

## What transfers from the Iceberg provider

- Terraform Plugin Framework provider/resource/data-source organization
- lazy catalog initialization so `terraform validate` does not require a live
  service
- separate user-managed and server-returned property maps
- stable import identifiers and removal from state after a REST 404
- explicit lifecycle handling for catalog objects

## What must be Paimon-specific

The Iceberg provider delegates catalog semantics to `iceberg-go`. Paimon does
not currently provide an equivalent official Go catalog SDK, but it does
publish a language-neutral OpenAPI contract. This provider therefore contains
a small Go client for the catalog-control endpoints rather than depending on
Iceberg types or running Java in the provider process.

Paimon databases are single names rather than Iceberg's segmented namespaces.
Paimon tables also expose primary keys, partition keys, schema options, table
comments, audit metadata, and Paimon `SchemaChange` actions. Iceberg partition
transforms and sort orders do not map directly and were not copied.

## Initial scope

The first implementation supports:

- REST config handshake and catalog prefix discovery
- database CRUD/read/import and partial option ownership
- table CRUD/read/import
- in-place table option and comment updates
- primitive and structured Paimon type decoding
- Bearer authentication and static custom headers
- DLF default and OpenAPI request signing
- static DLF AK/STS and refreshable local-file/ECS credential sources

The following are intentionally deferred:

- in-place column evolution (add/drop/rename/type/nullability/position)
- views, functions, branches, tags, partitions, snapshots, and consumers
- acceptance tests against a packaged REST Catalog server
- Terraform Registry release automation and generated documentation

## DLF compatibility

The DLF implementation follows Paimon's `DLFAuthProvider`,
`DLFDefaultSigner`, and `DLFOpenApiSigner` contracts. The default signer uses
the `DLF4-HMAC-SHA256` credential scope and the OpenAPI signer uses Alibaba
Cloud's ROA `acs` HMAC-SHA1 authorization format. Paths use Java
`URLEncoder`-compatible encoding because the exact encoded Paimon REST
resource path participates in default signing.

Dynamic credentials are loaded lazily and cached. A credential is refreshed
when its `Expiration` is less than one hour away, matching Paimon's safety
window. The provider serializes refreshes so concurrent Terraform operations
do not race to replace the cached token. Static AK/STS values have no
expiration metadata and therefore are never refreshed.

## Why schema changes replace in v1

Paimon's REST API exposes granular `SchemaChange` actions, but primary-key and
partition-key evolution is not symmetric with initial creation, and correct
column matching depends on stable field IDs and nested-field semantics. The
initial provider marks fields and key lists as replacement attributes rather
than emitting a partial or ambiguous migration. This is safe from silent
metadata corruption but can be destructive because dropping a managed table
may remove data. A follow-up should add tested, ID-based in-place column
evolution and keep replacement only for unsupported key changes.

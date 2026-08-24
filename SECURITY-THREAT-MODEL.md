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

# Apache Paimon Terraform Provider Security Threat Model

This document describes the security boundaries of the Apache Paimon
Terraform provider. It guides maintainers and automated security triage; it is
not a complete threat model for every Terraform deployment using the provider.

## Scope and roles

The provider is a control-plane client used by a trusted Terraform operator.
It communicates with a configured Paimon REST Catalog and may authenticate with
a bearer token or Alibaba Cloud DLF AK/STS credentials.

The relevant roles are:

- the operator, who selects endpoints, credentials, Terraform state backends,
  and resources to manage;
- the REST Catalog control plane, which is trusted to enforce server-side
  authorization and return valid catalog configuration;
- the provider instance, which owns request construction, signing, credential
  refresh, and Terraform state mapping;
- the surrounding Terraform runtime and execution environment, which are
  outside this repository's primary security boundary.

## Security goals

The provider should:

- avoid exposing bearer tokens, access key secrets, STS tokens, token-file
  contents, or metadata-service responses through logs, diagnostics, or state;
- keep authentication and cached credentials isolated between provider
  instances;
- sign the exact REST method, resource path, query, body, and required headers;
- refresh temporary credentials without races or partial credential reuse;
- bound responses read from REST and credential endpoints.

## Trusted operator configuration

Provider configuration is trusted operator input. This includes the REST URI,
custom headers, token-file path, DLF region, signing algorithm, and optional ECS
metadata endpoint override. Reports that require an attacker to control these
values directly are generally deployment or correctness issues, unless the
provider leaks secrets to a new audience or crosses a provider-owned boundary.

The default ECS metadata endpoint is link-local. An operator-supplied metadata
URL is intentionally supported for compatible services and testing and is not
an SSRF boundary by itself.

## In-scope security issues

Examples include:

- credentials or token-file contents appearing in errors, logs, Terraform
  state, or user-visible outputs unexpectedly;
- authentication state from one provider instance being reused by another;
- concurrent refresh publishing mixed AK, secret, and STS values;
- request signing that can be bypassed or that sends credentials to a different
  host than the configured catalog;
- unbounded provider-owned reads that enable practical memory exhaustion.

## Usually out of scope or non-security by default

The following may still be correctness or hardening bugs:

- unsafe Terraform state backend or execution-environment configuration;
- an operator intentionally configuring a malicious REST or metadata endpoint;
- authorization decisions owned by the REST Catalog or Alibaba Cloud RAM;
- resource drift or plan/apply inconsistencies without a trust-boundary breach;
- a principal obtaining an outcome it could already achieve through legitimate
  provider configuration with the same privileges.

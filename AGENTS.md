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

# Apache Paimon Terraform Provider Agent Instructions

## Build and test

Run `make check` before submitting changes. Keep tests focused on observable
provider and REST Catalog behavior. DLF signing changes must include stable
signature vectors and credential-refresh coverage.

## Security model

Use [`SECURITY-THREAT-MODEL.md`](SECURITY-THREAT-MODEL.md) when assessing
security findings. Never include bearer tokens, access key secrets, STS tokens,
token-file contents, or metadata-service credential responses in errors or
logs.

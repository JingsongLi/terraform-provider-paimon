#!/usr/bin/env bash

# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements. See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

provider_namespace="apache"
provider_type="paimon"
provider_version="${VERSION:-0.0.0}"
binary_name="terraform-provider-paimon_v${provider_version}"

provider_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
provider_arch="$(uname -m)"
case "${provider_arch}" in
  x86_64) provider_arch="amd64" ;;
  arm64|aarch64) provider_arch="arm64" ;;
esac

mirror_root="$(pwd)/terraform-plugins"
install_path="${mirror_root}/registry.terraform.io/${provider_namespace}/${provider_type}/${provider_version}/${provider_os}_${provider_arch}"

go build -o "${binary_name}" .
mkdir -p "${install_path}"
mv "${binary_name}" "${install_path}/"
chmod +x "${install_path}/${binary_name}"

printf 'Provider installed in the local mirror:\n  %s\n\n' "${install_path}"
printf 'Configure Terraform with:\n\n'
printf 'provider_installation {\n'
printf '  filesystem_mirror {\n'
printf '    path    = "%s"\n' "${mirror_root}"
printf '    include = ["registry.terraform.io/%s/%s"]\n' "${provider_namespace}" "${provider_type}"
printf '  }\n'
printf '  direct {\n'
printf '    exclude = ["registry.terraform.io/%s/%s"]\n' "${provider_namespace}" "${provider_type}"
printf '  }\n'
printf '}\n'

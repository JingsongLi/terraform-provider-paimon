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

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d -t paimon-provider-examples.XXXXXX)"
trap 'rm -rf "${work_dir}"' EXIT

provider_version="0.1.0"
provider_os="$(go env GOOS)"
provider_arch="$(go env GOARCH)"
mirror_root="${work_dir}/plugins"
install_dir="${mirror_root}/registry.terraform.io/apache/paimon/${provider_version}/${provider_os}_${provider_arch}"
mkdir -p "${install_dir}"
go build -trimpath -ldflags "-s -w -X main.version=${provider_version}" \
  -o "${install_dir}/terraform-provider-paimon_v${provider_version}" "${repo_root}"

index=0
while IFS= read -r example_dir; do
  validation_dir="${work_dir}/example-${index}"
  mkdir "${validation_dir}"
  cp "${example_dir}"/*.tf "${validation_dir}/"
  terraform -chdir="${validation_dir}" init -backend=false -input=false -no-color \
    -plugin-dir="${mirror_root}" >/dev/null
  terraform -chdir="${validation_dir}" validate -no-color
  index=$((index + 1))
done < <(find "${repo_root}/examples" -type f -name '*.tf' -exec dirname {} \; | LC_ALL=C sort -u)

if [[ ${index} -eq 0 ]]; then
  echo "no Terraform examples found" >&2
  exit 1
fi

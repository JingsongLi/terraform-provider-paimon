#!/usr/bin/env bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements. See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

if [[ $# -ne 2 || ! "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ || ! "$2" =~ ^[1-9][0-9]*$ ]]; then
  echo "usage: $0 <version> <rc>  (example: $0 0.1.0 1)" >&2
  exit 2
fi
version="$1"
rc="$2"
release_id="paimon-terraform-${version}"
rc_id="${release_id}-rc${rc}"
source_archive="apache-paimon-terraform-${version}-src.tgz"
source_directory="apache-paimon-terraform-${version}"
repository="apache/terraform-provider-paimon"
binary_sums="terraform-provider-paimon_${version}_SHA256SUMS"

work_dir="$(mktemp -d -t paimon-provider-verify.XXXXXX)"
trap 'rm -rf "${work_dir}"' EXIT
cd "${work_dir}"
curl -fsSLO https://downloads.apache.org/paimon/KEYS
gpg --import KEYS
for suffix in "" .asc .sha512; do
	curl -fsSLO "https://dist.apache.org/repos/dist/dev/paimon/${rc_id}/${source_archive}${suffix}"
done
gpg --verify "${source_archive}.asc" "${source_archive}"
shasum -a 512 -c "${source_archive}.sha512"

mkdir binaries
gh release download "v${version}-rc${rc}" --repo "${repository}" --dir binaries \
  --pattern 'terraform-provider-paimon_*'
(
  cd binaries
  gpg --verify "${binary_sums}.sig" "${binary_sums}"
  shasum -a 256 -c "${binary_sums}"
)

tar -xzf "${source_archive}"
cd "${source_directory}"
dev/check-license
./dev/update_licenses.sh --check
make check
make test-acceptance
echo "Release candidate ${version} RC${rc} verified successfully."

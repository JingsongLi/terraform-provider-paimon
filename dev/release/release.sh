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
tag="v${version}"
rc_tag="${tag}-rc${rc}"
repository="apache/terraform-provider-paimon"
release_id="paimon-terraform-${version}"
dist_dev="https://dist.apache.org/repos/dist/dev/paimon"
dist_release="https://dist.apache.org/repos/dist/release/paimon"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "final releases must be published from a clean checkout" >&2
  exit 1
fi
rc_commit="$(git rev-parse "${rc_tag}^{commit}")"
git tag -a "${tag}" "${rc_commit}" -m "Apache Paimon Terraform provider ${version}"
git push origin "${tag}"

svn mv "${dist_dev}/${release_id}-rc${rc}" "${dist_release}/${release_id}" \
  -m "Apache Paimon Terraform provider ${version}"

work_dir="$(mktemp -d -t paimon-provider-release.XXXXXX)"
trap 'rm -rf "${work_dir}"' EXIT
svn export "${dist_release}/${release_id}" "${work_dir}/source"
gh release download "${rc_tag}" --repo "${repository}" --dir "${work_dir}/assets" \
  --pattern 'terraform-provider-paimon_*'
cp "${work_dir}/source/"* "${work_dir}/assets/"
gh release create "${tag}" --repo "${repository}" --title "Apache Paimon Terraform provider ${version}" \
  --generate-notes --verify-tag "${work_dir}/assets/"*

echo "Published ${tag}: https://github.com/${repository}/releases/tag/${tag}"
echo "Remember to record the release at https://reporter.apache.org/addrelease.html?paimon"

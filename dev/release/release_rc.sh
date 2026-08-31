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
tag="v${version}-rc${rc}"
repository="apache/terraform-provider-paimon"
source_archive="apache-paimon-terraform-${version}-src.tgz"
binary_sums="terraform-provider-paimon_${version}_SHA256SUMS"
staging_name="paimon-terraform-${version}-rc${rc}"

: "${RELEASE_DEFAULT:=1}"
: "${RELEASE_PUSH_TAG:=${RELEASE_DEFAULT}}"
: "${RELEASE_SIGN:=${RELEASE_DEFAULT}}"
: "${RELEASE_UPLOAD:=${RELEASE_DEFAULT}}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "release candidates must be cut from a clean checkout" >&2
  exit 1
fi
./dev/update_licenses.sh --check
make check
make check-license
make test-acceptance

work_dir="$(mktemp -d -t paimon-provider-rc.XXXXXX)"
trap 'rm -rf "${work_dir}"' EXIT
staging="${work_dir}/stage"

if [[ ${RELEASE_PUSH_TAG} -gt 0 ]]; then
  git tag -a "${tag}" -m "Apache Paimon Terraform provider ${version} RC${rc}"
  git push origin "${tag}"
fi
rc_hash="$(git rev-list --max-count=1 "${tag}")"

if [[ ${RELEASE_SIGN} -gt 0 ]]; then
  run_id=""
  for _ in $(seq 1 60); do
    run_id="$(gh run list --repo "${repository}" --workflow=tf-release.yml --json databaseId,event,headBranch --jq ".[] | select(.event == \"push\" and .headBranch == \"${tag}\") | .databaseId" | head -1)"
    [[ -n "${run_id}" ]] && break
    sleep 5
  done
  [[ -n "${run_id}" ]] || { echo "timed out waiting for tf-release.yml" >&2; exit 1; }
  gh run watch --repo "${repository}" --exit-status "${run_id}"

  mkdir "${staging}"
  gh release download "${tag}" --repo "${repository}" --dir "${staging}" \
    --pattern "${source_archive}" --pattern "${source_archive}.sha512" --pattern "${binary_sums}"
  (
    cd "${staging}"
    gpg --armor --detach-sign "${source_archive}"
    gpg --batch --yes --detach-sign --output "${binary_sums}.sig" "${binary_sums}"
  )
  gh release upload "${tag}" --repo "${repository}" --clobber \
    "${staging}/${source_archive}.asc" "${staging}/${binary_sums}.sig"
fi

if [[ ${RELEASE_UPLOAD} -gt 0 ]]; then
  [[ -d "${staging}" ]] || { echo "missing signed staging directory ${staging}" >&2; exit 1; }
  rm -f "${staging}/${binary_sums}" "${staging}/${binary_sums}.sig"
  svn import "${staging}" "https://dist.apache.org/repos/dist/dev/paimon/${staging_name}" \
    -m "Apache Paimon Terraform provider ${version} RC${rc}"
fi

cat <<MAIL
To: dev@paimon.apache.org
Subject: [VOTE][Terraform] Release Apache Paimon Terraform provider v${version} RC${rc}

The candidate is based on ${rc_hash}.
Source: https://dist.apache.org/repos/dist/dev/paimon/${staging_name}
Convenience binaries: https://github.com/${repository}/releases/tag/${tag}

The vote is open for at least 72 hours.
MAIL

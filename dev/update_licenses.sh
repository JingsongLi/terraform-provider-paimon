#!/usr/bin/env bash
#
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

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
platforms="${LICENSE_PLATFORMS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64}"
check_only=0
if [[ "${1:-}" == "--check" ]]; then
  check_only=1
elif [[ $# -ne 0 ]]; then
  echo "usage: $0 [--check]" >&2
  exit 2
fi

work_dir="$(mktemp -d -t paimon-provider-licenses.XXXXXX)"
trap 'rm -rf "${work_dir}"' EXIT
generated_dir="${work_dir}/generated"
mkdir -p "${generated_dir}/licenses-binary"

cp "${repo_root}/LICENSE" "${generated_dir}/LICENSE-binary"
cp "${repo_root}/NOTICE" "${generated_dir}/NOTICE-binary"
printf '\n\nTHIRD-PARTY COMPONENTS INCLUDED IN THE CONVENIENCE BINARIES\n\n' >>"${generated_dir}/LICENSE-binary"
printf 'The complete license text for each linked Go module is provided in the licenses directory.\n\n' >>"${generated_dir}/LICENSE-binary"

: >"${work_dir}/modules.raw"
for platform in ${platforms}; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  if [[ -z "${goos}" || -z "${goarch}" || "${goos}" == "${platform}" ]]; then
    echo "invalid GOOS/GOARCH entry: ${platform}" >&2
    exit 1
  fi
  (
    cd "${repo_root}"
    GOOS="${goos}" GOARCH="${goarch}" go list -deps -f '{{with .Module}}{{.Path}}{{"\t"}}{{.Version}}{{"\t"}}{{.Dir}}{{end}}' .
  ) >>"${work_dir}/modules.raw"
done

main_module="$(cd "${repo_root}" && go list -m)"
LC_ALL=C sort -u "${work_dir}/modules.raw" | awk -F '\t' -v main="${main_module}" 'NF == 3 && $1 != main' >"${work_dir}/modules.tsv"
if [[ ! -s "${work_dir}/modules.tsv" ]]; then
  echo "no linked third-party modules were found" >&2
  exit 1
fi

while IFS=$'\t' read -r module version module_dir; do
  slug="$(printf '%s' "${module}" | sed -E 's|^github.com/||; s|/v[0-9]+$||; s|[^A-Za-z0-9._-]+|-|g')"
  license_source=""
  for candidate in LICENSE LICENSE.txt LICENSE.md COPYING COPYING.txt COPYING.md; do
    if [[ -f "${module_dir}/${candidate}" ]]; then
      license_source="${module_dir}/${candidate}"
      break
    fi
  done
  if [[ -z "${license_source}" ]]; then
    echo "no license file found for linked module ${module} ${version} in ${module_dir}" >&2
    exit 1
  fi
  license_name="LICENSE-${slug}.txt"
  awk '{ lines[NR] = $0; if ($0 !~ /^[[:space:]]*$/) last = NR } END { for (line = 1; line <= last; line++) print lines[line] }' \
    "${license_source}" >"${generated_dir}/licenses-binary/${license_name}"
  printf '%s %s -- licenses/%s\n' "${module}" "${version}" "${license_name}" >>"${generated_dir}/LICENSE-binary"

  for notice_name in NOTICE NOTICE.txt NOTICE.md; do
    if [[ -f "${module_dir}/${notice_name}" ]]; then
      printf '\n--------------------------------------------------------------------------------\n\n%s %s NOTICE:\n' "${module}" "${version}" >>"${generated_dir}/NOTICE-binary"
      awk '{ if (length($0) == 0) print "|"; else print "| " $0 }' \
        "${module_dir}/${notice_name}" >>"${generated_dir}/NOTICE-binary"
      break
    fi
  done
done <"${work_dir}/modules.tsv"

if [[ ${check_only} -eq 1 ]]; then
  status=0
  cmp -s "${generated_dir}/LICENSE-binary" "${repo_root}/LICENSE-binary" || {
    echo "LICENSE-binary is stale; run ./dev/update_licenses.sh" >&2
    status=1
  }
  cmp -s "${generated_dir}/NOTICE-binary" "${repo_root}/NOTICE-binary" || {
    echo "NOTICE-binary is stale; run ./dev/update_licenses.sh" >&2
    status=1
  }
  diff -qr "${generated_dir}/licenses-binary" "${repo_root}/licenses-binary" >/dev/null 2>&1 || {
    echo "licenses-binary is stale; run ./dev/update_licenses.sh" >&2
    status=1
  }
  exit "${status}"
fi

cp "${generated_dir}/LICENSE-binary" "${repo_root}/LICENSE-binary"
cp "${generated_dir}/NOTICE-binary" "${repo_root}/NOTICE-binary"
rm -rf "${repo_root}/licenses-binary"
cp -R "${generated_dir}/licenses-binary" "${repo_root}/licenses-binary"

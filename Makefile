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

.PHONY: build check check-license fmt fmt-check fmt-terraform test test-acceptance test-race validate-examples vet

build:
	go build ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)"

fmt-terraform:
	terraform fmt -check -recursive examples

validate-examples: fmt-terraform
	dev/validate_examples.sh

test:
	go test ./...

test-acceptance:
	TF_ACC=1 go test -v ./internal/provider -run '^TestAcc'

test-race:
	go test -race ./...

vet:
	go vet ./...

check: fmt-check vet test-race build validate-examples

check-license:
	dev/check-license

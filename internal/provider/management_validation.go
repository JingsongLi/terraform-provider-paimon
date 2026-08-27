// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package provider

import (
	"context"
	"errors"
	"strings"
	"unicode/utf16"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const maxPrincipalUTF16Units = 128

type nonBlankStringValidator struct{}

func (nonBlankStringValidator) Description(context.Context) string {
	return "must contain at least one non-whitespace character"
}

func (nonBlankStringValidator) MarkdownDescription(context.Context) string {
	return "must contain at least one non-whitespace character"
}

func (nonBlankStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if isManagementBlank(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid empty Paimon identifier", "The value must contain at least one non-whitespace character.")
	}
}

type principalStringValidator struct{}

func (principalStringValidator) Description(context.Context) string {
	return "must be non-blank and contain at most 128 UTF-16 code units"
}

func (principalStringValidator) MarkdownDescription(context.Context) string {
	return "must be non-blank and contain at most 128 UTF-16 code units"
}

func (principalStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := validateManagementPrincipal(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Paimon principal", err.Error())
	}
}

func nonEmptyStringValidators() []validator.String {
	return []validator.String{nonBlankStringValidator{}}
}

func principalValidators() []validator.String {
	return []validator.String{principalStringValidator{}}
}

func validateManagementPrincipal(value string) error {
	if isManagementBlank(value) {
		return errors.New("principal cannot be empty")
	}
	if len(utf16.Encode([]rune(value))) > maxPrincipalUTF16Units {
		return errors.New("principal must contain at most 128 UTF-16 code units")
	}

	return nil
}

// Paimon's Java management models use String.trim(), which removes code units
// through U+0020, when deciding whether identity strings are blank.
func isManagementBlank(value string) bool {
	return strings.TrimFunc(value, func(character rune) bool {
		return character <= '\u0020'
	}) == ""
}

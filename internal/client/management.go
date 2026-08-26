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

package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

const (
	ResourceTypeCatalog     = "CATALOG"
	ResourceTypeCatalogAll  = "CATALOG_ALL"
	ResourceTypeDatabase    = "DATABASE"
	ResourceTypeDatabaseAll = "DATABASE_ALL"
	ResourceTypeTable       = "TABLE"
	ResourceTypeColumn      = "COLUMN"
	ResourceTypeFunction    = "FUNCTION"
	ResourceTypeView        = "VIEW"

	PermissionAccessAll            = "ALL"
	PermissionAccessCreateDatabase = "CREATEDATABASE"
	PermissionAccessDescribe       = "DESCRIBE"
	PermissionAccessAlter          = "ALTER"
	PermissionAccessDrop           = "DROP"
	PermissionAccessCreateTable    = "CREATETABLE"
	PermissionAccessCreateFunction = "CREATEFUNCTION"
	PermissionAccessCreateView     = "CREATEVIEW"
	PermissionAccessList           = "LIST"
	PermissionAccessSelect         = "SELECT"
	PermissionAccessUpdate         = "UPDATE"
	PermissionAccessGrant          = "GRANT"

	PolicyTypeRowFilter     = "ROW_FILTER"
	PolicyTypeColumnMasking = "COLUMN_MASKING"
)

type PermissionResource struct {
	Type     string `json:"type"`
	Database string `json:"database,omitempty"`
	Table    string `json:"table,omitempty"`
	Function string `json:"function,omitempty"`
	View     string `json:"view,omitempty"`
}

type PermissionColumns struct {
	ColumnNames         []string `json:"columnNames,omitempty"`
	ExcludedColumnNames []string `json:"excludedColumnNames,omitempty"`
}

type PermissionAssignment struct {
	Resource   PermissionResource `json:"resource"`
	Access     string             `json:"access"`
	Principal  string             `json:"principal"`
	Columns    *PermissionColumns `json:"columns,omitempty"`
	ExpireTime *string            `json:"expireTime,omitempty"`
}

type ListPermissionsRequest struct {
	Resource   PermissionResource
	Principal  string
	Access     string
	PageToken  string
	MaxResults int
}

type ListPermissionsResponse struct {
	Permissions   []PermissionAssignment `json:"permissions"`
	NextPageToken string                 `json:"nextPageToken"`
}

type RowFilter struct {
	Predicate string `json:"predicate"`
}

type ColumnMask struct {
	OnColumn  string `json:"onColumn"`
	Transform string `json:"transform"`
}

type DataPolicy struct {
	Resource   PermissionResource `json:"resource"`
	RowFilter  *RowFilter         `json:"rowFilter,omitempty"`
	ColumnMask *ColumnMask        `json:"columnMask,omitempty"`
	Principal  string             `json:"principal"`
}

type PolicyRequest struct {
	RowFilter  *RowFilter  `json:"rowFilter,omitempty"`
	ColumnMask *ColumnMask `json:"columnMask,omitempty"`
	Principal  string      `json:"principal"`
}

type ListPoliciesRequest struct {
	Database   string
	Table      string
	Type       string
	Principal  string
	Column     string
	PageToken  string
	MaxResults int
}

type ListPoliciesResponse struct {
	Policies      []DataPolicy `json:"policies"`
	NextPageToken string       `json:"nextPageToken"`
}

type dropPolicyRequest struct {
	Type      string `json:"type"`
	Principal string `json:"principal"`
	Column    string `json:"column,omitempty"`
}

func (c *Client) ListPermissions(ctx context.Context, request ListPermissionsRequest) (*ListPermissionsResponse, error) {
	query := make(url.Values)
	query.Set("resourceType", request.Resource.Type)
	setQueryValue(query, "database", request.Resource.Database)
	setQueryValue(query, "table", request.Resource.Table)
	setQueryValue(query, "function", request.Resource.Function)
	setQueryValue(query, "view", request.Resource.View)
	setQueryValue(query, "principal", request.Principal)
	setQueryValue(query, "access", request.Access)
	setQueryValue(query, "pageToken", request.PageToken)
	if request.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(request.MaxResults))
	}

	var response ListPermissionsResponse
	err := c.do(ctx, http.MethodGet, []string{"v1", c.catalogPrefix(), "permissions"}, query, nil, &response)

	return &response, err
}

func (c *Client) GrantPermission(ctx context.Context, assignment PermissionAssignment) error {
	return c.do(ctx, http.MethodPost, []string{"v1", c.catalogPrefix(), "permissions", "grant"}, nil, assignment, nil)
}

func (c *Client) RevokePermission(ctx context.Context, resource PermissionResource, access, principal string) error {
	request := struct {
		Resource  PermissionResource `json:"resource"`
		Access    string             `json:"access"`
		Principal string             `json:"principal"`
	}{Resource: resource, Access: access, Principal: principal}

	return c.do(ctx, http.MethodPost, []string{"v1", c.catalogPrefix(), "permissions", "revoke"}, nil, request, nil)
}

func (c *Client) ListPolicies(ctx context.Context, request ListPoliciesRequest) (*ListPoliciesResponse, error) {
	query := make(url.Values)
	setQueryValue(query, "type", request.Type)
	setQueryValue(query, "principal", request.Principal)
	setQueryValue(query, "column", request.Column)
	setQueryValue(query, "pageToken", request.PageToken)
	if request.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(request.MaxResults))
	}

	var response ListPoliciesResponse
	err := c.do(ctx, http.MethodGet, []string{"v1", c.catalogPrefix(), "databases", request.Database, "tables", request.Table, "policies"}, query, nil, &response)

	return &response, err
}

func (c *Client) CreatePolicy(ctx context.Context, database, table string, request PolicyRequest) error {
	return c.do(ctx, http.MethodPost, []string{"v1", c.catalogPrefix(), "databases", database, "tables", table, "policies"}, nil, request, nil)
}

func (c *Client) DropPolicy(ctx context.Context, database, table, policyType, principal, column string) error {
	request := dropPolicyRequest{Type: policyType, Principal: principal, Column: column}

	return c.do(ctx, http.MethodPost, []string{"v1", c.catalogPrefix(), "databases", database, "tables", table, "policies", "drop"}, nil, request, nil)
}

func setQueryValue(query url.Values, name, value string) {
	if value != "" {
		query.Set(name, value)
	}
}

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientConfigAndDatabaseLifecycle(t *testing.T) {
	var configCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		assert.Equal(t, "client-value", r.Header.Get("X-Client"))

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/config":
			configCalls.Add(1)
			assert.Equal(t, "warehouse-a", r.URL.Query().Get("warehouse"))
			writeJSON(t, w, map[string]any{
				"defaults":  map[string]string{"prefix": "default", "header.X-Server": "server-value"},
				"overrides": map[string]string{"prefix": "catalog"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/catalog/databases":
			assert.Equal(t, "server-value", r.Header.Get("X-Server"))
			var request createDatabaseRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.Equal(t, "analytics", request.Name)
			assert.Equal(t, map[string]string{"owner": "data"}, request.Options)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/catalog/databases/analytics":
			assert.Equal(t, "server-value", r.Header.Get("X-Server"))
			writeJSON(t, w, Database{ID: "db-1", Name: "analytics", Options: map[string]string{"owner": "data"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	api, err := New(Config{
		URI:       server.URL + "/api/",
		Warehouse: "warehouse-a",
		Token:     "secret",
		Prefix:    "client-prefix",
		Headers:   map[string]string{"X-Client": "client-value"},
	})
	require.NoError(t, err)

	require.NoError(t, api.CreateDatabase(context.Background(), "analytics", map[string]string{"owner": "data"}))
	database, err := api.GetDatabase(context.Background(), "analytics")
	require.NoError(t, err)
	assert.Equal(t, "db-1", database.ID)
	assert.Equal(t, int32(1), configCalls.Load())
}

func TestClientReturnsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/config" {
			writeJSON(t, w, ConfigResponse{Defaults: map[string]string{"prefix": "catalog"}})

			return
		}
		w.WriteHeader(http.StatusNotFound)
		writeJSON(t, w, map[string]any{"message": "missing", "resourceType": "TABLE", "code": 404})
	}))
	defer server.Close()

	api, err := New(Config{URI: server.URL})
	require.NoError(t, err)
	_, err = api.GetTable(context.Background(), "db", "unknown")
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "missing", apiErr.Message)
}

func TestClientTableLifecycleRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/config":
			writeJSON(t, w, ConfigResponse{Defaults: map[string]string{"prefix": "catalog"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/catalog/databases/db/tables":
			var request createTableRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.Equal(t, Identifier{Database: "db", Object: "events"}, request.Identifier)
			require.Len(t, request.Schema.Fields, 1)
			assert.Equal(t, DataType("BIGINT NOT NULL"), request.Schema.Fields[0].Type)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/catalog/databases/db/tables/events":
			var request alterTableRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			require.Len(t, request.Changes, 1)
			assert.Equal(t, "setOption", request.Changes[0]["action"])
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/catalog/databases/db/tables/events":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	api, err := New(Config{URI: server.URL})
	require.NoError(t, err)
	require.NoError(t, api.CreateTable(context.Background(), "db", "events", Schema{
		Fields: []Field{{ID: 0, Name: "id", Type: DataType("BIGINT NOT NULL")}},
	}))
	require.NoError(t, api.AlterTable(context.Background(), "db", "events", []SchemaChange{{"action": "setOption", "key": "bucket", "value": "4"}}))
	require.NoError(t, api.DropTable(context.Background(), "db", "events"))
}

func TestDataTypeStructuredJSON(t *testing.T) {
	input := []byte(`{"type":"ROW NOT NULL","fields":[{"id":1,"name":"item","type":{"type":"ARRAY","element":"STRING NOT NULL"}}]}`)
	var dataType DataType
	require.NoError(t, json.Unmarshal(input, &dataType))
	assert.Equal(t, DataType("ROW<item ARRAY<STRING NOT NULL>> NOT NULL"), dataType)

	encoded, err := json.Marshal(dataType)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type":"ROW NOT NULL",
		"fields":[{"id":0,"name":"item","type":{"type":"ARRAY","element":"STRING NOT NULL"}}]
	}`, string(encoded))
}

func TestSchemaMarshalAssignsUniqueNestedFieldIDs(t *testing.T) {
	schema := Schema{
		Fields: []Field{
			{ID: 2, Name: "id", Type: DataType("BIGINT NOT NULL")},
			{
				ID:   7,
				Name: "payload",
				Type: DataType("ROW<`item name` ARRAY<MAP<STRING NOT NULL, ROW<value VECTOR<DOUBLE, 3>>>> COMMENT 'item''s label' DEFAULT CAST(NULL AS STRING)>"),
			},
		},
	}

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"fields":[
			{"id":2,"name":"id","type":"BIGINT NOT NULL"},
			{"id":7,"name":"payload","type":{
				"type":"ROW",
				"fields":[{
					"id":8,
					"name":"item name",
					"type":{"type":"ARRAY","element":{"type":"MAP","key":"STRING NOT NULL","value":{"type":"ROW","fields":[{"id":9,"name":"value","type":{"type":"VECTOR","element":"DOUBLE","length":3}}]}}},
					"description":"item's label",
					"defaultValue":"CAST(NULL AS STRING)"
				}]
			}}
		],
		"partitionKeys":[],
		"primaryKeys":[],
		"options":{}
	}`, string(encoded))
}

func TestDataTypeMarshalRejectsMalformedComposite(t *testing.T) {
	_, err := json.Marshal(DataType("MAP<STRING>"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected key and value types")
}

func TestDataTypeMarshalSupportsEmptyRow(t *testing.T) {
	encoded, err := json.Marshal(DataType("ROW<> NOT NULL"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"ROW NOT NULL","fields":[]}`, string(encoded))
}

func TestDataTypeMarshalKeepsComparisonsInNestedDefaults(t *testing.T) {
	for _, test := range []struct {
		name           string
		dataType       DataType
		greaterDefault string
		lesserDefault  string
	}{
		{
			name:           "parenthesized",
			dataType:       DataType("ROW<greater BOOLEAN DEFAULT (1 > 0), lesser BOOLEAN DEFAULT (1 < 2)>"),
			greaterDefault: "(1 > 0)",
			lesserDefault:  "(1 < 2)",
		},
		{
			name:           "unparenthesized",
			dataType:       DataType("ROW<greater BOOLEAN DEFAULT 1 > 0, lesser BOOLEAN DEFAULT 1 < 2>"),
			greaterDefault: "1 > 0",
			lesserDefault:  "1 < 2",
		},
		{
			name:           "opaque composite keyword",
			dataType:       DataType("ROW<greater BOOLEAN DEFAULT MAP > 0, lesser BOOLEAN DEFAULT MAP < 2>"),
			greaterDefault: "MAP > 0",
			lesserDefault:  "MAP < 2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.dataType)
			require.NoError(t, err)
			var structured struct {
				Fields []struct {
					DefaultValue string `json:"defaultValue"`
				} `json:"fields"`
			}
			require.NoError(t, json.Unmarshal(encoded, &structured))
			require.Len(t, structured.Fields, 2)
			assert.Equal(t, test.greaterDefault, structured.Fields[0].DefaultValue)
			assert.Equal(t, test.lesserDefault, structured.Fields[1].DefaultValue)
		})
	}
}

func TestEquivalentDataTypesNormalizesCompositeSpelling(t *testing.T) {
	assert.True(t, EquivalentDataTypes(DataType("MAP<STRING,STRING>"), DataType("MAP<STRING, STRING>")))
	assert.True(t, EquivalentDataTypes(DataType("ROW<`item` STRING>"), DataType("ROW<item STRING>")))
	assert.False(t, EquivalentDataTypes(DataType("MAP<STRING, STRING>"), DataType("MAP<STRING, BIGINT>")))
}

func TestDataTypeMarshalKeepsComparisonInNestedRowDefault(t *testing.T) {
	encoded, err := json.Marshal(DataType("ROW<nested ROW<flag BOOLEAN DEFAULT 1 > 0>, tail BOOLEAN DEFAULT 1 < 2>"))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type":"ROW",
		"fields":[
			{"id":0,"name":"nested","type":{"type":"ROW","fields":[
				{"id":1,"name":"flag","type":"BOOLEAN","defaultValue":"1 > 0"}
			]}},
			{"id":2,"name":"tail","type":"BOOLEAN","defaultValue":"1 < 2"}
		]
	}`, string(encoded))
}

func TestSchemaMarshalRejectsInvalidFieldIDs(t *testing.T) {
	_, err := json.Marshal(Schema{Fields: []Field{
		{ID: maxPaimonFieldID, Name: "max", Type: DataType("STRING")},
	}})
	require.NoError(t, err)

	_, err = json.Marshal(Schema{Fields: []Field{
		{ID: 1, Name: "first", Type: DataType("STRING")},
		{ID: 1, Name: "duplicate", Type: DataType("STRING")},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field ID 1 is duplicated")

	_, err = json.Marshal(Schema{Fields: []Field{
		{ID: maxPaimonFieldID + 1, Name: "reserved", Type: DataType("STRING")},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be between 0 and")

	_, err = json.Marshal(Schema{Fields: []Field{
		{ID: maxPaimonFieldID, Name: "row", Type: DataType("ROW<nested STRING>")},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested field IDs exceed")
}

func TestNewRejectsInvalidURI(t *testing.T) {
	_, err := New(Config{URI: "localhost:8080"})
	require.EqualError(t, err, "Paimon REST URI must use http or https")
}

func TestClientRejectsRedirectWithoutForwardingCredentials(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	for name, config := range map[string]Config{
		"bearer": {URI: redirect.URL, Token: "must-not-leak"},
		"dlf": {
			URI:          redirect.URL,
			AuthProvider: AuthProviderDLF,
			DLF: &DLFConfig{
				Region:           "cn-hangzhou",
				AccessKeyID:      "must-not-leak-id",
				AccessKeySecret:  "must-not-leak-secret",
				SecurityToken:    "must-not-leak-sts",
				SigningAlgorithm: DLFSigningDefault,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			api, err := New(config)
			require.NoError(t, err)
			_, err = api.GetDatabase(context.Background(), "analytics")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "redirects are not allowed")
			assert.NotContains(t, err.Error(), "must-not-leak")
			assert.Equal(t, int32(0), targetCalls.Load())
		})
	}
}

func TestClientRejectsOversizedSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/config" {
			writeJSON(t, w, ConfigResponse{Defaults: map[string]string{"prefix": "catalog"}})

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", maxAPIResponseBodySize)))
		_, _ = w.Write([]byte(`"}`))
	}))
	defer server.Close()

	api, err := New(Config{URI: server.URL})
	require.NoError(t, err)
	_, err = api.GetDatabase(context.Background(), "analytics")
	require.EqualError(t, err, "Paimon REST response exceeded 16 MiB size limit")
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

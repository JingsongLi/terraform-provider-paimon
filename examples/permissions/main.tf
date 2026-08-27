// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

terraform {
  required_providers {
    paimon = {
      source = "apache/paimon"
    }
  }
}

variable "paimon_token" {
  type      = string
  sensitive = true
}

provider "paimon" {
  uri            = "http://localhost:8080"
  warehouse      = "default"
  token_provider = "bear"
  token          = var.paimon_token
}

resource "paimon_database" "analytics" {
  name = "analytics"
}

resource "paimon_table" "events" {
  database = paimon_database.analytics.name
  name     = "events"
  fields = [
    {
      name = "region"
      type = "STRING"
    },
    {
      name = "email"
      type = "STRING"
    },
  ]
  options = {
    "query-auth.enabled" = "true"
  }
}

resource "paimon_permission" "analyst_select" {
  resource_type = "TABLE"
  database      = paimon_table.events.database
  table         = paimon_table.events.name
  access        = "SELECT"
  principal     = "role:analyst"
}

resource "paimon_row_filter" "analyst_region" {
  database  = paimon_table.events.database
  table     = paimon_table.events.name
  principal = paimon_permission.analyst_select.principal
  predicate = jsonencode({
    kind = "LEAF"
    transform = {
      name = "FIELD_REF"
      fieldRef = {
        index = 0
        name  = "region"
        type  = "STRING"
      }
    }
    function = "EQUAL"
    literals = ["APAC"]
  })
}

resource "paimon_column_mask" "analyst_email" {
  database  = paimon_table.events.database
  table     = paimon_table.events.name
  principal = paimon_permission.analyst_select.principal
  column    = "email"
  transform = jsonencode({
    name = "CONCAT"
    inputs = ["***@***.com"]
  })
}

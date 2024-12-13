// Copyright 2024 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const metadataURLIPPrefix = "http://169.254.169.254/computeMetadata/v1/instance/"

type Token struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// TestTokenFetch test service-accounts token could be retrieved from metadata.
func TestTokenFetch(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	metadata, err := utils.GetMetadata(ctx, "instance", "service-accounts", "default", "token")
	if err != nil {
		t.Fatalf("couldn't get token from metadata, err % v", err)
	}
	if err := json.Unmarshal([]byte(metadata), &Token{}); err != nil {
		t.Fatalf("token %s has incorrect format", metadata)
	}
}

// TestMetaDataResponseHeaders verify that HTTP response headers do not include confidential data.
func TestMetaDataResponseHeaders(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	_, headers, err := utils.GetMetadataWithHeaders(ctx, "instance", "id")
	if err != nil {
		t.Fatalf("couldn't get id from metadata, err % v", err)
	}
	for key, values := range headers {
		if key != "Metadata-Flavor" {
			for _, v := range values {
				if strings.Contains(strings.ToLower(v), "google") {
					t.Fatal("unexpected Google header exists in metadata response")
				}
			}
		}
	}
}

// TestGetMetaDataUsingIP test that metadata can be retrieved by IP
func TestGetMetaDataUsingIP(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", metadataURLIPPrefix, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Metadata-Flavor", "Google")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("http response code is %v", resp.StatusCode)
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

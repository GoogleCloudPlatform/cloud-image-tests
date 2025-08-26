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

package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/compute/v1"
)

const (
	// metadataURLPrefix is the base URL for the metadata server.
	metadataURLPrefix = "http://metadata.google.internal/computeMetadata/v1/"
	// httpTimeout is the timeout for HTTP requests.
	httpTimeout = time.Second * 30
)

var (
	// ErrMDSEntryNotFound is an error used to report 404 status code.
	ErrMDSEntryNotFound = errors.New("No metadata entry found: 404 error")
)

// GetMetadata does a HTTP Get request to the metadata server, the metadata entry of
// interest is provided by elem as the elements of the entry path, the following example
// does a Get request to the entry "instance/guest-attributes":
//
// resp, err := GetAttribute(context.Background(), "instance", "guest-attributes")
// ...
func GetMetadata(ctx context.Context, elem ...string) (string, error) {
	path, err := url.JoinPath(metadataURLPrefix, elem...)
	if err != nil {
		return "", fmt.Errorf("failed to parse metadata url: %+s", err)
	}

	body, _, err := doHTTPGet(ctx, path)
	return body, err
}

// GetMetadataWithHeaders is similar to GetMetadata it only differs on the return where GetMetadata
// returns only the response's body as a string and an error GetMetadataWithHeaders returns the
// response's body as a string, the headers and an error.
func GetMetadataWithHeaders(ctx context.Context, elem ...string) (string, http.Header, error) {
	path, err := url.JoinPath(metadataURLPrefix, elem...)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse metadata url: %+s", err)
	}

	return doHTTPGet(ctx, path)
}

// PutMetadata does a HTTP Put request to the metadata server, the metadata entry of
// interest is provided by path as the section of the path after the metadata server,
// with the data string as the post data. The following example sets the key
// "instance/guest-attributes/example" to "data":
//
// err := PutMetadata(context.Background(), path.Join("instance", "guest-attributes", "example"), "data")
// ...
func PutMetadata(ctx context.Context, path string, data string) error {
	path, err := url.JoinPath(metadataURLPrefix, path)
	if err != nil {
		return fmt.Errorf("failed to parse metadata url: %+v", err)
	}

	err = doHTTPPut(ctx, path, data)
	if err != nil {
		return err
	}

	return nil
}

func doHTTPRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: httpTimeout}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do the http request: %+v", err)
	}

	if resp.StatusCode == 404 {
		return nil, ErrMDSEntryNotFound
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http response code is %v", resp.StatusCode)
	}

	return resp, nil
}

func doHTTPGet(ctx context.Context, path string) (string, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create a http request with context: %+v", err)
	}
	req.Header.Add("Metadata-Flavor", "Google")

	httpGet := func() (string, http.Header, error) {
		resp, err := doHTTPRequest(req)
		if err != nil {
			return "", nil, err
		}

		val, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read http request body: %+v", err)
		}

		return string(val), resp.Header, nil
	}

	var resp string
	var header http.Header
	var getErr error

	for i := 1; i <= 5; i++ {
		if resp, header, getErr = httpGet(); getErr != nil {
			time.Sleep(time.Duration(i) * time.Second)
			continue
		}
		break
	}

	return resp, header, getErr
}

func doHTTPPut(ctx context.Context, path string, data string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, path, strings.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create a http request with context: %+v", err)
	}
	req.Header.Add("Metadata-Flavor", "Google")

	for i := 1; i <= 5; i++ {
		if _, err = doHTTPRequest(req); err != nil {
			time.Sleep(time.Duration(i) * time.Second)
			continue
		}
		break
	}

	return err
}

// SetInstanceMetadata sets the instance metadata.
func SetInstanceMetadata(t *testing.T, name string, metadata *compute.Metadata) {
	t.Helper()
	ctx := Context(t)
	client, err := GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("failed to get daisy client: %v", err)
	}

	prj, zone, err := GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("failed to get project zone: %v", err)
	}

	if err := client.SetInstanceMetadata(prj, zone, name, metadata); err != nil {
		t.Fatalf("failed to set instance metadata: %v", err)
	}
}

// GetInstanceMetadata gets the metadata for the instance.
func GetInstanceMetadata(t *testing.T, name string) *compute.Metadata {
	t.Helper()
	ctx := Context(t)
	client, err := GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("failed to get daisy client: %v", err)
	}

	prj, zone, err := GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("failed to get project zone: %v", err)
	}

	inst, err := client.GetInstance(prj, zone, name)
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	return inst.Metadata
}

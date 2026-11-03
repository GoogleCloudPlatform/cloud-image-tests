// Copyright 2026 Google LLC
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

package networkconfig

import (
	"context"
	_ "embed"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	pb "github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkconfig/config_expectations_proto"
)

//go:embed config_expectations/config_expectations.textproto
var configExpectationsBytes []byte

func parseConfigExpectations() (*pb.ConfigExpectations, error) {
	var configExpectations pb.ConfigExpectations
	if err := prototext.Unmarshal(configExpectationsBytes, &configExpectations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedded config expectations: %w", err)
	}
	return &configExpectations, nil
}

// guestOSConstraints fetches the guest image from MDS and merges all matching constraints.
func guestOSConstraints(ctx context.Context, constraints []*pb.GuestOsConstraint) (*pb.GuestOsConstraint, error) {
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		return nil, fmt.Errorf("getting instance image from metadata: %w", err)
	}
	imageName := path.Base(strings.TrimSpace(image))

	merged := &pb.GuestOsConstraint{}
	for _, c := range constraints {
		matched, err := regexp.MatchString(c.GetOsPattern(), imageName)
		if err != nil {
			return nil, fmt.Errorf("matching os_pattern %q: %w", c.GetOsPattern(), err)
		}
		if matched {
			proto.Merge(merged, c)
		}
	}
	return merged, nil
}

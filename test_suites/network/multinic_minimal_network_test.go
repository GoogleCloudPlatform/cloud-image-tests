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

package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func setupFirewall(t *testing.T) {
	if utils.IsWindows() {
		out, err := utils.RunPowershellCmd(`New-NetFirewallRule -DisplayName 'open8080inbound' -LocalPort 8080 -Action Allow -Profile 'Public' -Protocol TCP -Direction Inbound`)
		if err != nil {
			t.Fatalf("could not allow inbound traffic on port 8080: %s %s %v", out.Stdout, out.Stderr, err)
		}
		out, err = utils.RunPowershellCmd(`New-NetFirewallRule -DisplayName 'open8080outbound' -LocalPort 8080 -Action Allow -Profile 'Public' -Protocol TCP -Direction Outbound`)
		if err != nil {
			t.Fatalf("could not allow outbound traffic on port 8080: %s %s %v", out.Stdout, out.Stderr, err)
		}
	}
}

func TestSendPing(t *testing.T) {
	ctx := utils.Context(t)
	if utils.IsWindows() {
		setupFirewall(t)
	}
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	primaryIP, err := utils.GetMetadata(ctx, "instance", "network-interfaces", "0", "ip")
	if err != nil {
		t.Fatalf("couldn't get internal network IP from metadata, %v", err)
	}
	secondaryIP, err := utils.GetMetadata(ctx, "instance", "network-interfaces", "1", "ip")
	if err != nil {
		t.Fatalf("couldn't get internal network IP from metadata, %v", err)
	}

	targetName, err := utils.GetRealVMName(vm2Config.name)
	if err != nil {
		t.Fatalf("failed to determine target vm name: %v", err)
	}
	if err := pingTarget(ctx, primaryIP, targetName); err != nil {
		t.Fatalf("failed to ping remote %s via %s (primary network): %v", targetName, primaryIP, err)
	}
	if !strings.Contains(image, "cos") {
		if err := pingTarget(ctx, secondaryIP, vm2Config.ip); err != nil {
			t.Fatalf("failed to ping remote %s via %s (secondary network): %v", vm2Config.ip, secondaryIP, err)
		}
	}
}

// send "echo" over tcp to target, expect the same back retry until context is
// expired, fail early if we succesfully connect with an unexpected response.
func pingTarget(ctx context.Context, source, target string) error {
	d := net.Dialer{
		Timeout:   5 * time.Second,
		LocalAddr: &net.TCPAddr{IP: net.ParseIP(source), Port: 0},
		DualStack: false,
	}
	client := http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: d.DialContext,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:8080/", target), strings.NewReader("echo"))
	if err != nil {
		return fmt.Errorf("could not make request: %v", err)
	}
	var resp *http.Response
	for {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if string(body) != "echo" {
		return fmt.Errorf("unexpected response from target, got %s want echo", body)
	}
	return nil
}

func TestWaitForPing(t *testing.T) {
	marker := "/var/ping-done"
	if utils.IsWindows() {
		marker = `C:\ping-done`
		setupFirewall(t)
	}
	_, err := os.Stat(marker)
	if err == nil {
		return
	}
	ctx := utils.Context(t)
	wantPings := 2
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if strings.Contains(image, "cos") {
		wantPings = 1
	}
	var count int
	var mu sync.RWMutex
	srv := http.Server{
		Addr: ":8080",
	}
	c := make(chan struct{})
	go func() {
	countloop:
		for {
			select {
			case <-c:
				count++
				if count >= wantPings {
					break countloop
				}
			}
		}
		mu.Lock()
		defer mu.Unlock()
		srv.Shutdown(ctx)
	}()
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		c <- struct{}{}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Logf("could not read from connection: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, string(body))
	})
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		t.Errorf("Failed to serve http: %v", err)
	}
	if _, err := os.Create(marker); err != nil {
		t.Errorf("failed to mark ping test finished")
	}
}

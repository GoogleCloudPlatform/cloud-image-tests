// Copyright 2026 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package guestagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/api/compute/v1"
)

const (
	windowsReadinessTimeout    = 180 * time.Second
	windowsReadinessInterval   = 5 * time.Second
	windowsPostBootSettleDelay = 30 * time.Second
	windowsSnapshotMaxAttempts = 3
	windowsSnapshotRetryDelay  = 30 * time.Second
	windowsDiagTimeout         = 30 * time.Second

	guestFlushFailedCode = "GUEST_FLUSH_FAILED_WITH_DETAILS"
	vssStableWriterState = "[1] Stable"
	vssAgentChannel      = "GCE-VSS-Agent/Operational"
	vssProviderChannel   = "GCE-VSS-Provider/Operational"
	// utcSystemTimeLayout matches the UTC format emitted by PowerShell
	// ToString('o') and Windows Event Log TimeCreated/@SystemTime.
	utcSystemTimeLayout = "2006-01-02T15:04:05.0000000Z"

	windowsListServicesCmd = `
$names = @(
  'GoogleVssAgent',
  'GoogleVssProvider',
  'GCEAgentManager',
  'GCEWindowsCompatManager',
  'GCEAgent',
  'google_osconfig_agent'
)
Get-Service -Name $names -ErrorAction SilentlyContinue | ForEach-Object {
  '{0}|{1}|{2}' -f $_.Name, $_.Status, $_.StartType
}
exit 0`

	windowsListWritersCmd = `& vssadmin.exe list writers; exit $LASTEXITCODE`

	windowsCaptureUTCCmd = `[DateTime]::UtcNow.AddSeconds(-1).ToString('o')`

	windowsDiagWritersCmd = `
$ErrorActionPreference = 'SilentlyContinue'
& vssadmin.exe list writers
$code = $LASTEXITCODE
if ($code -ne 0) {
  Write-Output ('vssadmin exited with code {0}' -f $code)
}
exit 0`

	windowsDiagAppVSSCmd = `
$ErrorActionPreference = 'SilentlyContinue'
$xpath = "*[System[Provider[@Name='VSS' or @Name='VolSnap']]]"
$events = @(Get-WinEvent -LogName 'Application' -FilterXPath $xpath -MaxEvents 25 | Sort-Object RecordId)
if ($events.Count -eq 0) {
  Write-Output 'no VSS or VolSnap events found'
  exit 0
}
foreach ($ev in $events) {
  $ts = if ($null -ne $ev.TimeCreated) { $ev.TimeCreated.ToUniversalTime().ToString('o') } else { 'unknown' }
  $lvl = if ($ev.LevelDisplayName) { $ev.LevelDisplayName } else { [string]$ev.Level }
  $msg = ($ev.Message -replace '\r', '').Trim()
  Write-Output ('[{0}] Provider={1} Id={2} Level={3} Message={4}' -f $ts, $ev.ProviderName, $ev.Id, $lvl, $msg)
}
exit 0`
)

// windowsAutomaticGuestServices lists Windows guest services that, when
// installed and configured with StartType Automatic (including Delayed Start),
// should reach Running before taking a guest-flush snapshot.
var windowsAutomaticGuestServices = []string{
	"GoogleVssAgent",
	"GCEAgentManager",
	"GCEWindowsCompatManager",
	"GCEAgent",
	"google_osconfig_agent",
}

// expectedWindowsVSSAgentMessages lists the GCE-VSS-Agent/Operational log
// entries emitted during a successful VSS snapshot. Note that "aysnc" matches
// the literal spelling in GoogleVssAgent.
var expectedWindowsVSSAgentMessages = []string{
	"DoSnapshotSet aysnc operation completed.",
	"Creating the shadow in DoSnapshotSet.",
	"PrepareVolumes return status 0",
	"CheckSelectedWriterStatus returned with 0",
}

type snapshotCreator interface {
	CreateSnapshotWithGuestFlush(project, zone, disk string, s *compute.Snapshot) error
}

type powershellFunc func(ctx context.Context, command string) (utils.ProcessStatus, error)
type sleepFunc func(ctx context.Context, d time.Duration) error
type logFunc func(format string, args ...any)

type windowsService struct {
	name      string
	status    string
	startType string
}

type vssWriter struct {
	name      string
	state     string
	lastError string
}

type snapshotRunner struct {
	isWindows           bool
	readinessTimeout    time.Duration
	readinessInterval   time.Duration
	postBootSettleDelay time.Duration
	maxWindowsAttempts  int
	retryDelay          time.Duration
	diagTimeout         time.Duration
	runPS               powershellFunc
	sleep               sleepFunc
}

func newSnapshotRunner() *snapshotRunner {
	return &snapshotRunner{
		isWindows:           utils.IsWindows(),
		readinessTimeout:    windowsReadinessTimeout,
		readinessInterval:   windowsReadinessInterval,
		postBootSettleDelay: windowsPostBootSettleDelay,
		maxWindowsAttempts:  windowsSnapshotMaxAttempts,
		retryDelay:          windowsSnapshotRetryDelay,
		diagTimeout:         windowsDiagTimeout,
		runPS: func(ctx context.Context, command string) (utils.ProcessStatus, error) {
			return utils.RunPowershellCmdWithEnv(ctx, command)
		},
		sleep: sleepWithContext,
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isRetryableWindowsGuestFlushError reports whether err is a transient HTTP 412
// GUEST_FLUSH_FAILED_WITH_DETAILS error from CreateSnapshotWithGuestFlush.
// compute-daisy formats failed zone operations as plain error strings rather
// than returning a structured error type.
func isRetryableWindowsGuestFlushError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "HttpErrorStatusCode:412") && hasOperationErrorCode(msg, guestFlushFailedCode) {
		return true
	}
	return strings.Contains(msg, "googleapi: Error 412:") && strings.Contains(msg, guestFlushFailedCode)
}

func hasOperationErrorCode(msg, code string) bool {
	want := "Code: " + code
	for line := range strings.SplitSeq(msg, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func parseWindowsServices(output string) []windowsService {
	var services []windowsService
	for line := range strings.SplitSeq(output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		services = append(services, windowsService{
			name:      name,
			status:    strings.TrimSpace(parts[1]),
			startType: strings.TrimSpace(parts[2]),
		})
	}
	return services
}

func formatWindowsServices(services []windowsService) string {
	if len(services) == 0 {
		return "none"
	}
	items := make([]string, 0, len(services))
	for _, svc := range services {
		items = append(items, fmt.Sprintf("%s=%s(%s)", svc.name, svc.status, svc.startType))
	}
	return strings.Join(items, ", ")
}

func checkWindowsServicesReady(services []windowsService) (bool, string) {
	byName := make(map[string]windowsService, len(services))
	for _, svc := range services {
		byName[strings.ToLower(svc.name)] = svc
	}

	var issues []string
	vss, hasVSS := byName[strings.ToLower("GoogleVssAgent")]
	if !hasVSS {
		issues = append(issues, "GoogleVssAgent not found")
	} else if !strings.EqualFold(vss.status, "Running") {
		issues = append(issues, fmt.Sprintf("GoogleVssAgent status=%s(%s)", vss.status, vss.startType))
	}

	var pending []string
	for _, name := range windowsAutomaticGuestServices {
		if strings.EqualFold(name, "GoogleVssAgent") {
			continue
		}
		svc, ok := byName[strings.ToLower(name)]
		if !ok {
			continue
		}
		if strings.EqualFold(svc.startType, "Automatic") && !strings.EqualFold(svc.status, "Running") {
			pending = append(pending, fmt.Sprintf("%s=%s", svc.name, svc.status))
		}
	}
	if len(pending) > 0 {
		issues = append(issues, "pending automatic services: "+strings.Join(pending, ", "))
	}
	if len(issues) > 0 {
		return false, strings.Join(issues, "; ")
	}
	return true, formatWindowsServices(services)
}

func parseVSSWriters(output string) []vssWriter {
	var writers []vssWriter
	var current *vssWriter
	for raw := range strings.SplitSeq(output, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "Writer name:"):
			if current != nil {
				writers = append(writers, *current)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "Writer name:"))
			current = &vssWriter{name: strings.Trim(name, "'")}
		case current != nil && strings.HasPrefix(line, "State:"):
			current.state = strings.TrimSpace(strings.TrimPrefix(line, "State:"))
		case current != nil && strings.HasPrefix(line, "Last error:"):
			current.lastError = strings.TrimSpace(strings.TrimPrefix(line, "Last error:"))
		}
	}
	if current != nil {
		writers = append(writers, *current)
	}
	return writers
}

func checkVSSWritersStable(writers []vssWriter) (bool, string) {
	if len(writers) == 0 {
		return false, "no VSS writers reported"
	}
	var unstable []string
	var priorErrors []string
	for _, w := range writers {
		if w.state != vssStableWriterState {
			state := w.state
			if state == "" {
				state = "unknown"
			}
			desc := fmt.Sprintf("%s state=%s", w.name, state)
			if w.lastError != "" && !strings.EqualFold(w.lastError, "No error") {
				desc += fmt.Sprintf(" lastError=%s", w.lastError)
			}
			unstable = append(unstable, desc)
			continue
		}
		if w.lastError != "" && !strings.EqualFold(w.lastError, "No error") {
			priorErrors = append(priorErrors, fmt.Sprintf("%s(%s)", w.name, w.lastError))
		}
	}
	if len(unstable) > 0 {
		return false, "unstable VSS writers: " + strings.Join(unstable, ", ")
	}
	summary := fmt.Sprintf("%d writers stable", len(writers))
	if len(priorErrors) > 0 {
		summary += " (prior lastError: " + strings.Join(priorErrors, ", ") + ")"
	}
	return true, summary
}

func (r *snapshotRunner) checkWindowsReadinessOnce(ctx context.Context) (ready bool, summary, serviceState, writerOutput string) {
	svcRes, svcErr := r.runPS(ctx, windowsListServicesCmd)
	services := parseWindowsServices(svcRes.Stdout)
	var svcReady bool
	var svcDetail string
	if svcErr != nil {
		svcDetail = fmt.Sprintf("querying services: %v (stderr: %s)", svcErr, strings.TrimSpace(svcRes.Stderr))
	} else {
		svcReady, svcDetail = checkWindowsServicesReady(services)
		serviceState = formatWindowsServices(services)
	}

	vssRes, vssErr := r.runPS(ctx, windowsListWritersCmd)
	writerOutput = strings.TrimSpace(vssRes.Stdout)
	var vssReady bool
	var vssDetail string
	if vssErr != nil || vssRes.Exitcode != 0 {
		vssDetail = fmt.Sprintf("vssadmin failed (exit=%d, err=%v, stderr=%s)",
			vssRes.Exitcode, vssErr, strings.TrimSpace(vssRes.Stderr))
		if writerOutput == "" {
			writerOutput = strings.TrimSpace(vssRes.Stderr)
		}
	} else {
		vssReady, vssDetail = checkVSSWritersStable(parseVSSWriters(vssRes.Stdout))
	}

	summary = fmt.Sprintf("services=[%s]; vss=[%s]", svcDetail, vssDetail)
	return svcReady && vssReady, summary, serviceState, writerOutput
}

// waitForWindowsReady polls for Windows guest services and VSS writers to reach
// a stable state. Timeout expiration is best-effort: it logs the final service
// and writer states and returns nil so the snapshot attempt can still proceed.
func (r *snapshotRunner) waitForWindowsReady(ctx context.Context, logf logFunc) error {
	pollCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
	defer cancel()

	start := time.Now()
	var polls int
	var firstUnready, lastSummary, lastServices, lastWriters string
	err := utils.PollUntil(ctx, "Windows snapshot readiness", r.readinessInterval, r.readinessTimeout, func() (bool, string) {
		polls++
		ready, summary, svcState, writerOut := r.checkWindowsReadinessOnce(pollCtx)
		if summary != "" {
			lastSummary = summary
		}
		if svcState != "" {
			lastServices = svcState
		}
		if writerOut != "" {
			lastWriters = writerOut
		}
		if ready {
			return true, summary
		}
		if firstUnready == "" {
			firstUnready = summary
		}
		return false, summary
	})
	if err == nil { // if NO error
		elapsed := time.Since(start).Round(time.Millisecond)
		if firstUnready != "" {
			logf("Windows snapshot readiness satisfied after %d poll(s) (%v); initial: %s; final: %s",
				polls, elapsed, firstUnready, lastSummary)
			return nil
		}
		logf("Windows snapshot readiness satisfied on first poll (%v): %s", elapsed, lastSummary)
		return nil
	}
	if ctx.Err() != nil {
		return err
	}
	logf("Warning: Windows snapshot readiness timed out after %v (%d poll(s)): %s; proceeding anyway.\nGuest services: %s\nVSS writers:\n%s",
		r.readinessTimeout, polls, lastSummary, lastServices, lastWriters)
	return nil
}

func (r *snapshotRunner) prepareWindowsSnapshot(ctx context.Context, logf logFunc) error {
	if err := r.waitForWindowsReady(ctx, logf); err != nil {
		return err
	}
	logf("Sleeping %v for Windows post-boot settle window before snapshot", r.postBootSettleDelay)
	return r.sleep(ctx, r.postBootSettleDelay)
}

func (r *snapshotRunner) captureWindowsUTC(ctx context.Context, logf logFunc) string {
	cmdCtx, cancel := context.WithTimeout(ctx, r.diagTimeout)
	defer cancel()

	res, err := r.runPS(cmdCtx, windowsCaptureUTCCmd)
	if err != nil {
		logf("Warning: could not capture Windows UTC timestamp before snapshot attempt: %v (stderr: %s)",
			err, strings.TrimSpace(res.Stderr))
		return ""
	}
	raw := strings.TrimSpace(res.Stdout)
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		logf("Warning: could not parse Windows UTC timestamp %q: %v", raw, err)
		return ""
	}
	return ts.UTC().Format(utcSystemTimeLayout)
}

func winEventChannelDiagCmd(channel string, maxEvents int) string {
	quoted := utils.PSSingleQuote(channel)
	return fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
$log = Get-WinEvent -ListLog '%s'
if ($null -eq $log) {
  Write-Output 'event log channel not found'
  exit 0
}
$events = @(Get-WinEvent -LogName '%s' -MaxEvents %d | Sort-Object RecordId)
if ($events.Count -eq 0) {
  Write-Output 'no events found'
  exit 0
}
foreach ($ev in $events) {
  $ts = if ($null -ne $ev.TimeCreated) { $ev.TimeCreated.ToUniversalTime().ToString('o') } else { 'unknown' }
  $lvl = if ($ev.LevelDisplayName) { $ev.LevelDisplayName } else { [string]$ev.Level }
  $msg = ($ev.Message -replace '\r', '').Trim()
  Write-Output ('[{0}] Id={1} Level={2} Message={3}' -f $ts, $ev.Id, $lvl, $msg)
}
exit 0`, quoted, quoted, maxEvents)
}

func (r *snapshotRunner) runDiagnosticCmd(ctx context.Context, label, command string, logf logFunc) {
	diagCtx, cancel := context.WithTimeout(ctx, r.diagTimeout)
	defer cancel()

	res, err := r.runPS(diagCtx, command)
	out := strings.TrimSpace(res.Stdout)
	stderr := strings.TrimSpace(res.Stderr)
	if err != nil {
		logf("Windows VSS diagnostic [%s] command returned error (non-fatal): %v\nstdout:\n%s\nstderr:\n%s",
			label, err, out, stderr)
		return
	}
	logf("Windows VSS diagnostic [%s]:\n%s", label, out)
}

func (r *snapshotRunner) logWindowsVSSDiagnostics(ctx context.Context, attempt, maxAttempts int, logf logFunc) {
	if ctx.Err() != nil {
		logf("Skipping Windows VSS diagnostics after attempt %d/%d because context is done: %v",
			attempt, maxAttempts, ctx.Err())
		return
	}
	prefix := fmt.Sprintf("attempt %d/%d ", attempt, maxAttempts)
	r.runDiagnosticCmd(ctx, prefix+vssAgentChannel, winEventChannelDiagCmd(vssAgentChannel, 120), logf)
	r.runDiagnosticCmd(ctx, prefix+vssProviderChannel, winEventChannelDiagCmd(vssProviderChannel, 80), logf)
	r.runDiagnosticCmd(ctx, prefix+"Application(VSS/VolSnap)", windowsDiagAppVSSCmd, logf)
	r.runDiagnosticCmd(ctx, prefix+"vssadmin list writers", windowsDiagWritersCmd, logf)
}

func (r *snapshotRunner) createSnapshotWithRetry(ctx context.Context, client snapshotCreator, project, zone, inst string, logf logFunc) (startUTC string, attempts int, err error) {
	snapshotName := "snapshot-" + inst
	sourceDisk := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, inst)
	if !r.isWindows {
		snap := &compute.Snapshot{Name: snapshotName, SourceDisk: sourceDisk}
		return "", 1, client.CreateSnapshotWithGuestFlush(project, zone, inst, snap)
	}

	maxAttempts := max(r.maxWindowsAttempts, 1)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			logf("Retrying Windows guest-flush snapshot %q after %v (attempt %d/%d)",
				snapshotName, r.retryDelay, attempt, maxAttempts)
			if err := r.sleep(ctx, r.retryDelay); err != nil {
				return "", attempt - 1, fmt.Errorf("waiting before snapshot retry %d: %w", attempt, err)
			}
		}
		if err := r.waitForWindowsReady(ctx, logf); err != nil {
			return "", attempt - 1, err
		}

		attemptStartUTC := r.captureWindowsUTC(ctx, logf)
		logf("Creating Windows guest-flush snapshot %q (attempt %d/%d, startUTC=%s)",
			snapshotName, attempt, maxAttempts, attemptStartUTC)
		snap := &compute.Snapshot{Name: snapshotName, SourceDisk: sourceDisk}
		snapErr := client.CreateSnapshotWithGuestFlush(project, zone, inst, snap)
		if snapErr == nil {
			logf("Windows guest-flush snapshot %q succeeded on attempt %d/%d",
				snapshotName, attempt, maxAttempts)
			return attemptStartUTC, attempt, nil
		}

		logf("Windows guest-flush snapshot %q failed on attempt %d/%d: %v",
			snapshotName, attempt, maxAttempts, snapErr)
		r.logWindowsVSSDiagnostics(ctx, attempt, maxAttempts, logf)
		if !isRetryableWindowsGuestFlushError(snapErr) || attempt == maxAttempts {
			return "", attempt, snapErr
		}
	}
	return "", maxAttempts, fmt.Errorf("exhausted %d snapshot attempts for %q", maxAttempts, snapshotName)
}

func windowsAgentEventsQuery(sinceUTC string) (string, error) {
	quotedChannel := utils.PSSingleQuote(vssAgentChannel)
	filterArg := ""
	if sinceUTC != "" {
		ts, err := time.Parse(time.RFC3339Nano, sinceUTC)
		if err != nil {
			return "", fmt.Errorf("invalid UTC start timestamp %q: %w", sinceUTC, err)
		}
		utc := ts.UTC().Format(utcSystemTimeLayout)
		filterArg = fmt.Sprintf(` -FilterXPath "*[System[TimeCreated[@SystemTime>='%s']]]"`, utc)
	}
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$null = Get-WinEvent -ListLog '%s'
$events = @(Get-WinEvent -LogName '%s'%s -Oldest -ErrorAction SilentlyContinue)
foreach ($ev in $events) {
  $ts = if ($null -ne $ev.TimeCreated) { $ev.TimeCreated.ToUniversalTime().ToString('o') } else { 'unknown' }
  $lvl = if ($ev.LevelDisplayName) { $ev.LevelDisplayName } else { [string]$ev.Level }
  $msg = ($ev.Message -replace '\r', '').Trim()
  Write-Output ('[{0}] Id={1} Level={2} Message={3}' -f $ts, $ev.Id, $lvl, $msg)
}
exit 0`, quotedChannel, quotedChannel, filterArg), nil
}

func (r *snapshotRunner) verifyWindowsSnapshotLogs(ctx context.Context, sinceUTC string, logf logFunc) ([]string, error) {
	query, err := windowsAgentEventsQuery(sinceUTC)
	if err != nil {
		return nil, err
	}
	res, err := r.runPS(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying %s events (stderr: %s): %w",
			vssAgentChannel, strings.TrimSpace(res.Stderr), err)
	}
	scope := "all"
	if sinceUTC != "" {
		scope = "since " + sinceUTC
	}
	logf("%s output (%s):\n%s", vssAgentChannel, scope, strings.TrimSpace(res.Stdout))

	var missing []string
	for _, msg := range expectedWindowsVSSAgentMessages {
		if !strings.Contains(res.Stdout, msg) {
			missing = append(missing, msg)
		}
	}
	return missing, nil
}

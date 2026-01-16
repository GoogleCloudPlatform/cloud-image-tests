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

// Package utils contains commonly needed utility functions for test suites
// inside the VM and in test workflow setup.
package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	"golang.org/x/crypto/ssh"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

const (
	bytesInGB = 1073741824
	// GuestAttributeTestNamespace is the namespace for the guest attribute in the daisy "wait for instance" step for CIT.
	GuestAttributeTestNamespace = "citTest"
	// GuestAttributeTestKey is the key for the guest attribute in the daisy "wait for instance" step for CIT in the common case.
	GuestAttributeTestKey = "test-complete"
	// FirstBootGAKey is the key for guest attribute in the daisy "wait for instance" step in the case where it is the first boot, and we still want to wait for results from a subsequent reboot.
	FirstBootGAKey = "first-boot-key"
)

var (
	// ErrPackageManagersNotFound is the error message returned when an object is not found.
	ErrPackageManagersNotFound = fmt.Errorf("no supported package managers found")

	windowsClientImagePatterns = []string{
		"windows-7-",
		"windows-8-",
		"windows-10-",
		"windows-11-",
	}

	skipInterfaces = []string{
		"isatap", // isatap tunnel on windows.
		"teredo", // teredo tunnel on windows.
	}
)

// BlockDeviceList gives full information about blockdevices, from the output of lsblk.
type BlockDeviceList struct {
	BlockDevices []BlockDevice `json:"blockdevices,omitempty"`
}

// BlockDevice defines information about a single partition or disk in the output of lsblk.
type BlockDevice struct {
	Name string `json:"name,omitempty"`
	// on some OSes, size is a string, and on some OSes, size is a number.
	// This allows both to be parsed
	Size json.Number `json:"size,omitempty"`
	Type string      `json:"type,omitempty"`
	// Other fields are not currently used.
	X map[string]any `json:"-"`
}

// GetRealVMName returns the real name of a VM running in the same test.
func GetRealVMName(ctx context.Context, name string) (string, error) {
	instanceName, err := GetInstanceName(ctx)
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(instanceName, "-", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("instance name %q doesn't match scheme", instanceName)
	}
	return strings.Join([]string{name, parts[1], parts[2]}, "-"), nil
}

// DownloadGCSObject downloads a GCS object.
func DownloadGCSObject(ctx context.Context, client *storage.Client, gcsPath string) ([]byte, error) {
	u, err := url.Parse(gcsPath)
	if err != nil {
		log.Fatalf("Failed to parse GCS url: %v\n", err)
	}
	object := strings.TrimPrefix(u.Path, "/")
	rc, err := client.Bucket(u.Host).Object(object).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// DownloadGCSObjectToFile downloads a GCS object, writing it to the specified file.
func DownloadGCSObjectToFile(ctx context.Context, client *storage.Client, gcsPath, file string) error {
	data, err := DownloadGCSObject(ctx, client, gcsPath)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(file, data, 0755); err != nil {
		return err
	}
	return nil
}

// ExtractBaseImageName extract the base image name from full image resource.
func ExtractBaseImageName(image string) (string, error) {
	// Example: projects/rhel-cloud/global/images/rhel-8-v20210217
	splits := strings.SplitN(image, "/", 5)
	if len(splits) < 5 {
		return "", fmt.Errorf("malformed image metadata")
	}

	splits = strings.Split(splits[4], "-")
	if len(splits) < 2 {
		return "", fmt.Errorf("malformed base image name")
	}
	imageName := strings.Join(splits[:len(splits)-1], "-")
	return imageName, nil
}

// DownloadPrivateKey download private key from daisy source.
func DownloadPrivateKey(ctx context.Context, user string) ([]byte, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	sourcesPath, err := GetMetadata(ctx, "instance", "attributes", "daisy-sources-path")
	if err != nil {
		return nil, err
	}
	gcsPath := fmt.Sprintf("%s/%s-ssh-key", sourcesPath, user)

	privateKey, err := DownloadGCSObject(ctx, client, gcsPath)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

// GetHostKeysFromDisk read ssh host public key and parse
func GetHostKeysFromDisk() (map[string]string, error) {
	totalBytes, err := GetHostKeysFileFromDisk()
	if err != nil {
		return nil, err
	}
	keymap, err := ParseHostKey(totalBytes)
	if err != nil {
		return nil, err
	}
	return keymap, nil
}

// GetHostKeysFileFromDisk read ssh host public key as bytes
func GetHostKeysFileFromDisk() ([]byte, error) {
	var totalBytes []byte
	keyglob := "/etc/ssh/ssh_host_*_key.pub"
	if IsWindows() {
		keyglob = `C:\ProgramData\ssh\ssh_host_*_key.pub`
	}
	keyFiles, err := filepath.Glob(keyglob)
	if err != nil {
		return nil, err
	}

	for _, file := range keyFiles {
		bytes, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		totalBytes = append(totalBytes, bytes...)
	}
	if IsWindows() {
		// winrm and rdp certs are stored in a certificate store, just grab the thumbprints and pretend we got them from disk for the parser
		winrmThumb, err := RunPowershellCmd(`Get-ChildItem 'Cert:\LocalMachine\My\' | Where-Object {$_.Subject -eq "CN=$(hostname)"} | Format-List -Property Thumbprint`)
		if err != nil {
			return nil, err
		}
		winrm := strings.TrimPrefix(strings.TrimSpace(winrmThumb.Stdout), "Thumbprint : ")
		if winrm == "" {
			return nil, fmt.Errorf("Could not find winrm cert thumbprint, got %s from cert store query", winrmThumb.Stdout)
		}
		winrm = "winrm " + winrm
		rdpThumb, err := RunPowershellCmd(`Get-ChildItem 'Cert:\LocalMachine\Remote Desktop\' | Where-Object {$_.Subject -eq "CN=$(hostname)"} | Format-List -Property Thumbprint`)
		if err != nil {
			return nil, err
		}
		rdp := strings.TrimPrefix(strings.TrimSpace(rdpThumb.Stdout), "Thumbprint : ")
		if rdp == "" {
			return nil, fmt.Errorf("Could not find rdp cert thumbprint, got %s from cert store query", rdpThumb.Stdout)
		}
		rdp = "rdp " + rdp
		totalBytes = []byte(winrm + "\n" + rdp)
	}
	return totalBytes, nil
}

// ParseHostKey parse hostkey data from bytes.
func ParseHostKey(bytes []byte) (map[string]string, error) {
	hostkeyLines := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(hostkeyLines) == 0 {
		return nil, fmt.Errorf("hostkey does not exist")
	}
	hostkeyMap := make(map[string]string)
	for _, hostkey := range hostkeyLines {
		hostkey = strings.TrimSuffix(hostkey, "\r")
		splits := strings.Split(hostkey, " ")
		if len(splits) < 2 {
			return nil, fmt.Errorf("hostkey has wrong format %s", hostkey)
		}
		keyType := strings.Split(hostkey, " ")[0]
		keyValue := strings.Split(hostkey, " ")[1]
		hostkeyMap[keyType] = keyValue
	}
	return hostkeyMap, nil
}

// GetDaisyClient returns a daisy compute client with the correct compute endpoint.
func GetDaisyClient(ctx context.Context) (daisyCompute.Client, error) {
	computeEndpoint, err := GetMetadata(ctx, "instance", "attributes", "_compute_endpoint")
	if err != nil {
		return nil, fmt.Errorf("failed to get compute endpoint: %v", err)
	}
	if computeEndpoint == "" {
		return daisyCompute.NewClient(ctx)
	}
	return daisyCompute.NewClient(ctx, option.WithEndpoint(computeEndpoint))
}

// GetProjectZone gets the project and zone of the instance.
func GetProjectZone(ctx context.Context) (string, string, error) {
	projectZone, err := GetMetadata(ctx, "instance", "zone")
	if err != nil {
		return "", "", fmt.Errorf("failed to get instance zone: %v", err)
	}
	projectZoneSplit := strings.Split(string(projectZone), "/")
	project := projectZoneSplit[1]
	zone := projectZoneSplit[3]
	return project, zone, nil
}

// GetInstanceName gets the instance name.
func GetInstanceName(ctx context.Context) (string, error) {
	name, err := GetMetadata(ctx, "instance", "name")
	if err != nil {
		return "", fmt.Errorf("failed to get instance name: %v", err)
	}
	return name, nil
}

// AccessSecret accesses the given secret.
func AccessSecret(ctx context.Context, client *secretmanager.Client, secretName string) (string, error) {
	// Get project
	project, _, err := GetProjectZone(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get project: %v", err)
	}

	// Make request call to Secret Manager.
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secretName),
	}
	resp, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %v", err)
	}
	return string(resp.Payload.Data), nil
}

// CreateClient create a ssh client to connect host.
func CreateClient(user, host string, pembytes []byte) (*ssh.Client, error) {
	// generate signer instance from plain key
	signer, err := ssh.ParsePrivateKey(pembytes)
	if err != nil {
		return nil, fmt.Errorf("parsing plain private key failed %v", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
	}
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// GetInterfaceByMAC returns the interface with the specified MAC address.
func GetInterfaceByMAC(mac string) (net.Interface, error) {
	hwaddr, err := net.ParseMAC(mac)
	if err != nil {
		return net.Interface{}, err
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}

	for _, iface := range interfaces {
		if iface.HardwareAddr.String() == hwaddr.String() {
			return iface, nil
		}
	}
	return net.Interface{}, fmt.Errorf("no interface found with MAC %s", mac)
}

// GetInterface returns the interface corresponding to the metadata interface array at the specified index.
func GetInterface(ctx context.Context, index int) (net.Interface, error) {
	mac, err := GetMetadata(ctx, "instance", "network-interfaces", fmt.Sprintf("%d", index), "mac")
	if err != nil {
		return net.Interface{}, err
	}

	return GetInterfaceByMAC(mac)
}

// ParseInterfaceIPv4 parses the interface's IPv4 address.
func ParseInterfaceIPv4(iface net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return ipnet.IP, nil
		}
	}
	return nil, fmt.Errorf("no ipv4 address found for interface %s", iface.Name)
}

// FilterLoopbackTunnelingInterfaces filters the list of interfaces to contain
// only the actual NICs, removing the loopback and tunneling interfaces listed
// in skipInterfaces.
func FilterLoopbackTunnelingInterfaces(ifaces []net.Interface) []net.Interface {
	var filtered []net.Interface
	for _, iface := range ifaces {
		// Remove the loopback interface.
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Remove the tunneling interfaces listed in skipInterfaces.
		skip := false
		for _, skipIface := range skipInterfaces {
			if strings.Contains(strings.ToLower(iface.Name), skipIface) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, iface)
		}
	}
	return filtered
}

// CheckLinuxCmdExists checks that a command exists on the linux image, and is executable.
func CheckLinuxCmdExists(cmd string) bool {
	cmdPath, err := exec.LookPath(cmd)
	// returns nil prior to go 1.19, exec.ErrDot after
	if errors.Is(err, exec.ErrDot) || err == nil {
		cmdFileInfo, err := os.Stat(cmdPath)
		cmdFileMode := cmdFileInfo.Mode()
		// check the the file has executable permissions.
		if err == nil {
			return cmdFileMode&0111 != 0
		}
	}
	return false
}

// IsAccelerator returns true if the image is an accelerator image.
func IsAccelerator(image string) bool {
	return strings.Contains(image, "nvidia")
}

// IsAlmaLinux returns true if the image is AlmaLinux.
func IsAlmaLinux(image string) bool {
	return strings.Contains(image, "almalinux")
}

// IsBYOS returns true if the image is BYOS(Bring Your Own Service).
func IsBYOS(image string) bool {
	return strings.Contains(image, "byos")
}

// IsCOS returns true if the image is cos.
func IsCOS(image string) bool {
	return strings.Contains(image, "cos")
}

// IsCentOS returns true if the image is CentOS.
func IsCentOS(image string) bool {
	return strings.Contains(image, "centos")
}

// IsDebian returns true if the image is Debian.
func IsDebian(image string) bool {
	return strings.Contains(image, "debian")
}

// IsEL returns true if the image is an EL image.
func IsEL(image string) bool {
	if IsCentOS(image) || IsRHEL(image) || IsRocky(image) || IsAlmaLinux(image) || IsOracle(image) {
		return true
	}
	return false
}

// IsFedora returns true if the image is Fedora Linux.
func IsFedora(image string) bool {
	return strings.Contains(image, "fedora")
}

// IsOpenSUSE returns true if the image is OpenSUSE.
func IsOpenSUSE(image string) bool {
	return strings.Contains(image, "opensuse")
}

// IsOracle returns true if the image is Oracle.
func IsOracle(image string) bool {
	return strings.Contains(image, "oracle")
}

// IsRHEL returns true if the image is RHEL.
func IsRHEL(image string) bool {
	return strings.Contains(image, "rhel")
}

// IsRHELEUS returns true if the image is RHEL EUS.
func IsRHELEUS(image string) bool {
	return strings.Contains(image, "eus")
}

// IsRHELLVM returns true if the image is RHEL LVM.
func IsRHELLVM(image string) bool {
	return strings.Contains(image, "lvm")
}

// IsRocky returns true if the image is Rocky.
func IsRocky(image string) bool {
	return strings.Contains(image, "rocky")
}

// IsSAP returns true if the image is SAP.
func IsSAP(image string) bool {
	return strings.Contains(image, "sap")
}

// IsSLES returns true if the image is SLES.
func IsSLES(image string) bool {
	return strings.Contains(image, "sles")
}

// IsSUSE returns true if the image is SUSE.
func IsSUSE(image string) bool {
	return strings.Contains(image, "suse")
}

// IsUbuntu returns true if the image is Ubuntu.
func IsUbuntu(image string) bool {
	return strings.Contains(image, "ubuntu")
}

// IsWindowsImage returns true if the image is Windows.
func IsWindowsImage(image string) bool {
	return strings.Contains(image, "windows")
}

// IsWindowsSQLImage returns true if the image is Windows SQL.
func IsWindowsSQLImage(image string) bool {
	return strings.Contains(image, "sql")
}

// LinuxOnly skips tests not on Linux.
func LinuxOnly(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only run on linux.")
	}
}

// WindowsOnly skips tests not on Windows.
func WindowsOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Test only run on Windows.")
	}
}

// SkipWindowsClientImages skips tests on Windows Client Images.
func SkipWindowsClientImages(t *testing.T) {
	image, err := GetMetadata(Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}
	if IsWindowsClient(image) {
		t.Skip("Tests do not run on Windows Client Images.")
	}
}

// Is32BitWindows returns true if the image contains -x86.
func Is32BitWindows(image string) bool {
	return strings.Contains(image, "-x86")
}

// Skip32BitWindows skips tests on 32-bit client images.
func Skip32BitWindows(t *testing.T, skipMsg string) {
	image, err := GetMetadata(Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata: %v", err)
	}

	if Is32BitWindows(image) {
		t.Skip(skipMsg)
	}
}

// IsWindows returns true if the detected runtime environment is Windows.
func IsWindows() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return false
}

// IsWindowsClient returns true if the image is a client (non-server) Windows image.
func IsWindowsClient(image string) bool {
	for _, pattern := range windowsClientImagePatterns {
		if strings.Contains(image, pattern) {
			return true
		}
	}
	return false
}

// WindowsContainersOnly skips tests not on Windows "for Containers" images.
func WindowsContainersOnly(t *testing.T) {
	WindowsOnly(t)
	image, err := GetMetadata(Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata: %v", err)
	}

	if !strings.Contains(image, "-for-containers") {
		t.Skip("Test only run on Windows for Containers images")
	}
}

// ProcessStatus holds stdout, stderr and the exit code from an external command call.
type ProcessStatus struct {
	Stdout   string
	Stderr   string
	Exitcode int
}

// RunPowershellCmd runs a powershell command and returns stdout and stderr if successful.
func RunPowershellCmd(command string) (ProcessStatus, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := ProcessStatus{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Exitcode: cmd.ProcessState.ExitCode(),
	}

	return output, err
}

// CheckPowershellSuccess returns an error if the powershell command fails.
func CheckPowershellSuccess(command string) error {
	output, err := RunPowershellCmd(command)
	if err != nil {
		return err
	}

	if output.Exitcode != 0 {
		return fmt.Errorf("Non-zero exit code: %d", output.Exitcode)
	}

	return nil
}

// CheckPowershellReturnCode returns an error if the exit code doesn't match the expected value.
func CheckPowershellReturnCode(command string, want int) error {
	output, _ := RunPowershellCmd(command)

	if output.Exitcode == want {
		return nil
	}

	return fmt.Errorf("Exit Code not as expected: want %d, got %d", want, output.Exitcode)

}

// FailOnPowershellFail fails the test if the powershell command fails.
func FailOnPowershellFail(command string, errorMsg string, t *testing.T) {
	err := CheckPowershellSuccess(command)
	if err != nil {
		t.Fatalf("%s: %v", errorMsg, err)
	}
}

// GetMountDiskPartition runs lsblk to get the partition of the mount disk on linux, assuming the
// size of the mount disk is diskExpectedSizeGb.
func GetMountDiskPartition(diskExpectedSizeGB int) (string, error) {
	var diskExpectedSizeBytes int64 = int64(diskExpectedSizeGB) * int64(bytesInGB)
	lsblkCmd := "lsblk"
	if !CheckLinuxCmdExists(lsblkCmd) {
		return "", fmt.Errorf("could not find lsblk")
	}
	diskType := "disk"
	lsblkout, err := exec.Command(lsblkCmd, "-b", "-o", "name,size,type", "--json").CombinedOutput()
	if err != nil {
		errorString := err.Error()
		// execute lsblk without json as a backup
		lsblkout, err = exec.Command(lsblkCmd, "-b", "-o", "name,size,type").CombinedOutput()
		if err != nil {
			errorString += err.Error()
			return "", fmt.Errorf("failed to execute lsblk with and without json: %s", errorString)
		}
		lsblkoutlines := strings.Split(string(lsblkout), "\n")
		for _, line := range lsblkoutlines {
			linetokens := strings.Fields(line)
			if len(linetokens) != 3 {
				continue
			}
			// we should have a slice of length 3, with fields name, size, type. Search for the line with the partition of the correct size.
			blkname, blksize, blktype := linetokens[0], linetokens[1], linetokens[2]
			blksizeInt, err := strconv.ParseInt(blksize, 10, 64)
			if err != nil {
				continue
			}
			if blktype == diskType && blksizeInt == diskExpectedSizeBytes {
				return blkname, nil
			}
		}
		return "", fmt.Errorf("failed to find disk partition with expected size %d", diskExpectedSizeBytes)
	}

	var blockDevices BlockDeviceList
	if err := json.Unmarshal(lsblkout, &blockDevices); err != nil {
		return "", fmt.Errorf("failed to unmarshal lsblk output %s with error: %v", lsblkout, err)
	}
	for _, blockDev := range blockDevices.BlockDevices {
		// deal with the case where the unmarshalled size field can be a number with or without quotes on different operating systems.
		blockDevSizeInt, err := blockDev.Size.Int64()
		if err != nil {
			return "", fmt.Errorf("block dev size %s was not an int: error %v", blockDev.Size.String(), err)
		}
		if strings.ToLower(blockDev.Type) == diskType && blockDevSizeInt == diskExpectedSizeBytes {
			return blockDev.Name, nil
		}
	}

	return "", fmt.Errorf("disk block with size not found")
}

// GetMountDiskPartitionSymlink uses symlinks to get the partition of the mount disk
// on linux, assuming the name of the disk resource is mountDiskName.
func GetMountDiskPartitionSymlink(mountDiskName string) (string, error) {
	mountDiskSymlink := "/dev/disk/by-id/google-" + mountDiskName
	symlinkRealPath, err := filepath.EvalSymlinks(mountDiskSymlink)
	if err != nil {
		return "", fmt.Errorf("symlink could not be resolved with error: %v", err)
	}
	return symlinkRealPath, nil
}

// HasFeature reports whether a compute.Image has a GuestOsFeature tag.
func HasFeature(img *compute.Image, feature string) bool {
	for _, f := range img.GuestOsFeatures {
		if f.Type == feature {
			return true
		}
	}
	return false
}

// Context returns a context to be used by test implementations, it handles
// the context cancellation based on the test's timeout(deadline), if no timeout
// is defined (or the deadline can't be assessed) then a plain background context
// is returned.
func Context(t *testing.T) context.Context {
	// If the test has a deadline defined use it as a last resort
	// context cancelation.
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.NewTimer(time.Until(deadline))
		go func() {
			<-timer.C
			cancel()
		}()
		return ctx
	}

	// If there's not deadline defined then we just use a
	// plain background context as we won't need to cancel it.
	return context.Background()
}

// ValidWindowsPassword returns a random password of the given length which
// meets Windows complexity requirements.
func ValidWindowsPassword(userPwLgth int) string {
	var pwLgth int
	minPwLgth := 14
	maxPwLgth := 255
	lower := []byte("abcdefghijklmnopqrstuvwxyz")
	upper := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	numbers := []byte("0123456789")
	special := []byte(`~!@#$%^&*_-+=|(){}[]:;<>,.?`)
	chars := bytes.Join([][]byte{lower, upper, numbers, special}, nil)
	pwLgth = minPwLgth
	if userPwLgth > minPwLgth {
		pwLgth = userPwLgth
	}
	if userPwLgth > maxPwLgth {
		pwLgth = maxPwLgth
	}

	for {
		b := make([]byte, pwLgth)
		for i := range b {
			ci, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
			if err != nil {
				continue
			}
			b[i] = chars[ci.Int64()]
		}

		var l, u, n, s int
		if bytes.ContainsAny(lower, string(b)) {
			l = 1
		}
		if bytes.ContainsAny(upper, string(b)) {
			u = 1
		}
		if bytes.ContainsAny(numbers, string(b)) {
			n = 1
		}
		if bytes.ContainsAny(special, string(b)) {
			s = 1
		}
		// If the password does not meet Windows complexity requirements, try again.
		// https://technet.microsoft.com/en-us/library/cc786468
		if l+u+n+s >= 3 {
			return string(b)
		}
	}
}

// FileType represents the type of file to check.
type FileType int

const (
	// TypeFile represents a file.
	TypeFile FileType = iota
	// TypeDir represents a directory.
	TypeDir
)

// Exists checks if a file or directory exists.
func Exists(path string, fileType FileType) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	switch fileType {
	case TypeFile:
		return !fileInfo.IsDir()
	case TypeDir:
		return fileInfo.IsDir()
	default:
		return false
	}
}

// UpsertMetadata inserts or updates a metadata entry on a currently running
// instance.
func UpsertMetadata(ctx context.Context, key, value string) error {
	client, err := GetDaisyClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get daisy client: %w", err)
	}

	prj, zone, err := GetProjectZone(ctx)
	if err != nil {
		return fmt.Errorf("failed to get project zone: %w", err)
	}

	name, err := GetMetadata(ctx, "instance", "name")
	if err != nil {
		return fmt.Errorf("failed to get instance name: %w", err)
	}

	inst, err := client.GetInstance(prj, zone, name)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	updated := false
	for _, item := range inst.Metadata.Items {
		if item.Key == key {
			item.Value = &value
			updated = true
			break
		}
	}

	// If the key is not found, append it to the metadata items.
	if !updated {
		inst.Metadata.Items = append(inst.Metadata.Items, &compute.MetadataItems{Key: key, Value: &value})
	}

	err = client.SetInstanceMetadata(prj, zone, name, inst.Metadata)
	if err != nil {
		return fmt.Errorf("failed to set instance metadata: %w", err)
	}

	return nil
}

// IfOldAgentInstalled returns true if the old agent is installed.
func IfOldAgentInstalled() bool {
	var oldggactl string
	if IsWindows() {
		oldggactl = `C:\Program Files\Google\Compute Engine\agent\ggactl_plugin_cleanup.exe`
	} else {
		oldggactl = "/usr/bin/ggactl_plugin_cleanup"
	}
	return Exists(oldggactl, TypeFile)
}

// IsCoreDisabled returns true if the core plugin is disabled by the config file.
func IsCoreDisabled() bool {
	var file string
	if IsWindows() {
		file = `C:\ProgramData\Google\Compute Engine\google-guest-agent\core-plugin-enabled`
	} else {
		file = "/etc/google-guest-agent/core-plugin-enabled"
	}
	content, err := os.ReadFile(file)
	if err != nil {
		// This file was present only when core plugin was disabled.
		return IfOldAgentInstalled()
	}
	return strings.Contains(string(content), "enabled=false")
}

// RestartAgent restarts the guest agent on the instance.
func RestartAgent(ctx context.Context) error {
	var cmd *exec.Cmd
	var ggactl string
	var wait bool
	if IsWindows() {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NonInteractive", "Restart-Service", "GCEAgent")
		ggactl = `C:\Program Files\Google\Compute Engine\agent\ggactl_plugin.exe`
	} else {
		cmd = exec.CommandContext(ctx, "systemctl", "restart", "google-guest-agent")
		ggactl = "/usr/bin/ggactl_plugin"
	}

	if Exists(ggactl, TypeFile) && !IsCoreDisabled() {
		cmd = exec.CommandContext(ctx, ggactl, "coreplugin", "restart")
		wait = true
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not restart agent: %w, output: %s", err, string(output))
	}

	if wait {
		// Wait for the core plugin to be restarted by plugin manager.
		time.Sleep(time.Second * 10)
	}
	return nil
}

// InstallPackage installs the given packages on the instance.
//
// It supports apt, yum, dnf, and zypper package managers. Returns ErrNotFound
// if none of the package managers are found.
func InstallPackage(packages ...string) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages to install")
	}

	if CheckLinuxCmdExists("apt") {
		args := []string{"install", "-y"}
		args = append(args, packages...)
		return exec.Command("apt", args...).Run()
	}
	if CheckLinuxCmdExists("yum") {
		args := []string{"install", "-y"}
		args = append(args, packages...)
		return exec.Command("yum", args...).Run()
	}
	if CheckLinuxCmdExists("dnf") {
		args := []string{"install", "-y"}
		args = append(args, packages...)
		return exec.Command("dnf", args...).Run()
	}

	// Zypper may fail because it fails instead of waiting for the package
	// lock to be released.
	// TODO(andrewhl): Determine if we need this, or if we can either remove it
	// or find a way to make it more reliable.
	if CheckLinuxCmdExists("zypper") {
		args := []string{"--non-interactive", "install"}
		args = append(args, packages...)
		return exec.Command("zypper", args...).Run()
	}
	return ErrPackageManagersNotFound
}

// ProcessExistsWindows checks if the process exists on Windows.
func ProcessExistsWindows(t *testing.T, shouldExist bool, processName string) {
	t.Helper()

	status, err := RunPowershellCmd(fmt.Sprintf("Get-Process -Name %s", processName))
	if (err != nil) != !shouldExist {
		t.Fatalf("Failed to run Get-Process powershell command: %v, process status: %+v, shouldExist: %t", err, status, shouldExist)
	}
}

// ProcessExistsLinux checks if the process exists on Linux.
func ProcessExistsLinux(t *testing.T, shouldExist bool, processName string) {
	t.Helper()

	cmd := exec.Command("ps", "-e", "-o", "command")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run ps command: %v", err)
	}

	output := string(out)
	processes := strings.Split(output, "\n")
	found := false
	for _, process := range processes {
		cmd := strings.Split(strings.TrimSpace(process), " ")
		if cmd[0] == processName {
			found = true
			break
		}
	}

	if found != shouldExist {
		t.Fatalf("Process %q expected to exist: %t, got: %t\nOutput:\n %s", processName, shouldExist, found, output)
	}
}

// ParseStderr returns the stderr output from an exec.ExitError, or the error
// string if the error is not an exec.ExitError.
func ParseStderr(err error) string {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return err.Error()
	}
	return string(exitErr.Stderr)
}

// CopyFile copies a file from the source to the destination.
func CopyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

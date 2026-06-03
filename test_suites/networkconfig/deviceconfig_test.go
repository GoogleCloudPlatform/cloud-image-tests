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
	_ "embed"
	"fmt"
	fs "io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	pb "github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkconfig/config_expectations"
)

var (
	virtioRxIRQRe = regexp.MustCompile(`virtio\d+-input\.(\d+)`)
	virtioTxIRQRe = regexp.MustCompile(`virtio\d+-output\.(\d+)`)
)

//go:embed config_expectations.textproto
var configExpectationsBytes []byte

type systemKey struct {
	machineType string
	nicTypes    []string
}

func matchKey(a, b *systemKey) bool {
	if a == nil || b == nil {
		return false
	}
	if a.machineType != b.machineType {
		return false
	}
	if len(a.nicTypes) != len(b.nicTypes) {
		return false
	}
	for i, nicType := range a.nicTypes {
		if nicType != b.nicTypes[i] {
			return false
		}
	}
	return true
}

func expectedConfigForMachine(configExpectations *pb.ConfigExpectations, machineType string, nicTypes []string) (*pb.SystemConfig, error) {
	thisSystemKey := &systemKey{machineType: machineType, nicTypes: nicTypes}
	for _, config := range configExpectations.GetConfigExpectations() {
		var configNicTypes []string
		for _, nic := range config.GetNics() {
			configNicTypes = append(configNicTypes, nic.GetType())
		}
		configSystemKey := &systemKey{
			machineType: config.GetMachineType(),
			nicTypes:    configNicTypes,
		}
		if matchKey(configSystemKey, thisSystemKey) {
			return config, nil
		}
	}
	return nil, fmt.Errorf("no config expectation found for machine type %q and nic types %v", machineType, nicTypes)
}

func deviceIRQs(ifaceName string) ([]int, error) {
	deviceRealPath, err := filepath.EvalSymlinks(fmt.Sprintf("/sys/class/net/%s/device", ifaceName))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve device path for %q: %v", ifaceName, err)
	}

	// virtio devices have the MSI interrupts in a subdirectory of the parent directory,
	// while most other devices have them in a subdirectory of the device itself.
	var deviceIRQsPath string
	candidatePaths := []string{
		filepath.Join(deviceRealPath, "msi_irqs"),
		filepath.Join(deviceRealPath, "..", "msi_irqs"),
	}
	for _, path := range candidatePaths {
		if utils.Exists(path, utils.TypeDir) {
			deviceIRQsPath = path
			break
		}
	}
	if deviceIRQsPath == "" {
		return nil, fmt.Errorf("device IRQs file does not exist for %q", ifaceName)
	}

	var irqs []int
	err = filepath.WalkDir(deviceIRQsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == deviceIRQsPath {
			return nil
		}

		baseAsInt, err := strconv.Atoi(d.Name())
		if err != nil {
			return fmt.Errorf("unexpected non-integer device IRQ name %q: %w", d.Name(), err)
		}

		allegedProcPath := fmt.Sprintf("/proc/irq/%d", baseAsInt)
		if !utils.Exists(allegedProcPath, utils.TypeDir) {
			// This is a normal state in the kernel. It just indicates that we overallocated MSI
			// interrupts, and are only using a subset of them.
			return nil
		}
		irqs = append(irqs, baseAsInt)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking device IRQs path %q: %w", deviceIRQsPath, err)
	}

	return irqs, nil
}

// findFileWithRegex walks the given directory, returning the first match group for an arbitrary
// matching file name in dirPath. Returns an empty string if no match is found, or an error if
// the walk fails.
func findFileWithRegex(dirPath string, r *regexp.Regexp) (string, error) {
	ret := ""
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		matches := r.FindStringSubmatch(d.Name())
		if len(matches) < 2 {
			return nil
		}
		ret = matches[1]
		return filepath.SkipAll
	})
	if err != nil {
		return "", err
	}
	return ret, nil
}

// rxQueueIndex returns the index of the RX queue for the given IRQ path.
// Returns -1 if no RX queue is found, or an error if the walk fails.
func rxQueueIndex(irqPath string) (int, error) {
	fileName, err := findFileWithRegex(irqPath, virtioRxIRQRe)
	if err != nil {
		return 0, err
	}
	if fileName == "" {
		return -1, nil
	}
	return strconv.Atoi(fileName)
}

// txQueueIndex returns the index of the TX queue for the given IRQ path.
// Returns -1 if no TX queue is found, or an error if the walk fails.
func txQueueIndex(irqPath string) (int, error) {
	fileName, err := findFileWithRegex(irqPath, virtioTxIRQRe)
	if err != nil {
		return 0, err
	}
	if fileName == "" {
		return -1, nil
	}
	return strconv.Atoi(fileName)
}

type irqCPULists struct {
	rxIRQAffinity map[int]string
	txIRQAffinity map[int]string
}

func queueIndexToIRQs(irqs []int) (*irqCPULists, error) {
	irqCPUListsMap := &irqCPULists{
		rxIRQAffinity: make(map[int]string),
		txIRQAffinity: make(map[int]string),
	}
	for _, irqNumber := range irqs {
		path := fmt.Sprintf("/proc/irq/%d", irqNumber)
		if !utils.Exists(path, utils.TypeDir) {
			return nil, fmt.Errorf("IRQ %d does not have a corresponding directory %q", irqNumber, path)
		}

		cpusetFile := filepath.Join(path, "smp_affinity")
		if !utils.Exists(cpusetFile, utils.TypeFile) {
			return nil, fmt.Errorf("IRQ %d does not have a corresponding file %q", irqNumber, cpusetFile)
		}
		cpusetBytes, err := os.ReadFile(cpusetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read %q: %w", cpusetFile, err)
		}
		cpuset, err := parseHexMask(string(cpusetBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to parse smp_affinity for IRQ %d: %w", irqNumber, err)
		}
		cpuListStr := cpuListString(cpuset)

		rxQueueIndex, err := rxQueueIndex(path)
		if err != nil {
			return nil, err
		}
		txQueueIndex, err := txQueueIndex(path)
		if err != nil {
			return nil, err
		}

		if rxQueueIndex >= 0 {
			irqCPUListsMap.rxIRQAffinity[rxQueueIndex] = cpuListStr
		}
		if txQueueIndex >= 0 {
			irqCPUListsMap.txIRQAffinity[txQueueIndex] = cpuListStr
		}
	}
	return irqCPUListsMap, nil
}

func deviceNICType(ifaceName string) (string, error) {
	cmd := exec.Command("ethtool", "-i", ifaceName)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run `ethtool -i %q`: %w", ifaceName, err)
	}
	outStr := string(out)
	matches := ethtoolDriverRe.FindStringSubmatch(outStr)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to find driver in `ethtool -i %q` output: %q", ifaceName, outStr)
	}
	driver := matches[1]

	switch driver {
	case "virtio_net":
		return nicTypeVIRTIONET, nil
	case "gve":
		return nicTypeGVNIC, nil
	case "gvnic":
		return nicTypeGVNIC, nil
	case "idpf":
		{
			// This driver is used for both IDPF and IRDMA, but only IRDMA has the
			// infiniband subdirectory. This is the same mechanism used by set_multiqueue.
			hasInfiniband := utils.Exists(fmt.Sprintf("/sys/class/net/%s/device/infiniband", ifaceName), utils.TypeDir)
			if hasInfiniband {
				return nicTypeIRDMA, nil
			}
			return nicTypeIDPF, nil
		}
	default:
		return "", fmt.Errorf("unknown driver type %q", driver)
	}
}

func queueIndexToXPS(nicName string) (map[int]string, error) {
	txQueueToXPS := make(map[int]string)
	queuesPath := fmt.Sprintf("/sys/class/net/%s/queues", nicName)
	err := filepath.WalkDir(queuesPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if !strings.Contains(d.Name(), "tx") {
			return nil
		}

		nameParts := strings.Split(d.Name(), "-")
		if len(nameParts) != 2 {
			return fmt.Errorf("unexpected tx queue name %q", d.Name())
		}
		queueIndex, err := strconv.Atoi(nameParts[1])
		if err != nil {
			return fmt.Errorf("unexpected non-integer tx queue name %q: %w", d.Name(), err)
		}

		xpsCpusFile := filepath.Join(path, "xps_cpus")
		if !utils.Exists(xpsCpusFile, utils.TypeFile) {
			return nil
		}
		xpsCPUSetBytes, err := os.ReadFile(xpsCpusFile)
		if err != nil {
			return fmt.Errorf("failed to read xps_cpus for %q: %w", d.Name(), err)
		}

		xpsCPUSet, err := parseHexMask(string(xpsCPUSetBytes))
		if err != nil {
			return fmt.Errorf("failed to parse xps_cpus for %q: %w", d.Name(), err)
		}
		xpsCPUListStr := cpuListString(xpsCPUSet)

		txQueueToXPS[queueIndex] = xpsCPUListStr
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking queues path %q: %w", queuesPath, err)
	}
	return txQueueToXPS, nil
}

func thisSystemConfig(mdsIfaces []networkInterface) (*pb.SystemConfig, error) {
	var nics []*pb.NicExpectation
	for _, mdsIface := range mdsIfaces {
		systemIface, err := utils.GetInterfaceByMAC(mdsIface.MAC)
		if err != nil {
			return nil, fmt.Errorf("getting interface for MAC %q: %w", mdsIface.MAC, err)
		}
		nicName := systemIface.Name

		nicType, err := deviceNICType(nicName)
		if err != nil {
			return nil, fmt.Errorf("getting NIC type for %q: %w", nicName, err)
		}

		irqs, err := deviceIRQs(nicName)
		if err != nil {
			return nil, fmt.Errorf("getting device IRQs for %q: %w", nicName, err)
		}
		queueToIRQs, err := queueIndexToIRQs(irqs)
		if err != nil {
			return nil, fmt.Errorf("converting IRQs to queue index to IRQs for %q: %w", nicName, err)
		}

		txQueueToXPS, err := queueIndexToXPS(nicName)
		if err != nil {
			return nil, fmt.Errorf("getting XPS for %q: %w", nicName, err)
		}

		if len(queueToIRQs.txIRQAffinity) != len(txQueueToXPS) {
			return nil, fmt.Errorf("number of tx queues in IRQs (%d) does not match number of tx queues in XPS (%d)", len(queueToIRQs.txIRQAffinity), len(txQueueToXPS))
		}

		var txQueues []*pb.TxQueue
		for txQueueIndex, irqList := range queueToIRQs.txIRQAffinity {
			txQueues = append(txQueues, pb.TxQueue_builder{
				Index:      proto.Int32(int32(txQueueIndex)),
				IrqCpulist: proto.String(irqList),
				XpsCpulist: proto.String(txQueueToXPS[txQueueIndex]),
			}.Build())
		}

		var rxQueues []*pb.RxQueue
		for rxQueueIndex, irqList := range queueToIRQs.rxIRQAffinity {
			rxQueues = append(rxQueues, pb.RxQueue_builder{
				Index:      proto.Int32(int32(rxQueueIndex)),
				IrqCpulist: proto.String(irqList),
			}.Build())
		}

		nic := pb.NicExpectation_builder{
			Type:     proto.String(nicType),
			TxQueues: txQueues,
			RxQueues: rxQueues,
		}.Build()
		nics = append(nics, nic)
	}

	return pb.SystemConfig_builder{
		Nics: nics,
	}.Build(), nil
}

func TestDeviceConfig(t *testing.T) {
	ctx := utils.Context(t)

	var configExpectations pb.ConfigExpectations
	if err := prototext.Unmarshal(configExpectationsBytes, &configExpectations); err != nil {
		t.Fatalf("failed to unmarshal embedded config expectations: %v", err)
	}

	mdsIfaces, err := listMDSIfaces(ctx)
	if err != nil {
		t.Fatalf("listMDSIfaces(ctx) = err %v want nil", err)
	}
	if len(mdsIfaces) == 0 {
		t.Fatalf("no network interfaces found in metadata")
	}

	machineTypePath, err := utils.GetMetadata(ctx, "instance", "machine-type")
	if err != nil {
		t.Fatalf("failed to get machine type: %v", err)
	}
	machineType := filepath.Base(machineTypePath)

	var nicTypes []string
	for _, mdsIface := range mdsIfaces {
		nicTypes = append(nicTypes, mdsIface.NICType)
	}

	wantSystemConfig, err := expectedConfigForMachine(&configExpectations, machineType, nicTypes)
	if err != nil {
		t.Fatalf("expectedConfigForMachine(&configExpectations, %q, %v) = err %v want nil", machineType, nicTypes, err)
	}

	gotSystemConfig, err := thisSystemConfig(mdsIfaces)
	if err != nil {
		t.Fatalf("thisSystemConfig(mdsIfaces) = err %v want nil", err)
	}

	if diff := cmp.Diff(wantSystemConfig, gotSystemConfig,
		protocmp.Transform(),
		protocmp.SortRepeatedFields(&pb.SystemConfig{}, "nics"),
		protocmp.SortRepeatedFields(&pb.NicExpectation{}, "tx_queues"),
		protocmp.SortRepeatedFields(&pb.NicExpectation{}, "rx_queues"),
		protocmp.IgnoreFields(&pb.SystemConfig{}, "description", "machine_type"),
	); diff != "" {
		t.Errorf("SystemConfig mismatch (-want +got):\n%s", diff)
	}
}

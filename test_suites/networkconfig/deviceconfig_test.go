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

// Package networkconfig is a CIT suite for testing the guest network configuration of machines.
package networkconfig

import (
	_ "embed"
	"fmt"
	fs "io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/networkutils"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	pb "github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkconfig/config_expectations"
)

var (
	virtioRXIRQRe = regexp.MustCompile(`virtio\d+-input\.(\d+)`)
	virtioTXIRQRe = regexp.MustCompile(`virtio\d+-output\.(\d+)`)

	// GVE IRQ names are different across different kernel versions, so check for both.
	//
	// Additionally, note that the decimal value doesn't directly map to the queue index generally.
	// The TX queues are directly mapped, but the RX queues are mapped after the last TX queue as
	// a function of total notify blocks.
	gveNotifyIRQRe      = regexp.MustCompile(`gve-ntfy-blk(\d+)@.*`)
	gveOlderNotifyIRQRe = regexp.MustCompile(`.*-ntfy-block\.(\d+)`)

	txQueueRe = regexp.MustCompile(`tx-(\d+)`)
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
	return slices.Equal(a.nicTypes, b.nicTypes)
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

func virtioRXQueueIndex(irqPath string) (found bool, index int, err error) {
	fileName, err := findFileWithRegex(irqPath, virtioRXIRQRe)
	if err != nil {
		return
	}
	if fileName == "" {
		return
	}
	index, err = strconv.Atoi(fileName)
	if err != nil {
		return
	}

	found = true
	return
}

func virtioTXQueueIndex(irqPath string) (bool, int, error) {
	fileName, err := findFileWithRegex(irqPath, virtioTXIRQRe)
	if err != nil {
		return false, 0, err
	}
	if fileName == "" {
		return false, 0, nil
	}
	index, err := strconv.Atoi(fileName)
	if err != nil {
		return false, 0, err
	}
	return true, index, nil
}

func gveQueueIndex(irqPath string, isRX bool, queueCounts *networkutils.EthtoolQueueCounts) (bool, int, error) {
	candidateRegexes := []*regexp.Regexp{gveNotifyIRQRe, gveOlderNotifyIRQRe}
	var fileName string
	for _, re := range candidateRegexes {
		if f, err := findFileWithRegex(irqPath, re); err != nil {
			return false, 0, err
		} else if f != "" {
			fileName = f
			break
		}
	}
	if fileName == "" {
		return false, 0, nil
	}
	index, err := strconv.Atoi(fileName)
	if err != nil {
		return false, 0, err
	}

	// Mathematically inverse the GVE driver's logic, which is roughly:
	//   gve_tx_idx_to_ntfy(queue_index) = queue_index
	//   gve_rx_idx_to_ntfy(queue_index) = (num_ntfy_blks / 2) + queue_index
	//
	// Note that the number of notify blocks is based on the maximum number of queues, not
	// the currently configured number.
	if isRX && index >= queueCounts.MaxTXQueues {
		index -= queueCounts.MaxTXQueues
		return true, index, nil
	}
	if !isRX && index < queueCounts.MaxTXQueues {
		return true, index, nil
	}

	return false, 0, nil
}

// rxQueueIndex returns the index of the RX queue for the given IRQ path.
// Returns -1 if no RX queue is found, or an error if the calculation fails unexpectedly.
func rxQueueIndex(irqPath string, queueCounts *networkutils.EthtoolQueueCounts) (int, error) {
	if found, index, err := virtioRXQueueIndex(irqPath); err != nil {
		return -1, err
	} else if found {
		return index, nil
	}

	if found, index, err := gveQueueIndex(irqPath, true /*isRX*/, queueCounts); err != nil {
		return -1, err
	} else if found {
		return index, nil
	}

	return -1, nil
}

// txQueueIndex returns the index of the TX queue for the given IRQ path.
// Returns -1 if no TX queue is found, or an error if the calculation fails unexpectedly.
func txQueueIndex(irqPath string, queueCounts *networkutils.EthtoolQueueCounts) (int, error) {
	if found, index, err := virtioTXQueueIndex(irqPath); err != nil {
		return -1, err
	} else if found {
		return index, nil
	}

	if found, index, err := gveQueueIndex(irqPath, false /*isRX*/, queueCounts); err != nil {
		return -1, err
	} else if found {
		return index, nil
	}

	return -1, nil
}

type irqCPULists struct {
	rxIRQAffinity map[int]string
	txIRQAffinity map[int]string
}

func queueIndexToIRQs(irqs []int, queueCounts *networkutils.EthtoolQueueCounts) (*irqCPULists, error) {
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
		cpuset, err := networkutils.ParseCpusetMask(string(cpusetBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to parse smp_affinity for IRQ %d: %w", irqNumber, err)
		}
		cpuListStr := cpuset.ListString()

		rxQueueIndex, err := rxQueueIndex(path, queueCounts)
		if err != nil {
			return nil, err
		}
		txQueueIndex, err := txQueueIndex(path, queueCounts)
		if err != nil {
			return nil, err
		}

		if rxQueueIndex >= 0 && rxQueueIndex < queueCounts.CurrentRXQueues {
			irqCPUListsMap.rxIRQAffinity[rxQueueIndex] = cpuListStr
		}
		if txQueueIndex >= 0 && txQueueIndex < queueCounts.CurrentTXQueues {
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
	matches := networkutils.EthtoolDriverRe.FindStringSubmatch(outStr)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to find driver in `ethtool -i %q` output: %q", ifaceName, outStr)
	}
	driver := matches[1]

	switch driver {
	case "virtio_net":
		return networkutils.NICTypeVIRTIONET, nil
	case "gve":
		return networkutils.NICTypeGVNIC, nil
	case "gvnic":
		return networkutils.NICTypeGVNIC, nil
	case "idpf":
		{
			// This driver is used for both IDPF and IRDMA, but only IRDMA has the
			// infiniband subdirectory. This is the same mechanism used by set_multiqueue.
			hasInfiniband := utils.Exists(fmt.Sprintf("/sys/class/net/%s/device/infiniband", ifaceName), utils.TypeDir)
			if hasInfiniband {
				return networkutils.NICTypeIRDMA, nil
			}
			return networkutils.NICTypeIDPF, nil
		}
	case "mlx5_core":
		return networkutils.NICTypeMRDMA, nil
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
		match := txQueueRe.FindStringSubmatch(d.Name())
		if len(match) != 2 {
			return nil
		}
		queueIndex, err := strconv.Atoi(match[1])
		if err != nil {
			return fmt.Errorf("unexpected non-integer tx queue name %q: %w", d.Name(), err)
		}

		xpsCpusFile := filepath.Join(path, "xps_cpus")
		xpsCPUSetBytes, err := os.ReadFile(xpsCpusFile)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read xps_cpus for %q: %w", d.Name(), err)
		}

		xpsCPUSet, err := networkutils.ParseCpusetMask(string(xpsCPUSetBytes))
		if err != nil {
			return fmt.Errorf("failed to parse xps_cpus for %q: %w", d.Name(), err)
		}
		xpsCPUListStr := xpsCPUSet.ListString()

		txQueueToXPS[queueIndex] = xpsCPUListStr
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking queues path %q: %w", queuesPath, err)
	}
	return txQueueToXPS, nil
}

func queueCountsForInterface(ifaceName string) (*networkutils.EthtoolQueueCounts, error) {
	cmd := exec.Command("ethtool", "-l", ifaceName)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run `ethtool -l %q`: %w", ifaceName, err)
	}
	return networkutils.ParseEthtoolLOutput(string(out))
}

func buildQueuesXPSOnly(txQueueToXPS map[int]string) ([]*pb.TxQueue, error) {
	var txQueues []*pb.TxQueue
	for txQueueIndex, xpsCPUListStr := range txQueueToXPS {
		txQueues = append(txQueues, pb.TxQueue_builder{
			Index:      proto.Int32(int32(txQueueIndex)),
			XpsCpulist: proto.String(xpsCPUListStr),
		}.Build())
	}
	return txQueues, nil
}

func buildQueues(nicType string, irqCPULists *irqCPULists, txQueueToXPS map[int]string) ([]*pb.TxQueue, []*pb.RxQueue, error) {
	// The guest environment don't configure IRQs for IRDMA and MRDMA.
	// We may wish to assert on this in the future (i.e. values from the driver), but for now we
	// ignore it.
	switch nicType {
	case networkutils.NICTypeIRDMA, networkutils.NICTypeMRDMA:
		txQueues, err := buildQueuesXPSOnly(txQueueToXPS)
		if err != nil {
			return nil, nil, err
		}
		return txQueues, nil, nil
	default:
		// continue
	}

	if len(irqCPULists.txIRQAffinity) != len(txQueueToXPS) {
		return nil, nil, fmt.Errorf("number of tx queues in IRQs (%d) does not match number of tx queues in XPS (%d)", len(irqCPULists.txIRQAffinity), len(txQueueToXPS))
	}

	var txQueues []*pb.TxQueue
	for txQueueIndex, irqList := range irqCPULists.txIRQAffinity {
		txQueues = append(txQueues, pb.TxQueue_builder{
			Index:      proto.Int32(int32(txQueueIndex)),
			IrqCpulist: proto.String(irqList),
			XpsCpulist: proto.String(txQueueToXPS[txQueueIndex]),
		}.Build())
	}

	var rxQueues []*pb.RxQueue
	for rxQueueIndex, irqList := range irqCPULists.rxIRQAffinity {
		rxQueues = append(rxQueues, pb.RxQueue_builder{
			Index:      proto.Int32(int32(rxQueueIndex)),
			IrqCpulist: proto.String(irqList),
		}.Build())
	}
	return txQueues, rxQueues, nil
}

func thisSystemConfig(mdsIfaces []networkutils.NetworkInterface) (*pb.SystemConfig, error) {
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

		queueCounts, err := queueCountsForInterface(nicName)
		if err != nil {
			return nil, fmt.Errorf("parsing ethtool -l output for %q: %w", nicName, err)
		}

		irqs, err := deviceIRQs(nicName)
		if err != nil {
			return nil, fmt.Errorf("getting device IRQs for %q: %w", nicName, err)
		}
		queueToIRQs, err := queueIndexToIRQs(irqs, queueCounts)
		if err != nil {
			return nil, fmt.Errorf("converting IRQs to queue index to IRQs for %q: %w", nicName, err)
		}

		txQueueToXPS, err := queueIndexToXPS(nicName)
		if err != nil {
			return nil, fmt.Errorf("getting XPS for %q: %w", nicName, err)
		}

		txQueues, rxQueues, err := buildQueues(nicType, queueToIRQs, txQueueToXPS)
		if err != nil {
			return nil, fmt.Errorf("building queues for %q: %w", nicName, err)
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

	mdsIfaces, err := networkutils.ListMDSIfaces(ctx)
	switch {
	case err != nil:
		t.Fatalf("ListMDSIfaces(ctx) = err %v want nil", err)
	case len(mdsIfaces) == 0:
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
		t.Fatalf("expectedConfigForMachine(&configExpectations, %q, %v) = err: %v, want nil", machineType, nicTypes, err)
	}

	gotSystemConfig, err := thisSystemConfig(mdsIfaces)
	if err != nil {
		t.Fatalf("thisSystemConfig(mdsIfaces) = err: %v, want nil", err)
	}

	if diff := cmp.Diff(wantSystemConfig, gotSystemConfig,
		protocmp.Transform(),
		protocmp.SortRepeatedFields(&pb.NicExpectation{}, "tx_queues"),
		protocmp.SortRepeatedFields(&pb.NicExpectation{}, "rx_queues"),
		protocmp.IgnoreFields(&pb.SystemConfig{}, "description", "machine_type"),
	); diff != "" {
		t.Errorf("SystemConfig mismatch (-want +got):\n%s", diff)
	}
}

// Package configexpectations provides tests for the config_expectations.textproto file.
package configexpectations

import (
	_ "embed"
	"fmt"
	"sort"
	"testing"

	pb "github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkconfig/config_expectations_proto"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/networkutils"
	"google.golang.org/protobuf/encoding/prototext"
)

//go:embed config_expectations.textproto
var configExpectationsBytes []byte

func proto() *pb.ConfigExpectations {
	configExpectations := &pb.ConfigExpectations{}
	if err := prototext.Unmarshal(configExpectationsBytes, configExpectations); err != nil {
		panic(fmt.Errorf("failed to unmarshal config expectations data: %v", err))
	}
	return configExpectations
}

func TestValidTextproto(t *testing.T) {
	configExpectations := &pb.ConfigExpectations{}
	if err := prototext.Unmarshal(configExpectationsBytes, configExpectations); err != nil {
		t.Fatalf("Failed to unmarshal config expectations data: %v", err)
	}
}

func TestUniqueDescriptions(t *testing.T) {
	c := proto()
	seenDescriptions := make(map[string]bool)
	for _, config := range c.GetConfigExpectations() {
		t.Run(config.GetDescription(), func(t *testing.T) {
			if seenDescriptions[config.GetDescription()] {
				t.Errorf("Duplicate description found: %q", config.GetDescription())
			}
			seenDescriptions[config.GetDescription()] = true
		})
	}
}

func TestQueueIndices(t *testing.T) {
	c := proto()
	for _, config := range c.GetConfigExpectations() {
		t.Run(config.GetDescription(), func(t *testing.T) {
			for _, nic := range config.GetNics() {
				seenTxIndices := make(map[int32]bool)
				seenRxIndices := make(map[int32]bool)

				for _, txQueue := range nic.GetTxQueues() {
					if txQueue.GetIndex() < 0 {
						t.Errorf("Negative TX queue index found: %v", txQueue)
					}
					if seenTxIndices[txQueue.GetIndex()] {
						t.Errorf("Duplicate TX queue index found: %v", txQueue)
					}
					seenTxIndices[txQueue.GetIndex()] = true
				}
				for _, rxQueue := range nic.GetRxQueues() {
					if rxQueue.GetIndex() < 0 {
						t.Errorf("Negative RX queue index found: %v", rxQueue)
					}
					if seenRxIndices[rxQueue.GetIndex()] {
						t.Errorf("Duplicate RX queue index found: %v", rxQueue)
					}
					seenRxIndices[rxQueue.GetIndex()] = true
				}

				var sortedTxIndices []int32
				for txIndex := range seenTxIndices {
					sortedTxIndices = append(sortedTxIndices, txIndex)
				}
				sort.Slice(sortedTxIndices, func(i, j int) bool {
					return sortedTxIndices[i] < sortedTxIndices[j]
				})
				for i, txIndex := range sortedTxIndices {
					if txIndex != int32(i) {
						t.Errorf("Non-sequential TX queue indices found, %d != %d", txIndex, i)
					}
				}

				var sortedRxIndices []int32
				for rxIndex := range seenRxIndices {
					sortedRxIndices = append(sortedRxIndices, rxIndex)
				}
				sort.Slice(sortedRxIndices, func(i, j int) bool {
					return sortedRxIndices[i] < sortedRxIndices[j]
				})
				for i, rxIndex := range sortedRxIndices {
					if rxIndex != int32(i) {
						t.Errorf("Non-sequential RX queue indices found, %d != %d", rxIndex, i)
					}
				}
			}
		})
	}
}

func TestCPUSets(t *testing.T) {
	c := proto()
	for _, config := range c.GetConfigExpectations() {
		t.Run(config.GetDescription(), func(t *testing.T) {
			for nicIdx, nic := range config.GetNics() {
				for _, txQueue := range nic.GetTxQueues() {
					irqCPUs := txQueue.GetIrqCpulist()
					xpsCPUs := txQueue.GetXpsCpulist()

					_, err := networkutils.ParseCpusetList(irqCPUs)
					if err != nil {
						t.Errorf("Failed to parse IRQ cpuset list for nic %d, tx queue %d: %q: %v", nicIdx, txQueue.GetIndex(), irqCPUs, err)
					}
					_, err = networkutils.ParseCpusetList(xpsCPUs)
					if err != nil {
						t.Errorf("Failed to parse XPS cpuset list for nic %d, tx queue %d: %q: %v", nicIdx, txQueue.GetIndex(), xpsCPUs, err)
					}
				}

				for _, rxQueue := range nic.GetRxQueues() {
					irqCPUs := rxQueue.GetIrqCpulist()
					_, err := networkutils.ParseCpusetList(irqCPUs)
					if err != nil {
						t.Errorf("Failed to parse IRQ cpuset list for nic %d, rx queue %d: %q: %v", nicIdx, rxQueue.GetIndex(), irqCPUs, err)
					}
				}
			}
		})
	}
}

func TestRingSizes(t *testing.T) {
	c := proto()
	for _, config := range c.GetConfigExpectations() {
		t.Run(config.GetDescription(), func(t *testing.T) {
			for nicIdx, nic := range config.GetNics() {
				if nic.HasRxRingSize() && nic.GetRxRingSize() <= 0 {
					t.Errorf("NIC %d: non-positive rx_ring_size %d", nicIdx, nic.GetRxRingSize())
				}
				if nic.HasTxRingSize() && nic.GetTxRingSize() <= 0 {
					t.Errorf("NIC %d: non-positive tx_ring_size %d", nicIdx, nic.GetTxRingSize())
				}
			}
		})
	}
}

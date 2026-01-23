// Copyright 2022 Google LLC.
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

// Package cleanerupper provides a library of functions to delete gcp resources
// in a project matching the given deletion policy.
package cleanerupper

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	osconfigV1alpha "cloud.google.com/go/osconfig/apiv1alpha"
	osconfig "cloud.google.com/go/osconfig/apiv1beta"
	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	osconfigv1alphapb "google.golang.org/genproto/googleapis/cloud/osconfig/v1alpha"
	osconfigpb "google.golang.org/genproto/googleapis/cloud/osconfig/v1beta"
)

const keepLabel = "do-not-delete"

// Clients contains all of the clients needed by cleanerupper functions.
type Clients struct {
	Daisy         daisyCompute.Client
	OSConfig      osconfigInterface
	OSConfigZonal osconfigZonalInterface
}

type osconfigInterface interface {
	ListGuestPolicies(context.Context, *osconfigpb.ListGuestPoliciesRequest, ...gax.CallOption) *osconfig.GuestPolicyIterator
	DeleteGuestPolicy(context.Context, *osconfigpb.DeleteGuestPolicyRequest, ...gax.CallOption) error
}

type osconfigZonalInterface interface {
	ListOSPolicyAssignments(context.Context, *osconfigv1alphapb.ListOSPolicyAssignmentsRequest, ...gax.CallOption) *osconfigV1alpha.OSPolicyAssignmentIterator
	DeleteOSPolicyAssignment(context.Context, *osconfigv1alphapb.DeleteOSPolicyAssignmentRequest, ...gax.CallOption) (*osconfigV1alpha.DeleteOSPolicyAssignmentOperation, error)
}

// NewClients initializes a struct of Clients for use by CleanX functions
func NewClients(ctx context.Context, opts ...option.ClientOption) (*Clients, error) {
	var c Clients
	var err error
	c.Daisy, err = daisyCompute.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	c.OSConfig, err = osconfig.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	c.OSConfigZonal, err = osconfigV1alpha.NewOsConfigZonalClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// PolicyFunc describes a function which takes a some resource and returns a
// bool indicating whether it should be deleted.
type PolicyFunc func(any) bool

// AgePolicy takes a time.Time and returns a PolicyFunc which indicates to
// delete anything older than the given time. Also contains safeguards such as
// refusing to delete default networks or resources with a "do-not-delete" label.
func AgePolicy(t time.Time) PolicyFunc {
	return func(resource any) bool {
		var labels map[string]string
		var desc, name string
		var created time.Time
		var err error
		switch r := resource.(type) {
		case *osconfigv1alphapb.OSPolicyAssignment:
			name = r.Name
			desc = r.Description
			created = time.Unix(r.GetRevisionCreateTime().GetSeconds(), int64(r.GetRevisionCreateTime().GetNanos()))
		case *osconfigpb.GuestPolicy:
			name = r.Name
			desc = r.Description
			created = time.Unix(r.GetCreateTime().GetSeconds(), int64(r.GetCreateTime().GetNanos()))
		case *compute.Network:
			name = r.Name
			desc = r.Description
			if r.Name == "default" || strings.Contains(r.Description, "delete") {
				return false
			}
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.MachineImage:
			name = r.Name
			desc = r.Description
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.Disk:
			name = r.Name
			desc = r.Description
			labels = r.Labels
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.Image:
			name = r.Name
			desc = r.Description
			labels = r.Labels
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.Snapshot:
			name = r.Name
			desc = r.Description
			labels = r.Labels
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.Instance:
			name = r.Name
			desc = r.Description
			if r.DeletionProtection {
				return false
			}
			labels = r.Labels
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.ForwardingRule:
			desc = r.Description
			labels = r.Labels
			name = r.Name
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.UrlMap:
			desc = r.Description
			name = r.Name
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.BackendService:
			desc = r.Description
			name = r.Name
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.TargetHttpProxy:
			desc = r.Description
			name = r.Name
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.HealthCheck:
			desc = r.Description
			name = r.Name
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		case *compute.NetworkEndpointGroup:
			desc = r.Description
			name = r.Name
			created, err = time.Parse(time.RFC3339, r.CreationTimestamp)
		default:
			return false
		}
		if err != nil {
			return false
		}
		if _, keep := labels[keepLabel]; keep {
			return false
		}
		return t.After(created) && !strings.Contains(desc, keepLabel) && !strings.Contains(name, keepLabel)
	}
}

// WorkflowPolicy takes a daisy workflow ID and returns a PolicyFunc which
// indicates to delete anything which appears to have been created by this
// workflow. Note that daisy does have its own resource deletion hooks, this is
// used in edge cases where workflow deletion hooks are unreliable. Also
// contains safeguards such as refusing to delete default networks or resources
// with a "do-not-delete" label.
func WorkflowPolicy(id string) PolicyFunc {
	return func(resource any) bool {
		var name, desc string
		var labels map[string]string
		switch r := resource.(type) {
		case *osconfigv1alphapb.OSPolicyAssignment:
			name = r.Name
			desc = r.Description
		case *osconfigpb.GuestPolicy:
			name = r.Name
			desc = r.Description
		case *compute.Network:
			if r.Name == "default" {
				return false
			}
			name = r.Name
			desc = r.Description
		case *compute.MachineImage:
			name = r.Name
			desc = r.Description
		case *compute.Disk:
			desc = r.Description
			labels = r.Labels
			name = r.Name
		case *compute.Image:
			desc = r.Description
			labels = r.Labels
			name = r.Name
		case *compute.Snapshot:
			desc = r.Description
			labels = r.Labels
			name = r.Name
		case *compute.Instance:
			desc = r.Description
			if r.DeletionProtection {
				return false
			}
			labels = r.Labels
			name = r.Name
		case *compute.ForwardingRule:
			desc = r.Description
			labels = r.Labels
			name = r.Name
		case *compute.UrlMap:
			desc = r.Description
			name = r.Name
		case *compute.BackendService:
			desc = r.Description
			name = r.Name
		case *compute.TargetHttpProxy:
			desc = r.Description
			name = r.Name
		case *compute.HealthCheck:
			desc = r.Description
			name = r.Name
		case *compute.NetworkEndpointGroup:
			desc = r.Description
			name = r.Name
		default:
			return false
		}
		if _, keep := labels[keepLabel]; keep {
			return false
		}
		return strings.HasSuffix(name, id) && !strings.Contains(desc, keepLabel)
	}
}

// CleanInstances deletes all instances indicated, returning a slice of deleted
// instance partial URLs and a slice of errors encountered. On dry run, returns
// what would have been deleted.
func CleanInstances(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	instances, err := clients.Daisy.AggregatedListInstances(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing instance in project %q: %v", project, err)}
	}

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, i := range instances {
		if !delete(i) {
			continue
		}

		zone := path.Base(i.Zone)
		name := path.Base(i.SelfLink)
		partial := fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteInstance(project, zone, name); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, partial)
		}()
	}
	wg.Wait()
	return deleted, errs
}

// CleanDisks deletes all disks indicated, returning a slice of deleted partial
// urls and a slice of encountered errors. On dry run, returns what would have
// been deleted.
func CleanDisks(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	disks, err := clients.Daisy.AggregatedListDisks(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing disks in project %q: %v", project, err)}
	}

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, d := range disks {
		if !delete(d) {
			continue
		}

		zone := path.Base(d.Zone)
		name := path.Base(d.SelfLink)
		partial := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteDisk(project, zone, name); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, partial)
		}()
	}
	wg.Wait()
	return deleted, errs
}

// CleanImages deletes all images indicated, returning a slice of deleted
// partial urls and a slice of encountered errors. On dry run, returns what
// would have been deleted.
func CleanImages(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	images, err := clients.Daisy.ListImages(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing images in project %q: %v", project, err)}
	}

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, d := range images {
		if !delete(d) {
			continue
		}

		name := path.Base(d.SelfLink)
		partial := fmt.Sprintf("projects/%s/global/images/%s", project, name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteImage(project, name); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, partial)
		}()
	}
	wg.Wait()
	return deleted, errs
}

// CleanMachineImages deletes all machine images indicated, returning a slice
// of deleted partial urls and a slice of encountered errors. On dry run,
// returns what would have been deleted.
func CleanMachineImages(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	images, err := clients.Daisy.ListMachineImages(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing machine images in project %q: %v", project, err)}
	}

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, d := range images {
		if !delete(d) {
			continue
		}

		name := path.Base(d.SelfLink)
		partial := fmt.Sprintf("projects/%s/global/machineImages/%s", project, name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteMachineImage(project, name); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, partial)
		}()
	}
	wg.Wait()
	return deleted, errs
}

// CleanSnapshots deletes all snapshots indicated, returning a slice of deleted
// partial urls and a slice of encountered errors. On dry run, returns what
// would have been deleted.
func CleanSnapshots(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	images, err := clients.Daisy.ListSnapshots(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing snapshots in project %q: %v", project, err)}
	}

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, d := range images {
		if !delete(d) {
			continue
		}

		name := path.Base(d.SelfLink)
		partial := fmt.Sprintf("projects/%s/global/snapshots/%s", project, name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteSnapshot(project, name); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, partial)
		}()
	}
	wg.Wait()
	return deleted, errs
}

// CleanLoadBalancerResources deletes load balancer backend services and associated URL Maps, forwarding rules, network endpoint groups, and HTTP Proxies indicated by the policy func.
func CleanLoadBalancerResources(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	globalForwardingRules, err := clients.Daisy.AggregatedListForwardingRules(project)
	if err != nil {
		return nil, []error{fmt.Errorf("could not list all forwarding rules: %v", err)}
	}

	regions, err := clients.Daisy.ListRegions(project)
	if err != nil {
		return nil, []error{fmt.Errorf("could not list regions: %v", err)}
	}

	regionalForwardingRules := make(map[string][]*compute.ForwardingRule)
	regionalBackendServices := make(map[string][]*compute.BackendService)
	regionalURLMaps := make(map[string][]*compute.UrlMap)
	regionalHTTPProxies := make(map[string][]*compute.TargetHttpProxy)

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	// Clean global forwarding rules.
	for _, fr := range globalForwardingRules {
		if !delete(fr) {
			continue
		}
		frpartial := fmt.Sprintf("projects/%s/regions/%s/forwardingRules/%s", project, fr.Region, fr.Name)
		wg.Add(1)
		go func(frName, frRegion string) {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteForwardingRule(project, frRegion, frName); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, frpartial)
		}(fr.Name, fr.Region)
	}
	wg.Wait()

	for _, computeRegion := range regions {
		region := path.Base(computeRegion.SelfLink)
		regionFRs, ok := regionalForwardingRules[region]
		if !ok {
			var err error
			regionFRs, err = clients.Daisy.ListForwardingRules(project, region)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			} else {
				regionalForwardingRules[region] = regionFRs
			}
		}
		// Clean regional forwarding rules in the region.
		for _, fr := range regionFRs {
			if !delete(fr) {
				continue
			}
			frpartial := fmt.Sprintf("projects/%s/regions/%s/forwardingRules/%s", project, region, fr.Name)
			wg.Add(1)
			go func(frName string) {
				defer wg.Done()
				if !dryRun {
					if err := clients.Daisy.DeleteForwardingRule(project, region, frName); err != nil {
						errsMu.Lock()
						defer errsMu.Unlock()
						errs = append(errs, err)
						return
					}
				}
				deletedMu.Lock()
				defer deletedMu.Unlock()
				deleted = append(deleted, frpartial)
			}(fr.Name)
		}

		// Clean backend services in the region.
		regionBSs, ok := regionalBackendServices[region]
		if !ok {
			var err error
			regionBSs, err = clients.Daisy.ListRegionBackendServices(project, region)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			} else {
				regionalBackendServices[region] = regionBSs
			}
		}
		for _, bs := range regionBSs {
			if !delete(bs) {
				continue
			}
			bspartial := fmt.Sprintf("projects/%s/regions/%s/backendServices/%s", project, region, bs.Name)
			regionUMs, ok := regionalURLMaps[region]
			if !ok {
				var err error
				regionUMs, err = clients.Daisy.ListRegionURLMaps(project, region)
				if err != nil {
					errsMu.Lock()
					errs = append(errs, err)
					errsMu.Unlock()
				} else {
					regionalURLMaps[region] = regionUMs
				}
			}
			// Clean URL maps in the region.
			for _, um := range regionUMs {
				if !delete(um) {
					continue
				}
				umpartial := fmt.Sprintf("projects/%s/regions/%s/urlMaps/%s", project, region, um.Name)
				regionHPs, ok := regionalHTTPProxies[region]
				if !ok {
					var err error
					regionHPs, err = clients.Daisy.ListRegionTargetHTTPProxies(project, region)
					if err != nil {
						errsMu.Lock()
						errs = append(errs, err)
						errsMu.Unlock()
					} else {
						regionalHTTPProxies[region] = regionHPs
					}
				}
				// Clean target http proxies in the region.
				for _, hp := range regionHPs {
					if !delete(hp) {
						continue
					}
					hppartial := fmt.Sprintf("projects/%s/regions/%s/targetHttpProxies/%s", project, region, hp.Name)
					wg.Add(1)
					go func(hpName string) {
						defer wg.Done()
						if !dryRun {
							if err := clients.Daisy.DeleteRegionTargetHTTPProxy(project, region, hpName); err != nil {
								errsMu.Lock()
								defer errsMu.Unlock()
								errs = append(errs, err)
								return
							}
						}
						deletedMu.Lock()
						defer deletedMu.Unlock()
						deleted = append(deleted, hppartial)
					}(path.Base(hp.Name))
				}
				// URL maps might be associated with target http proxies, wait for proxies to be
				// deleted first.
				wg.Wait()
				wg.Add(1)
				go func(unName string) {
					defer wg.Done()
					if !dryRun {
						if err := clients.Daisy.DeleteRegionURLMap(project, region, unName); err != nil {
							errsMu.Lock()
							defer errsMu.Unlock()
							errs = append(errs, err)
							return
						}
					}
					deletedMu.Lock()
					defer deletedMu.Unlock()
					deleted = append(deleted, umpartial)
				}(path.Base(um.Name))
			}
			// Backend services might be associated with URL maps, wait for them to be
			// deleted first.
			wg.Wait()
			wg.Add(1)
			go func(bsName string) {
				defer wg.Done()
				if !dryRun {
					if err := clients.Daisy.DeleteRegionBackendService(project, region, bsName); err != nil {
						errsMu.Lock()
						defer errsMu.Unlock()
						errs = append(errs, err)
						return
					}
				}
				deletedMu.Lock()
				defer deletedMu.Unlock()
				deleted = append(deleted, bspartial)
			}(bs.Name)
			wg.Wait()
			// Delete health checks, they might associated with backend services so wait for
			// those to be deleted first.
			for _, hc := range bs.HealthChecks {
				if !delete(hc) {
					continue
				}
				hcpartial := fmt.Sprintf("projects/%s/regions/%s/healthChecks/%s", project, region, path.Base(hc))
				wg.Add(1)
				go func(hcName string) {
					defer wg.Done()
					if !dryRun {
						if err := clients.Daisy.DeleteRegionHealthCheck(project, region, hcName); err != nil {
							errsMu.Lock()
							defer errsMu.Unlock()
							errs = append(errs, err)
							return
						}
					}
					deletedMu.Lock()
					defer deletedMu.Unlock()
					deleted = append(deleted, hcpartial)
				}(path.Base(hc))
			}
		}
		wg.Wait()

		// Delete network endpoint groups, they might associated with health checks so
		// wait for those to be deleted first.
		deletedNEGs, err := deleteNetworkEndpointGroups(clients, project, region, delete, nil, dryRun)
		if err != nil {
			errs = append(errs, err)
		}
		deleted = append(deleted, deletedNEGs...)
	}
	wg.Wait()
	return deleted, errs
}

// CleanNetworks deletes all networks indicated, as well as all subnetworks and
// firewall rules that are part of the network indicated for deleted. Returns a
// slice of deleted partial urls and a slice of encountered errors. On dry run,
// returns what would have been deleted.
func CleanNetworks(clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	networks, err := clients.Daisy.ListNetworks(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing networks in project %q: %v", project, err)}
	}

	firewalls, err := clients.Daisy.ListFirewallRules(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing firewalls in project %q: %v", project, err)}
	}

	subnetworks, err := clients.Daisy.AggregatedListSubnetworks(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing subnetworks in project %q: %v", project, err)}
	}

	routes, err := clients.Daisy.ListRoutes(project)
	if err != nil {
		return nil, []error{fmt.Errorf("error listing routes in project %q: %v", project, err)}
	}

	regionalForwardingRules := make(map[string][]*compute.ForwardingRule)
	regionalBackendServices := make(map[string][]*compute.BackendService)
	regionalURLMaps := make(map[string][]*compute.UrlMap)
	regionalHTTPProxies := make(map[string][]*compute.TargetHttpProxy)

	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	var networkWG sync.WaitGroup
	for _, n := range networks {
		networkWG.Go(func() {
			if !delete(n) {
				return
			}

			var wg sync.WaitGroup

			name := path.Base(n.SelfLink)
			netpartial := fmt.Sprintf("projects/%s/global/networks/%s", project, name)

			// Delete firewall rules associated with network.
			for _, f := range firewalls {
				if f.Network != n.SelfLink {
					continue
				}
				name := path.Base(f.SelfLink)
				fwallpartial := fmt.Sprintf("projects/%s/global/firewalls/%s", project, name)
				wg.Add(1)
				go func() {
					defer wg.Done()
					if !dryRun {
						if err := clients.Daisy.DeleteFirewallRule(project, name); err != nil {
							errsMu.Lock()
							defer errsMu.Unlock()
							errs = append(errs, err)
							return
						}
					}
					deletedMu.Lock()
					defer deletedMu.Unlock()
					deleted = append(deleted, fwallpartial)
				}()
			}

			for _, sn := range subnetworks {
				if sn.Network != n.SelfLink {
					continue
				}
				// If this network is setup with auto subnetworks we need to ignore any subnetworks that are in 10.128.0.0/9.
				// https://cloud.google.com/vpc/docs/vpc#ip-ranges
				if n.AutoCreateSubnetworks == true {
					i, err := strconv.Atoi(strings.Split(sn.IpCidrRange, ".")[1])
					if err != nil {
						fmt.Printf("Error parsing network range %q: %v\n", sn.IpCidrRange, err)
					}
					if i >= 128 {
						continue
					}
				}
				// Don't delete network yet - wait until resources associated with it are
				// deleted to avoid resource in use issues.
				region := path.Base(sn.Region)
				regionFRs, ok := regionalForwardingRules[region]
				if !ok {
					var err error
					regionFRs, err = clients.Daisy.ListForwardingRules(project, region)
					if err != nil {
						errsMu.Lock()
						errs = append(errs, err)
						errsMu.Unlock()
					} else {
						regionalForwardingRules[region] = regionFRs
					}
				}

				// Delete all forwarding rules in the same region as the subnetwork.
				for _, fr := range regionFRs {
					if fr.Network != n.SelfLink {
						continue
					}
					frpartial := fmt.Sprintf("projects/%s/regions/%s/forwardingRules/%s", project, region, fr.Name)
					wg.Add(1)
					go func(frName string) {
						defer wg.Done()
						if !dryRun {
							if err := clients.Daisy.DeleteForwardingRule(project, region, frName); err != nil {
								errsMu.Lock()
								defer errsMu.Unlock()
								errs = append(errs, err)
								return
							}
						}
						deletedMu.Lock()
						defer deletedMu.Unlock()
						deleted = append(deleted, frpartial)
					}(fr.Name)
				}

				regionBSs, ok := regionalBackendServices[region]
				if !ok {
					var err error
					regionBSs, err = clients.Daisy.ListRegionBackendServices(project, region)
					if err != nil {
						errsMu.Lock()
						errs = append(errs, err)
						errsMu.Unlock()
					} else {
						regionalBackendServices[region] = regionBSs
					}
				}
				// Delete all backend services in the same region as the subnetwork.
				for _, bs := range regionBSs {
					if bs.Network != n.SelfLink {
						continue
					}
					bspartial := fmt.Sprintf("projects/%s/regions/%s/backendServices/%s", project, region, bs.Name)
					regionUMs, ok := regionalURLMaps[region]
					if !ok {
						var err error
						regionUMs, err = clients.Daisy.ListRegionURLMaps(project, region)
						if err != nil {
							errsMu.Lock()
							errs = append(errs, err)
							errsMu.Unlock()
						} else {
							regionalURLMaps[region] = regionUMs
						}
					}
					// Delete all url maps in the same region as the subnetwork.
					for _, um := range regionUMs {
						if um.DefaultService != bs.SelfLink {
							continue
						}
						umpartial := fmt.Sprintf("projects/%s/regions/%s/urlMaps/%s", project, region, um.Name)
						regionHPs, ok := regionalHTTPProxies[region]
						if !ok {
							var err error
							regionHPs, err = clients.Daisy.ListRegionTargetHTTPProxies(project, region)
							if err != nil {
								errsMu.Lock()
								errs = append(errs, err)
								errsMu.Unlock()
							} else {
								regionalHTTPProxies[region] = regionHPs
							}
						}
						// Delete all target http proxies in the same region as the subnetwork.
						for _, hp := range regionHPs {
							if hp.UrlMap != um.SelfLink {
								continue
							}
							hppartial := fmt.Sprintf("projects/%s/regions/%s/targetHttpProxies/%s", project, region, hp.Name)
							wg.Add(1)
							go func(hpName string) {
								defer wg.Done()
								if !dryRun {
									if err := clients.Daisy.DeleteRegionTargetHTTPProxy(project, region, hpName); err != nil {
										errsMu.Lock()
										defer errsMu.Unlock()
										errs = append(errs, err)
										return
									}
								}
								deletedMu.Lock()
								defer deletedMu.Unlock()
								deleted = append(deleted, hppartial)
							}(path.Base(hp.Name))
						}
						wg.Wait()
						// Wait for target http proxy deletion before URL map deletion to avoid resource
						// in use issues.
						wg.Add(1)
						go func(unName string) {
							defer wg.Done()
							if !dryRun {
								if err := clients.Daisy.DeleteRegionURLMap(project, region, unName); err != nil {
									errsMu.Lock()
									defer errsMu.Unlock()
									errs = append(errs, err)
									return
								}
							}
							deletedMu.Lock()
							defer deletedMu.Unlock()
							deleted = append(deleted, umpartial)
						}(path.Base(um.Name))
					}
					// Wait for URL Map deletion before backend service deletion to avoid resource
					// in use issues.
					wg.Wait()
					wg.Add(1)
					go func(bsName string) {
						defer wg.Done()
						if !dryRun {
							if err := clients.Daisy.DeleteRegionBackendService(project, region, bsName); err != nil {
								errsMu.Lock()
								defer errsMu.Unlock()
								errs = append(errs, err)
								return
							}
						}
						deletedMu.Lock()
						defer deletedMu.Unlock()
						deleted = append(deleted, bspartial)
					}(bs.Name)
					wg.Wait()
					// Delete all health checks in the same region as the subnetwork.
					for _, hc := range bs.HealthChecks {
						hcpartial := fmt.Sprintf("projects/%s/regions/%s/healthChecks/%s", project, region, path.Base(hc))
						wg.Add(1)
						go func(hcName string) {
							defer wg.Done()
							if !dryRun {
								if err := clients.Daisy.DeleteRegionHealthCheck(project, region, hcName); err != nil {
									errsMu.Lock()
									defer errsMu.Unlock()
									errs = append(errs, err)
									return
								}
							}
							deletedMu.Lock()
							defer deletedMu.Unlock()
							deleted = append(deleted, hcpartial)
						}(path.Base(hc))
					}
				}
				wg.Wait()

				deletedNEGs, err := deleteNetworkEndpointGroups(clients, project, region, delete, n, dryRun)
				if err != nil {
					errs = append(errs, err)
				}
				deleted = append(deleted, deletedNEGs...)

				subnetpartial := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, sn.Name)
				wg.Add(1)
				go func(snName string) {
					defer wg.Done()
					if !dryRun {
						if err := clients.Daisy.DeleteSubnetwork(project, region, snName); err != nil {
							errsMu.Lock()
							defer errsMu.Unlock()
							errs = append(errs, err)
							return
						}
					}
					deletedMu.Lock()
					defer deletedMu.Unlock()
					deleted = append(deleted, subnetpartial)
				}(sn.Name)
			}
			// Delete all routes in the same network.
			for _, r := range routes {
				if r.Network != n.SelfLink {
					continue
				}
				rpartial := fmt.Sprintf("projects/%s/global/routes/%s", project, r.Name)
				wg.Add(1)
				go func(rName string) {
					defer wg.Done()
					if !dryRun {
						if err := clients.Daisy.DeleteRoute(project, rName); err != nil {
							errsMu.Lock()
							defer errsMu.Unlock()
							errs = append(errs, err)
							return
						}
					}
					deletedMu.Lock()
					defer deletedMu.Unlock()
					deleted = append(deleted, rpartial)
				}(r.Name)
			}
			// Wait for subnetwork and routes deletion before network deletion to avoid
			// resource in use issues.
			wg.Wait()

			if !dryRun {
				if err := clients.Daisy.DeleteNetwork(project, name); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, netpartial)
		})
	}
	networkWG.Wait()
	return deleted, errs
}

// deleteNetworkEndpointGroups deletes all network endpoint groups in the given network and region.
func deleteNetworkEndpointGroups(clients Clients, project, region string, delete PolicyFunc, network *compute.Network, dryRun bool) ([]string, error) {
	var errs []error
	var errsMu sync.Mutex
	var deleted []string
	var deletedMu sync.Mutex
	var wg sync.WaitGroup
	regionalNEGs, err := clients.Daisy.ListRegionNetworkEndpointGroups(project, region)
	if err != nil {
		errsMu.Lock()
		errs = append(errs, err)
		errsMu.Unlock()
	}
	for _, neg := range regionalNEGs {
		// Make sure the NEG is associated with the given network.
		if network != nil && neg.Network != network.SelfLink {
			continue
		}
		// Make sure the NEG should be deleted.
		if !delete(neg) {
			continue
		}
		negpartial := fmt.Sprintf("projects/%s/regions/%s/networkEndpointGroups/%s", project, region, neg.Name)
		wg.Add(1)
		go func(negName string) {
			defer wg.Done()
			if !dryRun {
				if err := clients.Daisy.DeleteRegionNetworkEndpointGroup(project, region, negName); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, err)
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, negpartial)
		}(neg.Name)
	}

	// Delete all zonal NEGs within the given region in the given network.
	zones, err := clients.Daisy.ListZones(project, daisyCompute.Filter(fmt.Sprintf("name eq %s-[a-z]", region)))
	if err != nil {
		errsMu.Lock()
		errs = append(errs, err)
		errsMu.Unlock()
	}
	for _, zone := range zones {
		zoneString := zone.Name
		zonalNEGs, err := clients.Daisy.ListNetworkEndpointGroups(project, zoneString)
		if err != nil {
			errsMu.Lock()
			errs = append(errs, err)
			errsMu.Unlock()
		}
		for _, neg := range zonalNEGs {
			// Make sure the NEG is associated with the given network.
			if network != nil && neg.Network != network.SelfLink {
				continue
			}
			// Make sure the NEG should be deleted.
			if !delete(neg) {
				continue
			}
			negpartial := fmt.Sprintf("projects/%s/zones/%s/networkEndpointGroups/%s", project, zoneString, neg.Name)
			wg.Add(1)
			go func(negName string) {
				defer wg.Done()
				if !dryRun {
					if err := clients.Daisy.DeleteNetworkEndpointGroup(project, zoneString, negName); err != nil {
						errsMu.Lock()
						errs = append(errs, err)
						errsMu.Unlock()
					}
				}
				deletedMu.Lock()
				defer deletedMu.Unlock()
				deleted = append(deleted, negpartial)
			}(neg.Name)
		}
	}
	wg.Wait()
	return deleted, errors.Join(errs...)
}

// CleanGuestPolicies deletes all guest policies indicated, returning a slice
// of deleted policy names and a slice of encountered errors. On dry run,
// returns what would have been deleted.
func CleanGuestPolicies(ctx context.Context, clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	gpolicies, err := getGuestPolicies(ctx, clients, project)
	if err != nil {
		return nil, []error{err}
	}
	return deleteGuestPolicies(ctx, clients, gpolicies, delete, dryRun)
}

func deleteGuestPolicies(ctx context.Context, clients Clients, gpolicies []*osconfigpb.GuestPolicy, delete PolicyFunc, dryRun bool) ([]string, []error) {
	var wg sync.WaitGroup
	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	for _, gp := range gpolicies {
		if !delete(gp) {
			continue
		}
		partial := fmt.Sprintf("%s", gp.GetName())
		wg.Add(1)
		go func(gp *osconfigpb.GuestPolicy) {
			defer wg.Done()
			if !dryRun {
				if err := clients.OSConfig.DeleteGuestPolicy(ctx, &osconfigpb.DeleteGuestPolicyRequest{Name: gp.GetName()}); err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, fmt.Errorf("Failed to delete Guest Policy %s: %v", gp.GetName(), err))
					return
				}
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, partial)
		}(gp)
	}
	wg.Wait()
	return deleted, errs
}

func getGuestPolicies(ctx context.Context, clients Clients, project string) (gpolicies []*osconfigpb.GuestPolicy, err error) {
	var gp *osconfigpb.GuestPolicy
	itr := clients.OSConfig.ListGuestPolicies(ctx, &osconfigpb.ListGuestPoliciesRequest{Parent: "projects/" + project})
	for {
		gp, err = itr.Next()
		if err != nil {
			if err == iterator.Done {
				err = nil
			} else {
				err = fmt.Errorf("Failed to list Guest Policies : %v", err)
			}
			return
		}
		gpolicies = append(gpolicies, gp)
	}
}

func getOSPolicies(ctx context.Context, clients Clients, project string) (ospolicies []*osconfigv1alphapb.OSPolicyAssignment, errs []error) {
	zones, err := clients.Daisy.ListZones(project)
	if err != nil {
		return nil, []error{fmt.Errorf("Failed to list zones : %v", err)}
	}
	var osp *osconfigv1alphapb.OSPolicyAssignment
	for _, zone := range zones {
		itr := clients.OSConfigZonal.ListOSPolicyAssignments(ctx, &osconfigv1alphapb.ListOSPolicyAssignmentsRequest{Parent: fmt.Sprintf("projects/%s/locations/%s", project, zone.Name)})
		for {
			osp, err = itr.Next()
			if err != nil {
				if err != iterator.Done {
					errs = append(errs, fmt.Errorf("Failed to list OSPolicy assignments for zone %s : %v", zone.Name, err))
				}
				break
			}
			ospolicies = append(ospolicies, osp)
		}
	}
	return
}

func deleteOSPolicies(ctx context.Context, clients Clients, ospolicies []*osconfigv1alphapb.OSPolicyAssignment, delete PolicyFunc, dryRun bool) ([]string, []error) {
	var wg sync.WaitGroup
	var deletedMu sync.Mutex
	var deleted []string
	var errsMu sync.Mutex
	var errs []error
	for _, osp := range ospolicies {
		if !delete(osp) {
			continue
		}
		wg.Add(1)
		go func(osp *osconfigv1alphapb.OSPolicyAssignment) {
			defer wg.Done()
			if !dryRun {
				op, err := clients.OSConfigZonal.DeleteOSPolicyAssignment(ctx, &osconfigv1alphapb.DeleteOSPolicyAssignmentRequest{Name: osp.GetName()})
				if err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					errs = append(errs, fmt.Errorf("Failed to delete OSPolicy assignment %s: %v", osp.GetName(), err))
					return
				}
				op.Wait(ctx)
			}
			deletedMu.Lock()
			defer deletedMu.Unlock()
			deleted = append(deleted, osp.Name)
		}(osp)
	}
	wg.Wait()
	return deleted, errs
}

// CleanOSPolicyAssignments deletes all OS policy assignments indicated,
// returning a slice of deleted policy assignment names and a slice of
// encountered errors. On dry run, returns what would have been deleted.
func CleanOSPolicyAssignments(ctx context.Context, clients Clients, project string, delete PolicyFunc, dryRun bool) ([]string, []error) {
	ospolicies, errs := getOSPolicies(ctx, clients, project)
	deleted, deleteerrs := deleteOSPolicies(ctx, clients, ospolicies, delete, dryRun)
	return deleted, append(errs, deleteerrs...)
}

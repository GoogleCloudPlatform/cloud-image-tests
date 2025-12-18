# Image test suites

## What is being tested

The tests are a combination of various types - end to end tests on certain
software components, image validations and feature validations, etc. The
collective whole represents the quality assurance bar for releasing a
[supported GCE Image](https://cloud.google.com/compute/docs/images/os-details),
and the test suites here must all pass before Google engineers will release a
new GCE image.

Tests are broken down by suite below:

## Test Suites

### Test suite: cvm

#### TestSEVEnabled/TestSEVSNPEnabled/TestTDXEnabled
Validate that an instance can boot with the specified confidential instance type and that the guest kernel supports the associated CPU feature.

#### TestLiveMigrate
Test that live migration works on each supported confidential instance type.

#### TestTDXAttestation/TestSEVSNPAttestation
Produces an attestation quote, verifies the quote's signatures and certificates, and validates the non-signature report fields against a user-provided policy.

#### TestCheckApicId
Tests for correct APIC ID for the VCPUs

#### TestCheckCpuidLeaf7
Tests for correct enabling of bits in CPUID leaf 7. The test checks the following features: ADX, RDSEED, SMAP, FPDP, FPCSDS, LA57. Also checks that the LA57 bit is not set on platforms that do not support it.

### Test suite: disk

#### TestDiskResize
Validate the filesystem is resized on reboot after a disk resize.

- <b>Background</b>: A convenience feature offered on supported GCE Images, if you resize the
underlying disk to be larger, then a set of scripts invoked during boot will
automatically resize the root partition and filesystem to take advantage of the
new space.

- <b>Test logic</b>: Launch a VM with the default disk size. Wait for it to boot up, then resize the
disk and reboot the VM via the API. Wait for the VM to boot again, and validate
the new size as reported by the operating system matches the expected size.

#### TestDiskReadWrite

Validate that we can write some data to a disk and read it back.

#### TestBlockDeviceNaming

Test that guest disks have `google-DEVICE_NAME` [disk symlinks](https://cloud.google.com/compute/docs/disks/disk-symlinks) created. Because the guest environment is only involved in the creation of these symlinks with nvme disks in a linux guest, this is the only case that is tested.

### Test suite: guestagent ###

Tests which verify functionality of the guest agent.

#### TestTelemetry

Test that the guest-agent schedules reporting telemetry when it is enabled, and does not schedule reporting telemetry when it is disabled.

#### TestSnapshotScripts

Test that application consistent snapshots are working on [Linux](https://cloud.google.com/compute/docs/disks/creating-linux-application-consistent-pd-snapshots) and [Windows](https://cloud.google.com/compute/docs/instances/windows/creating-windows-persistent-disk-snapshot).

On linux, add a script to the VM and test that it is executed when a guest flush snapshot is created. On windows, test that the VSS provider service provider was notified when a guest flush snapshot is created.

### Test suite: hostnamevalidation ###

Tests which verify that the metadata hostname is created and works with the DNS record.

#### TestHostname
Test that the system hostname is correctly set.

- <b>Background</b>: The hostname is one of many pieces of 'dynamic' configuration that supported
GCE Images will set for you. This is compared to the
'static' configuration which is present on the image to be tested. Dynamic
configuration allows a single GCE Image to be used on many VMs without
pre-modification.

- <b>Test logic</b>: Retrieve the intended FQDN from metadata (which is authoritative) and
compare the hostname part of it (first label) to the currently set hostname as
returned by the kernel.

#### TestFQDN
Test that the fully-qualified domain name is correctly set.

- <b>Background</b>: The FQDN is a complicated concept in Linux operating systems, and setting it in
an incorrect way can lead to unexpected behavior in some software.

- <b>Test logic</b>: Retrieve the intended FQDN from metadata and compare the full value to the
output of `/bin/hostname -f`. See `man 1 hostname` for more details.

#### TestCustomHostname
Test that custom domain names are correctly set.

- <b>Background</b>: The domain name for a VM matches the configured internal GCE DNS setting (https://cloud.google.com/compute/docs/internal-dns). By default, this will be the zonal or global DNS name. However, if you
specify a custom domain name at instance creation time, this will be used instead.

- <b>Test logic</b>: Launch a VM with a custom domain name. Validate the domain name as with TestFQDN.

#### TestHostKeysGeneratedOnce
Validate that SSH host keys are only generated once per instance.

- <b>Background</b>: The Google guest agent will generate new SSH hostkeys on the first boot of an
instance. This is a dynamic configuration to enable GCE Images to be used on
many instances, as multiple instances sharing host keys or having predictable
host keys is a security risk. However, the host keys should remain constant for
the lifetime of an instance, as changing them after the first generation may
prevent new SSH connections.

- <b>Test logic</b>: Launch a VM and confirm the guest agent generates unique host keys on startup.
Restart the guest agent and confirm the host keys are not changed.

#### TestHostsFile

On linux guests, test that `/etc/hosts` is populated with an appropriate entry to set the FQDN and an entry to add an alias to the metadata server.

### Test suite: hotattach

#### TestFileHotAttach
Validate that hot attach disks work: a file can be written to the disk, the disk can be detached and
reattached, and the file can still be read.

### Test suite: lssd

#### TestMount
Validate that mounting and un-mounting a local ssd works, and files written are not lost when unmounted.

### Test suite: imageboot

#### TestGuestBoot
Test that the VM can boot.

#### TestGuestReboot
Test that the VM can be rebooting using the GCE API.

- <b>Background</b>: Some categories of errors can produce an OS image that boots but cannot
successfully reboot. Documenting these errors is out of scope for this document,
but this test is a regression test against this category of error.

- <b>Test logic</b>: Launch a VM and create a 'marker file' on disk. Reboot the VM and validate the
marker file exists on the second boot.

#### TestGuestSecureBoot
Test that VM launched with
[secure boot](https://cloud.google.com/security/shielded-cloud/shielded-vm#secure-boot)
features works properly.

- <b>Background</b>: Secure Boot is a Linux system feature that is supported on certain GCE Images
and VM types. Documenting how Secure Boot works is out of scope for this
document.

- <b>Test logic</b>: Launch a VM with Secure Boot enabled via the shielded instance config. Validate
that Secure Boot is enabled by querying the appropriate EFI variable through the
sysfs/efivarfs interface.

#### TestGuestRebootOnHost

Test that the VM can reboot successfully from inside the guest, rather that the GCE API.

#### TestStartTime and TestBootTime

TestStartTime is informational only, logging the time to test execution from VM creation.
TestBootTime tests that the time from VM start to when guest-agent (and sshd on linux) is not more than the allowed maximum. See [image_boot_test.go](imageboot/image_boot_test.go) for allowed boot times, the default is 60 seconds.

### Test suite: lvmvalidation ###

A suite which tests RHEL images for logical volume manager install status and
layout. It should skip on all other images.

### TestLVMPackage

Test that checks the lvm2 package install status.
If the image is LVM, the lvm2 package should be installed.
If the image is not LVM, the lvm2 package should not be installed.

### TestLVMLayout

Test that checks the LVM layout.
It checks for the existence of the LVM PV, Volume Group, and specific LVs.

### Test suite: licensevalidation ###

A suite which tests that linux licensing and windows activation are working successfully.

#### TestWindowsActivationStatus

Test that Windows is licensed and activated.

#### TestLicenses

Generate a list of licenses expected to see on the image based on the image name and family name. Test that the licenses are correct and that they propagate to the root disk. See [licensevalidation/setup.go](licensevalidation/setup.go) for license generation rules.

### Test suite: livemigrate

#### TestLiveMigrate

Test that an image can live migrate without rebooting.

### Test suite: loadbalancer

#### TestL3Backend and TestL3Client

Test that an image can serve as the backend for an L3 (network passthrough) load balancer. The Client VM sets up the load balancer resource, and the backends respond.

#### TestL7Backend and TestL7Client

Test that an image can serve as the backend for an L7 (application) load balancer. The Client VM sets up the load balancer resource, and the backends respond.

### Test suite: mdsmtls

#### TestMTLSCredsExists

Test that mTLS credentials were created and can be used to communicate with the MDS.

#### TestMTLSJobScheduled

Test that the guest agent has scheduled certificate rotations for mTLS credentials.

### Test suite: mdsroutes

#### TestMDSRoutes

Test that the MDS is accessible over the primary NIC only.

### Test suite: metadata

#### TestShutdownScripts TestShutdownURLScripts TestStartupScripts TestSysprepSpecialize

Test that shutdown, shutdown url, and startup, and sysprep-specialize scripts run correctly.

#### TestDaemonScripts

Test that a startup script can spawn a child that will continue running after the parent exits.

#### TestTestShutdownScriptsFailed TestStartupScriptsFailed

Test that a VM launched with an invalid metadata script only fails to execute that script.

#### TestTokenFetch

Test that VM service account tokens can be retrieved from the MDS.

#### TestMetaDataResponseHeaders

Test that metadata response don't contain unexpected google internal headers.

#### TestGetMetadatUsingIP

Test that compute metadata can be retrieved from 169.254.169.254.

### Test suite: network

#### TestDefaultMTU
Validate the primary interface has correct MTU of 1460

- <b>Background:</b> The default MTU for a GCE VPC is 1460. Setting the correct MTU on the network
interface to match will prevent unnecessary packet fragmentation.

- <b>Test logic:</b> Identify the primary network interface using metadata, and confirm it has the
correct MTU using the golang 'net' package, which uses the netlink interface on
Linux (same as the `ip` command).

#### TestGVNIC

Test that the gVNIC driver is loaded.

#### TestDHCP

Test that interfaces are configured with DHCP.

#### TestStaticIP

Test that interfaces with a static IP assigned to them in GCE receive that IP over DHCP.

#### TestNTP

Test that a time synchronization package is installed and properly configured.

- <b>Background</b>: Linux operating systems require a time synchronization sofware to be running to
correct any drift in the system clock. Correct clock time is required for a wide
variety of applications, and virtual machines are particularly prone to clock
drift.

- <b>Test logic</b>: Validate that an appropriate time synchronization package is installed using the
system package manager, and read its configuration file to verify that it is
configured to check the Google-provided time server.

#### TestAliases TestAliasAfterReboot TestAliasAgentRestart

Test that IP aliases are added by the guest-agent, and do not disappear after reboot or agent restart.

#### TestSendPing TestWaitForPing

Test that two multinic VMs connected to each other after two networks can send packets to each other over both networks.

### Test suite: networkperf

Validate the network performance of an image reaches at least 85% of advertised
speeds.

This test suite adds a flag to the manager which can be used to filter the test cases it runs.

  -networkperf_test_filter string
  	regexp filter for networkperf test cases, only cases with a matching family name will be run (default ".*")

To see the list of test cases, check [networkperf/setup.go](networkperf/setup.go)

#### TestNetworkPerformance

- <b>Background</b>: Reaching advertised speeds is important, as failing to reach them means that
there are problems with the image or its drivers. The 85% number is chosen as
that is the baseline that the performance tests generally can match or exceed.
Reaching 100% of the advertised speeds is unrealistic in real scenarios.

- <b>Test logic</b>: Launch a server VM and client VM, then run an iperf test between the two to test
network speeds. This test launches up to 3 sets of servers and clients: default
network, jumbo frames network, and tier1 networking tier.

### Test suite: networkinterfacenaming

#### TestNetworkInterfaceNamingScheme

Tests that the network interface names follow an acceptable scheme. On Linux,
only older images should be using the `eth0` scheme. New images should use Predictable Network
Interface names. IDPF and MLX5 NICs should follow their own scheme.

On Windows, interfaces should be named Ethernet*

### Test suite: nicsetup
Validates guest agent's configs with various NIC configurations.

- <b>Background</b>: Users can have a variety of different NIC setups on their
  VMs. It's important to have coverage for as many of these cases as possible to
  ensure that the guest agent behaves correctly in all these configurations.

- <b>Test logic</b>: The test creates a VM for every configuration of NIC. The
  NIC configurations that the test creates are as follows:
  - Single NIC:
    - IPv4
    - IPv4 + IPv6
    - IPv6
  - Multi NIC:
    - IPv4 x IPv4
    - IPv4 x Dual
    - IPv4 x IPv6
    - Dual x IPv4
    - Dual x Dual
    - Dual x IPv6
    - IPv6 x IPv4
    - IPv6 x Dual
    - IPv6 x IPv6
  
  On each of these, the test will check whether or not the guest agent has
  written a configuration for each NIC. For the primary NIC, it will toggle the
  guest agent configuration for `enable_primary_nic` to make sure that the guest
  agent reacts correctly to the toggling of the flag. For each NIC, it will also
  try to connect to Google's public DNS servers using IPv4 and IPv6, and
  verifying whether those connections should have succeeded based on the NIC's
  stack type.



### Test suite: oslogin
Validate that the user can SSH using OSLogin, and that the guest agent can correctly provision a
VM to utilize OSLogin.

- <b>Background</b>: OSLogin is a utility that helps manage users' keys and access for SSH. It also provides
features such as the ability to authenticate users using 2FA, security keys, or certificates.

- <b>Test logic</b>: Launch a client VM and two server VMs. Each of the server VMs will perform a check to
make sure the guest agent responds correctly to OSLogin metadata changes, and the client VM will use
test users to SSH to each of the server VMs. The methods covered by this test are normal SSH and 2FA SSH.

Note that this test must be run in a specifically prepared project. See the [OSlogin test README](oslogin/README.md) for more information.

### Test suite: packagemanager

#### TestRepoAvailabilityDualStack TestRepoAvailabilityIPv6Only TestRepoAvailabilityIPv4Only

Test that repositories are reachable from an instance with a NIC configured for ipv4, ipv6, and dual stack.

### Test suite: packageupgrade

#### TestDriverUpgrade TestPackageUpgrade

Test that the image can upgrade packages to the versions in the google-compute-engine-testing repository. Not implemented on Linux.

### Test suite: packagevalidation

#### TestStandardPrograms
Validate that Google-provided programs are present.

- <b>Background</b>: Google-provided Linux OS images come with certain Google utilities such as
`gsutil` and `gcloud` preinstalled as a convenience.

- <b>Test logic</b>: Attempt to invoke the utilities, confirming they are present, found in the PATH,
and executable.

#### TestGuestPackages
Validate that the Google guest environment packages are installed

- <b>Background</b>: Google-provided Linux OS images come with the Google guest environment
preinstalled. The guest environment enables many GCE features to function.

- <b>Test logic</b>: Validate that the guest environment packages are installed using the system
package manager.

#### TestGooGetInstalled TestGooGetAvailable TestSigned TestRemoveInstall TestPackagesInstalled TestPackagesAvailable TestPackagesSigned

Test that googet is fully functional on the image, and can install, uninstall, verify signatures, and manage repositories.

#### TestNetworkDriverLoaded TestDriversInstalled TestDriversRemoved

Test only run on Windows. Test that driver packages are installed from googet, and can be removed.

#### TestAutoUpdateEnabled TestNetworkConnecton TestEmsEnabled TestTimeZoneUTC TestPowershellVersion TestStartExe TestDotNETVersion TestServicesState TestWindowsEdition TestWindowsCore TestServerGuiShell

Test that native windows packages are configured correctly. These are the test cases:

* Windows auto updates are enabled
* Internet connection outside of GCP (to google.com) is working.
* Emergency Management Service is enabled
* Time Zone is UTC
* Powershell version is at least 5.1
* Server-Gui-Shell is not installed on Windows Server Core images, and vice versa.
* Powershell processes can be started, retrieved, and stopped.
* DotNET version is at least 4.7
* GCEAGent service is enabled
* GoogleVssAgent and GoogleVssProvider services are enabled on non-windows client images.
* google_osconfig_agent service is enabled on non 32-bit images.
* Windows Datacenter editions have "-dc-" in the image name.
* Windows Core editions have "-core-" in the image name.

#### TestGCESysprep

Test the functionality of the [GCESysprep](https://cloud.google.com/compute/docs/instances/windows/creating-windows-os-image#prepare_server_image) script. This is only applicable to Windows. These are the test cases:

* Windows Event Log was cleared.
* C:\Windows\Temp was cleared.
* RDP and WinRM certificates were cleared.
* RDP and WinRM traffic is allowed in the firewall.
* Known disk configurations were cleared.
* GCEStartup task is disabled.
* The windows setup script SetupComplete.cmd was written.
* google_osconfig_agent was disabled

### Test suite: rhel
Validate the RHEL images are set up properly

#### TestVersionLock
Check that the version lock for EUS & SAP RHEL image is set correctly & that
the base RHEL image doesn't do version lock

#### TestRhuiPackage
Check that the RHUI Client Package is present for PAYG images but not in BYOS
images

### Test suite: security

#### TestKernelSecuritySettings
Validate sysctl tuneables have correct values

- <b>Background</b>: Linux has a wide variety of kernel tuneables exposed via the sysctl interface.
Supported GCE Images are built with some of these setting predefined for best
behavior in the GCE environment, for example
"net.ipv4.icmp\_echo\_ignore\_broadcasts", which configures the kernel not
respond to broadcast pings.

- <b>Test logic</b>: Read each sysctl option from the /proc/sys filesystem interface and confirm it
has the correct value.

#### TestAutomaticUpdates
Validate automatic security updates are enabled on supported distributions

- <b>Background</b>: Some Linux distributions provide a mechanism for automatic package updates that
are marked as security updates. We enable these updates in supported GCE Images.

- <b>Test logic</b>: Confirm the relevant automatic updates package is installed, and that the
relevant configuration options are set in the configuration files.

#### TestPasswordSecurity
Validate security settings for SSHD and system accounts

- <b>Background</b>: As part of the default configuration provided in supported GCE Images, certain
security validations are performed. These include ensuring that password based
logins and root logins via SSH are disabled, and that system accounts have the
correct password and shell settings.

- <b>Test logic</b>: Read the SSHD configuration file and confirm it has the 'PasswordAuthentication
no' and 'PermitRootLogin no' directives set. Read the /etc/passwd file and
confirm all users have disabled passwords, and that 'system account' users
(those with UID < 1000) have the correct shell set (typically set to 'nologin'
or 'false')

#### TestSockets

Test that there are no network listeners on unexpected ports.

### Test suite: shapevalidation

Test that a VM can boot and access the virtual hardware of the large machine shape in a VM family. This test suite adds a flag to the manager which can be used to filter the test cases it runs.

  -shapevalidation_test_filter string
  	regexp filter for shapevalidation test cases, only cases with a matching family name will be run (default ".*")

To see the list of test cases, check [shapevalidation/setup.go](shapevalidation/setup.go)

#### Test`$FAMILY`Mem

Test that the available system memory is at least the expected amount of memory for this VM shape.

#### Test`$FAMILY`Cpu

Test the the number of active processors is equal to the number of processors expected for this VM shape.

#### Test`$FAMILY`Numa

Test the the number of active numa nodes is equal to the number of processors expected for this VM shape.

### Test suite: sql

Tests for Windows SQL server settings and functionality are correct.

#### TestPowerPlan

Test that the power plan is set to high perfomance.

#### TestSqlVersion

Test that the Windows SQL version matches the image name.

#### TestRemoteConnectivity

Test that a client can authentice and remotely manipulate SQL server tables.

### Test suite: ssh

Tests [metadata ssh key](https://cloud.google.com/compute/docs/connect/add-ssh-keys#metadata) functionality. On windows, tests use the [Windows SSH beta](https://cloud.google.com/compute/docs/connect/windows-ssh).

#### TestMatchingKeysInGuestAttributes

Validate that host keys in guest attributes match those on disk.

#### TestHostKeysAreUnique

Validate that host keys from disk are unique between instances.

#### TestHostKeysNotOverrideAfterAgentRestart

Test that SSH host keys do not change after restarting the guest agent.

#### TestSSHInstanceKey TestEmptyTest

TestSSHInstanceKey tests that it can add a metadata ssh key to another instance and use it to connect. TestEmptyTest does nothing, and waits to be connected to.

### Test suite: storageperf

This test suite verifies PD performance on linux and windows. The following documentation is relevant for working with these tests, as of January 2024.

Performance limits: https://cloud.google.com/compute/docs/disks/performance. In addition to machine type and vCPU performance limits, most disks have a performance limit per VM, as well as a performance limit per GB. 

FIO command options: https://cloud.google.com/compute/docs/disks/benchmarking-pd-performance. To reach maximum IOPS and bandwidth MB per second, the disk needs to be warmed up with a "random write" fio task before running the benchmarking test.

Hyperdisk limits: https://cloud.google.com/compute/docs/disks/benchmark-hyperdisk-performance. Hyperdisk disk types have a much higher performance limit and limit per GB of disk size. To reach the highest performance values on linux, some additional fio options may be required.

This test suite adds a flag to the manager which can be used to filter the test cases it runs.

  -storageperf_test_filter string
  	regexp filter for storageperf test cases, only cases with a matching family name will be run (default ".*")

To see the list of test cases, check [storageperf/setup.go](storageperf/setup.go)

#### TestRandomReadIOPS and TestSequentialReadIOPS
Checks random and sequential read performance on files and compares it to an expected IOPS value
(in a future change, this will be compared to the documented IOPS value).

- <b>Background</b>: The public documentation for machine shapes and types lists certain values for
read IOPS. This test was designed to verify that the read IOPS which are attainable
are within a certain range (such as 97%) of the documented value.

- <b>Test logic</b>: FIO is downloaded based on the machine type and distribution. Next, the fio program
is run and the json output is returned. Out of the json output, we can get the read iops
value which was achieved, and check that it is above a certain threshold.

#### TestRandomWriteIOPS and TestSequentialWriteIOPS
Checks random and sequential file write performance on a disk and compares it to an expected IOPS value
(in a future change, this will be compared to a documented IOPS value).

- <b>Background</b>: Similar to the read iops tests, we want to verify that write IOPS on disks work at
the rate we expect for both random writes and throughput.

### Test suite: suspendresume

#### TestSuspend

Tests that an image can suspend and be resumed without rebooting.

### Test suite: vmspec

Tests network and metadata server connectivity after a change in VM specs. The
vmspec change is accomplished by detaching the boot disk from a source VM and
re-attaching it to a new VM using a machine type containing LSSDs. This test is
not supported on ARM-only images and is always run in zone `us-central1-a`.

#### TestMetadata

Test that the primary NIC can reach the metadata server after a vmspec change.

#### TestPCIEChanged

Test that the topology of the VM did change after a vmspec change.

#### TestPing

Test pinging external IPs from the primary NIC after a vmspec change.

### Test suite: windowscontainers

Test Windows containers functionality.

#### TestDockerIsInstalled TestDockerAvailable

Test that docker is installed and available as a powershell module.

#### TestBaseContainerImagesPresent TestBaseContainerImagesRun

Test that base container images from "mcr.microsoft.com/windows/servercore" are configured and runnable.

#### TestCanBuildNewContainerFromDockerfile

Test that container building is functional.

#### TestRunAndKillBackgroundContainer

Test that containers can be run in the background, commands can be exec'd in them, and they can be killed and cleaned up.

#### TestContainerCanMountStorageVolume

Test that containers can mount local storage volumes and read and write to them.

### Test suite: winrm

Tests for windows remote management.

#### TestWinrmConnection TestWaitForWinrmConnection

Test that a VM can remotely autheticate and run powershell commands on other once trusted as a winrm client.

### Test suite: acceleratorrdmanetwork

#### TestRDMANetworkClient TestRDMANetworkHost

Test that Accelerator images can establish and send traffic through RDMA
reliable connections across two nodes.

### Test suite: acceleratorrdmawriteimmediate

#### TestWriteWithImmediateClient TestWriteWithImmediateHost

Test that Accelerator images can exercise the RDMA verb Write With Immediate
across two nodes.

### Test suite: acceleratorrdmabandwidth

#### TestIBWriteBWClient TestIBWriteBWHost

Test Accelerator Images RDMA bandwidth performance to verify that the RDMA NIC
can approach its line rate.

### Test suite: acceleratornccl

#### TestNCCL

Test Accelerator Images can run NCCL by running [nccl-tests](https://github.com/NVIDIA/nccl-tests).

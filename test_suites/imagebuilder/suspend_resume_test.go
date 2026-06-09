package imagebuilder

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const marker = "/var/suspend-test-start"

func TestSuspend(t *testing.T) {

	if _, err := os.Stat(marker); err != nil && !os.IsNotExist(err) {
		t.Fatalf("could not determine if suspend testing has already started: %v", err)
	} else if err == nil {
		t.Fatal("unexpected reboot during suspend test")
	}
	err := os.WriteFile(marker, nil, 0777)
	if err != nil {
		t.Fatalf("could not mark beginning of suspend testing: %v", err)
	}
	ctx := utils.Context(t)
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("could not find project and zone: %v", err)
	}
	inst, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("could not get instance: %v", err)
	}

	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("could not make compute api client: %v", err)
	}

	err = client.Suspend(prj, zone, inst)
	if err != nil {
		// We can't really check the operation error here, we want to attempt to wait until its suspended but the wait operation will likely error out due to being interrupted by the suspension
		if !strings.Contains(err.Error(), "operation failed") && !strings.Contains(err.Error(), "failed to get zone operation") {
			t.Fatalf("could not suspend self: %v", err)
		}
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("could not confirm suspend testing has started ok: %v", err)
	}
	_, err = http.Get("https://cloud.google.com")
	if err != nil {
		t.Errorf("no network connectivity after resume: %v", err)
	}
	t.Log("Instance has network connectivity after resuming")
}

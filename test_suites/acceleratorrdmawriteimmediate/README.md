# Accelerator Image Test Suite: AcceleratorRDMAWriteImmediate

This is a test suite for Accelerator Images that exercises the RDMA verb Write
With Immediate.

## Reservations

GPU availability may be limited for Accelerator Images, specifically for A3
Ultra, A4, etc. This is an example quota error:

```
Code: QUOTA_EXCEEDED
Message: Quota 'GPUS_PER_GPU_FAMILY' exceeded.  Limit: 16.0 in region europe-west1
```

In any GCP project where there is available GPU quota, a reservation
should have automatically been created when the quota was granted. The
reservation should not need to be manually created.

If there is a reservation for GPU capacity, the following flags should be set:

*   "use_reservations" should be set to true
*   "reservation_urls" should be the name of the reservation(s)

Note that in the test suites, if the reservation is set to ANY_RESERVATION,
such as like this,

```
&compute.ReservationAffinity{ConsumeReservationType: "ANY_RESERVATION"}
```

it may not find the specific reservation you want. Instead, the reservation
needs to be set to "SPECIFIC_RESERVATION" with the reservation name.

```
&compute.ReservationAffinity{ConsumeReservationType: "SPECIFIC_RESERVATION",
Values: []string{"fake-reservation"},
Key: "compute.googleapis.com/reservation-name"},
```

The same applies to ReservationAffinityBeta.

## Periodics

Any changes to the test suite may also require changes in params here:
https://github.com/GoogleCloudPlatform/oss-test-infra/blob/master/prow/prowjobs/GoogleCloudPlatform/gcp-guest/gcp-guest-config.yaml

Note that when a new reservation is created, it can sometimes change the name of
the old reservation. This means that the reservation parameters in the periodic
tests may need to change as well.

This is a test suite for Accelerator Images.

GPU availability may be limited for Accelerator Images, specifically for A3
Ultra, A4, etc. This is an example quota error:

```
Code: QUOTA_EXCEEDED
Message: Quota 'GPUS_PER_GPU_FAMILY' exceeded.  Limit: 16.0 in region europe-west1
```

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

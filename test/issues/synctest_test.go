//go:build go1.25

// synctest requires the Go 1.23+ timer implementation, which go.mod's older go version otherwise disables for tests.
//go:debug asynctimerchan=0

package issues

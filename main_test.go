//go:build integration
// +build integration

// Package main – conformance integration test.
//
// This test runs the full cert-manager DNS01 conformance suite against a live
// Infoblox GRID.  It is gated behind the "integration" build tag so it is
// never executed in normal `go test ./...` runs and does not require a real
// Infoblox environment in CI.
//
// To run:
//
//	TEST_ZONE_NAME=example.com. \
//	  go test -v -tags integration -timeout 5m .
//
// The testdata/infoblox/config.json file must be populated with valid
// Infoblox connection details before running (see config.json.sample).
package main

import (
	"os"
	"testing"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

var testZone = os.Getenv("TEST_ZONE_NAME")

func TestRunsSuite(t *testing.T) {
	if testZone == "" {
		t.Skip("TEST_ZONE_NAME not set – skipping conformance test")
	}

	fixture := acmetest.NewFixture(
		&infobloxDNSSolver{},
		acmetest.SetResolvedZone(testZone),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetManifestPath("testdata/infoblox"),
	)

	fixture.RunConformance(t)
}

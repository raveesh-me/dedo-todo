package main

import (
	"testing"

	"github.com/GoHighLevel/dedo/policy"

	"github.com/raveesh-me/dedo-todo/app"
)

// TestPolicyTable fails before deploy when a service or method lacks its
// declared operational contract.
func TestPolicyTable(t *testing.T) {
	if _, err := policy.Build(app.Services...); err != nil {
		t.Fatal(err)
	}
}

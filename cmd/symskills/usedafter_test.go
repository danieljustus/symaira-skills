package main

import (
	"testing"
	"time"
)

func TestUsedAfter(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	if usedAfter(nil, &now) {
		t.Error("nil a must sort last")
	}
	if !usedAfter(&now, nil) {
		t.Error("non-nil a with nil b must win")
	}
	if !usedAfter(&later, &now) {
		t.Error("later a must win")
	}
	if usedAfter(&now, &later) {
		t.Error("earlier a must not win")
	}
	if usedAfter(&now, &now) {
		t.Error("equal times must not win")
	}
}

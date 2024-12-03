package main

import (
	"strings"
	"testing"
)

func TestPrefix(t *testing.T) {
	t.Log(strings.HasPrefix("/emoji", "/emoji"))
}

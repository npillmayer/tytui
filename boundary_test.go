package main

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestApplicationDependencyBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "github.com/npillmayer/typography/paragraph").CombinedOutput()
	if err != nil {
		t.Fatalf("dependency inspection: %v: %s", err, out)
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var pkg struct{ ImportPath string }
		if err = dec.Decode(&pkg); err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("dependency inspection: %v: %s", err, out)
		}
		for _, blocked := range []string{"tytui", "github.com/npillmayer/typography/adapter", "github.com/npillmayer/typography/discovery", "github.com/npillmayer/opentype", "github.com/npillmayer/fontfind", "github.com/npillmayer/w3c-dom"} {
			if pkg.ImportPath == blocked || strings.HasPrefix(pkg.ImportPath, blocked+"/") {
				t.Fatalf("layout core acquired dependency %s", pkg.ImportPath)
			}
		}
	}
	out, err = exec.CommandContext(ctx, "go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("dependency inspection: %v: %s", err, out)
	}
	if strings.Contains(string(out), "tytui/fontbackend") || strings.Contains(string(out), "tytui/pipeline/internal/layout") {
		t.Fatal("compatibility backend or local paragraph engine reintroduced")
	}
}

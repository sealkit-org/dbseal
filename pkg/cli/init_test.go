package cli

import (
	"context"
	"os"
	"testing"
)

func TestRunInit_Default(t *testing.T) {
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tempDir)

	err := Run(context.Background(), []string{"init"})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if _, err := os.Stat("dbseal.yaml"); os.IsNotExist(err) {
		t.Fatalf("expected dbseal.yaml to be created")
	}

	// Try running init again without force, should fail
	err = Run(context.Background(), []string{"init"})
	if err == nil {
		t.Fatalf("expected error when file already exists, got nil")
	}

	// Try running init with -f / --force, should succeed
	err = Run(context.Background(), []string{"init", "-f"})
	if err != nil {
		t.Fatalf("expected nil error with -f, got: %v", err)
	}
}

func TestRunInit_CustomNamePositional(t *testing.T) {
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tempDir)

	// Test with .yaml extension
	err := Run(context.Background(), []string{"init", "staging.yaml"})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if _, err := os.Stat("staging.yaml"); os.IsNotExist(err) {
		t.Fatalf("expected staging.yaml to be created")
	}

	// Test without .yaml extension (auto-completion)
	err = Run(context.Background(), []string{"init", "dev"})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if _, err := os.Stat("dev.yaml"); os.IsNotExist(err) {
		t.Fatalf("expected dev.yaml to be created")
	}

	// Test force overwrite on custom file
	err = Run(context.Background(), []string{"init", "dev", "--force"})
	if err != nil {
		t.Fatalf("expected nil error with --force, got: %v", err)
	}
}

func TestRunInit_Help(t *testing.T) {
	// Running with -h should return nil error
	err := Run(context.Background(), []string{"init", "-h"})
	if err != nil {
		t.Fatalf("expected nil error on help, got: %v", err)
	}
}

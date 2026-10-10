package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRootCmd_Version(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"version", "--json"})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing version command: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("expected valid JSON from version --json: %v (output: %s)", err, buf.String())
	}

	if res["cli_version"] != Version {
		t.Errorf("expected version %s, got %v", Version, res["cli_version"])
	}
}

func TestRootCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--help"})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error running help: %v", err)
	}

	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected help output, got empty string")
	}
}

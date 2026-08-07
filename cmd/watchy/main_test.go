package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseOptionsSeparatesGlobalFlags(t *testing.T) {
	opts, err := parseOptions([]string{"--online", "--model=llama3.2", "logs", "12", "-n", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.online || !opts.modelSet || opts.model != "llama3.2" {
		t.Fatalf("options = %+v", opts)
	}
	if opts.command != "logs" || strings.Join(opts.args, " ") != "12 -n 5" {
		t.Fatalf("command = %q, args = %v", opts.command, opts.args)
	}
}

func TestParseOptionsRejectsMissingModel(t *testing.T) {
	if _, err := parseOptions([]string{"--model"}); err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestParseOptionsStartAndRunAreEquivalent(t *testing.T) {
	var baseline options
	for i, spelling := range []string{"start", "run"} {
		opts, err := parseOptions([]string{spelling, "printf 'ok'", "--name", "demo"})
		if err != nil {
			t.Fatalf("parse %s: %v", spelling, err)
		}
		if i == 0 {
			baseline = opts
			continue
		}
		if opts.command != baseline.command || strings.Join(opts.args, "\x00") != strings.Join(baseline.args, "\x00") {
			t.Fatalf("%s options = %+v, want %+v", spelling, opts, baseline)
		}
	}
}

func TestRunStartAndRunShareBehavior(t *testing.T) {
	for _, spelling := range []string{"start", "run"} {
		t.Run(spelling, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			if err := run([]string{spelling, "true", "--name", "demo"}, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != "Started task 1: demo\n" {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestRunStartAndRunShareValidation(t *testing.T) {
	for _, spelling := range []string{"start", "run"} {
		t.Run(spelling, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			err := run([]string{spelling, "true", "--name"}, &stdout, &stderr)
			if err == nil || err.Error() != "--name requires a value" {
				t.Fatalf("error = %v, want --name requires a value", err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunStartAndRunShareHelp(t *testing.T) {
	outputs := make(map[string]string)
	for _, spelling := range []string{"start", "run"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{spelling, "--help"}, &stdout, &stderr); err != nil {
			t.Fatalf("%s --help: %v", spelling, err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("%s stderr = %q", spelling, stderr.String())
		}
		outputs[spelling] = stdout.String()
	}
	if outputs["run"] != outputs["start"] {
		t.Fatalf("run help differs from start help\nrun:\n%s\nstart:\n%s", outputs["run"], outputs["start"])
	}
}

func TestVersionDoesNotInitializeApplicationState(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != version {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

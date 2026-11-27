package main

import "testing"

func TestParseDemoServerFlags(t *testing.T) {
	// Default without --serve succeeds without --token.
	opts, err := parseDemoServerFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error without --serve: %v", err)
	}
	if opts.Serve || opts.Address != "127.0.0.1:4242" {
		t.Fatalf("unexpected default opts: %+v", opts)
	}

	// --serve without --token fails fast.
	if _, err := parseDemoServerFlags([]string{"--serve"}); err == nil {
		t.Fatalf("expected error when --serve is set without --token")
	}

	// --serve with explicit empty --token="" succeeds.
	opts, err = parseDemoServerFlags([]string{"--serve", "--token="})
	if err != nil {
		t.Fatalf("unexpected error with --token=: %v", err)
	}
	if !opts.Serve || opts.Token != "" {
		t.Fatalf("unexpected opts with empty token: %+v", opts)
	}

	// --serve with custom --address and --token succeeds.
	opts, err = parseDemoServerFlags([]string{"--serve", "--address=0.0.0.0:9000", "--token=abc123"})
	if err != nil {
		t.Fatalf("unexpected error with custom flags: %v", err)
	}
	if !opts.Serve || opts.Address != "0.0.0.0:9000" || opts.Token != "abc123" {
		t.Fatalf("unexpected opts: %+v", opts)
	}
}

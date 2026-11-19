package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// ocrOwnConfig is a config.json as `ocr config provider` writes one: a
// preset of the user's on a model of its own, and a setting beside it.
const ocrOwnConfig = `{
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "language": "zh",
  "providers": {
    "anthropic": {"api_key": "sk-ant-own", "model": "claude-sonnet-4-6"}
  }
}
`

// Picking a model of magpie's for Open Code Review writes the custom
// provider "magpie" (openai protocol, the gateway's /v1, a key naming
// it, the model) and points provider at it; going back puts the user's
// own provider and model back and takes magpie's entry out.
func TestOCRWiring(t *testing.T) {
	syncHome(t)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1", "m2"}}); err != nil {
		t.Fatal(err)
	}
	a, err := Find(ocrID)
	if err != nil {
		t.Fatal(err)
	}
	path := a.Path
	if filepath.Base(filepath.Dir(path)) != ".opencodereview" || filepath.Base(path) != "config.json" {
		t.Fatalf("path %s", path)
	}
	hanakoWrite(t, path, ocrOwnConfig, 0o600)
	if !a.Detected() {
		t.Fatal("~/.opencodereview doesn't count as Open Code Review there")
	}
	if v := a.Field("model").Get(); v != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("own model reads %q", v)
	}
	if a.Wired() {
		t.Fatal("the user's own anthropic reads as magpie's")
	}
	opts := a.Field("model").Options(a.Values())
	if !slices.ContainsFunc(opts, func(o Option) bool { return o.Value == "anthropic/claude-sonnet-4-6" }) ||
		!slices.ContainsFunc(opts, func(o Option) bool { return o.Value == "magpie/fake/m1" }) {
		t.Fatalf("options: %+v", opts)
	}

	if err := a.Apply("model", "magpie/fake/m1"); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"provider":                         "magpie",
		"custom_providers.magpie.protocol": "openai",
		"custom_providers.magpie.url":      gatewayV1(),
		"custom_providers.magpie.api_key":  gateway.TokenFor(ocrID),
		"custom_providers.magpie.model":    "fake/m1",
		"language":                         "zh",
		"providers.anthropic.api_key":      "sk-ant-own",
	} {
		if v, _ := edit.GetJSON(path, k); v != want {
			b, _ := os.ReadFile(path)
			t.Fatalf("%s = %q, want %q in:\n%s", k, v, want, b)
		}
	}
	if v := a.Field("model").Get(); v != "magpie/fake/m1" {
		t.Fatalf("model reads %q", v)
	}
	if !a.Wired() {
		t.Fatalf("not connected after a pick: %v", a.Values())
	}
	if d := a.Drift(); d != nil {
		t.Fatalf("drift right after a set: %+v", d)
	}
	// another of magpie's models: the stash keeps the user's, not magpie's
	if err := a.Apply("model", "magpie/fake/m2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := edit.GetJSON(path, "custom_providers.magpie.model"); v != "fake/m2" {
		t.Fatalf("model %q", v)
	}

	// the url changed by hand: magpie says so
	if err := edit.SetJSON(path, edit.KV{Path: "custom_providers.magpie.url", Value: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	if d := a.Drift(); d == nil {
		t.Fatal("a url that no longer reaches the gateway reads as fine")
	}
	if err := a.Apply("model", "magpie/fake/m2"); err != nil {
		t.Fatal(err)
	}

	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-6",
		"language": "zh",
	} {
		if v, _ := edit.GetJSON(path, k); v != want {
			b, _ := os.ReadFile(path)
			t.Fatalf("after Disconnect %s = %q, want %q in:\n%s", k, v, want, b)
		}
	}
	if _, ok := edit.GetJSONRaw(path, "custom_providers"); ok {
		b, _ := os.ReadFile(path)
		t.Fatalf("magpie's entry, or the custom_providers made for it, left behind:\n%s", b)
	}
	if a.Wired() {
		t.Fatal("still connected after Disconnect")
	}
}

// A model of the user's own picked while on magpie goes the way `ocr
// config model` writes it, and magpie's entry goes; a provider OCR hasn't
// set up is turned away rather than written as one it can't reach.
func TestOCROwnPick(t *testing.T) {
	syncHome(t)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	a, err := Find(ocrID)
	if err != nil {
		t.Fatal(err)
	}
	hanakoWrite(t, a.Path, ocrOwnConfig, 0o600)
	if err := a.Apply("model", "magpie/fake/m1"); err != nil {
		t.Fatal(err)
	}
	if err := a.Apply("model", "anthropic/claude-opus-4-6"); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"provider":                  "anthropic",
		"model":                     "claude-opus-4-6",
		"providers.anthropic.model": "claude-opus-4-6",
	} {
		if v, _ := edit.GetJSON(a.Path, k); v != want {
			t.Fatalf("%s = %q, want %q", k, v, want)
		}
	}
	if _, ok := edit.GetJSONRaw(a.Path, "custom_providers.magpie"); ok {
		t.Fatal("magpie's entry left behind")
	}
	if err := a.Apply("model", "nowhere/x"); err == nil {
		t.Fatal("a provider OCR hasn't set up was written")
	}
}

// `ocr` names other programs too: only one under a folder named for Open
// Code Review is its.
func TestOCRNotOurs(t *testing.T) {
	for bin, ours := range map[string]bool{
		"/usr/local/lib/node_modules/@alibaba-group/open-code-review/bin/ocr": true,
		"/home/u/.opencodereview/bin/ocr":                                     true,
		"/usr/bin/ocr":                                                        false,
		"/opt/homebrew/bin/ocr":                                               false,
	} {
		if got := !ocrNotOurs(bin); got != ours {
			t.Errorf("%s: ours %v, want %v", bin, got, ours)
		}
	}
}

// prime167 (#7043): on Windows `npm i -g @alibaba-group/open-code-review`
// writes ocr.cmd into npm's prefix rather than a link into the package, so
// nothing in the command's own path names Open Code Review and the agent
// read "CLI not found" while `ocr` ran. What the shim starts names it.
func TestOCRWindowsShim(t *testing.T) {
	dir := t.TempDir()
	// npm's shim (cmd-shim), as npm writes one for bin/ocr.js
	npmShim := "@ECHO off\r\n" +
		"SETLOCAL\r\n" +
		"SET \"_prog=node\"\r\n" +
		"\"%_prog%\"  \"%dp0%\\node_modules\\@alibaba-group\\open-code-review\\bin\\ocr.js\" %*\r\n"
	shim := filepath.Join(dir, "ocr.cmd")
	if err := os.WriteFile(shim, []byte(npmShim), 0o755); err != nil {
		t.Fatal(err)
	}
	if ocrNotOurs(shim) {
		t.Fatal("npm's shim of Open Code Review taken for another program's")
	}
	// another program's `ocr`: nothing of the package in what it starts
	other := filepath.Join(dir, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	theirs := filepath.Join(other, "ocr.cmd")
	if err := os.WriteFile(theirs, []byte("@ECHO off\r\ntesseract.exe %*\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !ocrNotOurs(theirs) {
		t.Fatal("another program's ocr taken for Open Code Review's")
	}
	// a program of the name is no launcher, however named: not read
	program := filepath.Join(other, "ocr.exe")
	if err := os.WriteFile(program, make([]byte, ocrShimMax+1), 0o755); err != nil {
		t.Fatal(err)
	}
	if !ocrNotOurs(program) {
		t.Fatal("a file too big to be a launcher was read as one")
	}
	if runtime.GOOS != "windows" {
		return // only Windows asks PATH for ocr.exe and ocr.cmd
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", dir)
	a, err := Find(ocrID)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Detected() {
		t.Fatal("ocr on PATH through npm's shim isn't Open Code Review there")
	}
}

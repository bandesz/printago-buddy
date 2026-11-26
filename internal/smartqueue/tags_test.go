package smartqueue_test

import (
	"errors"
	"maps"
	"testing"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestPark_emptyUserTags(t *testing.T) {
	in := map[string]string{
		"printer.id":             "printer-1",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
	}
	before := maps.Clone(in)

	got, err := smartqueue.Park(in)
	if err != nil {
		t.Fatalf("Park() error = %v", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("Park() mutated input: %#v", in)
	}
	want := map[string]string{
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
		"user.tags":              "&=buddy-hold",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Park() = %#v, want %#v", got, want)
	}

	blank := map[string]string{
		"user.tags":         "",
		"printer.modelName": "P1S",
	}
	gotBlank, err := smartqueue.Park(blank)
	if err != nil {
		t.Fatalf("Park(blank user.tags) error = %v", err)
	}
	wantBlank := map[string]string{
		"user.tags":         "&=buddy-hold",
		"printer.modelName": "P1S",
	}
	if !maps.Equal(gotBlank, wantBlank) {
		t.Errorf("Park(blank user.tags) = %#v, want %#v", gotBlank, wantBlank)
	}
}

func TestPark_andList(t *testing.T) {
	in := map[string]string{
		"user.tags":              "&=a,b",
		"printer.id":             "printer-1",
		"printer.nozzleDiameter": "0.4",
		"filament.type":          "PETG",
	}
	before := maps.Clone(in)

	got, err := smartqueue.Park(in)
	if err != nil {
		t.Fatalf("Park() error = %v", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("Park() mutated input: %#v", in)
	}
	want := map[string]string{
		"user.tags":              "&=a,b,buddy-hold",
		"printer.nozzleDiameter": "0.4",
		"filament.type":          "PETG",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Park() = %#v, want %#v", got, want)
	}
}

func TestPark_twice(t *testing.T) {
	in := map[string]string{
		"user.tags":         "&=a,b",
		"printer.modelName": "P1S",
	}

	once, err := smartqueue.Park(in)
	if err != nil {
		t.Fatalf("Park() error = %v", err)
	}
	twice, err := smartqueue.Park(once)
	if err != nil {
		t.Fatalf("Park() again error = %v", err)
	}
	want := map[string]string{
		"user.tags":         "&=a,b,buddy-hold",
		"printer.modelName": "P1S",
	}
	if !maps.Equal(once, want) {
		t.Fatalf("Park() once = %#v, want %#v", once, want)
	}
	if !maps.Equal(twice, want) {
		t.Errorf("Park() twice = %#v, want %#v", twice, want)
	}
}

func TestPin_removesParkFromAndList(t *testing.T) {
	in := map[string]string{
		"user.tags":              "&=a,buddy-hold",
		"printer.id":             "old",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
	}
	before := maps.Clone(in)

	got, err := smartqueue.Pin(in, "printer-9")
	if err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("Pin() mutated input: %#v", in)
	}
	want := map[string]string{
		"user.tags":              "&=a",
		"printer.id":             "printer-9",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Pin() = %#v, want %#v", got, want)
	}
}

func TestPin_removesParkOmitsUserTags(t *testing.T) {
	in := map[string]string{
		"user.tags":     "&=buddy-hold",
		"filament.type": "PLA",
	}
	before := maps.Clone(in)

	got, err := smartqueue.Pin(in, "printer-9")
	if err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("Pin() mutated input: %#v", in)
	}
	want := map[string]string{
		"printer.id":    "printer-9",
		"filament.type": "PLA",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Pin() = %#v, want %#v", got, want)
	}
	if _, ok := got["user.tags"]; ok {
		t.Errorf("Pin() kept user.tags = %q", got["user.tags"])
	}
}

func TestClearTags_leavesOtherKeys(t *testing.T) {
	in := map[string]string{
		"user.tags":              "&=a,buddy-hold",
		"printer.id":             "printer-9",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
	}
	before := maps.Clone(in)

	got, err := smartqueue.ClearTags(in)
	if err != nil {
		t.Fatalf("ClearTags() error = %v", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("ClearTags() mutated input: %#v", in)
	}
	want := map[string]string{
		"user.tags":              "&=a",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
	}
	if !maps.Equal(got, want) {
		t.Errorf("ClearTags() = %#v, want %#v", got, want)
	}
	if _, ok := got["printer.id"]; ok {
		t.Errorf("ClearTags() kept printer.id = %q", got["printer.id"])
	}
}

func TestClearTags_omitsUserTags(t *testing.T) {
	in := map[string]string{
		"user.tags":     "&=buddy-hold",
		"printer.id":    "printer-9",
		"filament.type": "PLA",
	}
	before := maps.Clone(in)

	got, err := smartqueue.ClearTags(in)
	if err != nil {
		t.Fatalf("ClearTags() error = %v", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("ClearTags() mutated input: %#v", in)
	}
	want := map[string]string{
		"filament.type": "PLA",
	}
	if !maps.Equal(got, want) {
		t.Errorf("ClearTags() = %#v, want %#v", got, want)
	}
	if _, ok := got["user.tags"]; ok {
		t.Errorf("ClearTags() kept user.tags = %q", got["user.tags"])
	}
}

func TestPark_invalidUserTags(t *testing.T) {
	assertInvalidUserTags(t, smartqueue.Park)
}

func TestPin_invalidUserTags(t *testing.T) {
	assertInvalidUserTags(t, func(tags map[string]string) (map[string]string, error) {
		return smartqueue.Pin(tags, "printer-9")
	})
}

func TestClearTags_invalidUserTags(t *testing.T) {
	assertInvalidUserTags(t, smartqueue.ClearTags)
}

func assertInvalidUserTags(t *testing.T, edit func(map[string]string) (map[string]string, error)) {
	t.Helper()
	in := map[string]string{
		"user.tags":              "plain",
		"printer.id":             "printer-9",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1 Carbon",
		"filament.type":          "PLA",
		"custom.note":            "keep",
	}
	before := maps.Clone(in)

	got, err := edit(in)
	if !errors.Is(err, smartqueue.ErrUserTags) {
		t.Fatalf("error = %v, want ErrUserTags", err)
	}
	if !maps.Equal(in, before) {
		t.Fatalf("mutated input: %#v", in)
	}
	if got != nil && !maps.Equal(got, before) {
		t.Errorf("rewrote tags: %#v", got)
	}
}

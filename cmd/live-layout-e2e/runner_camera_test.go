package main

import "testing"

func TestCanonicalCameraUpMatchesSketchUpPerspectiveBasis(t *testing.T) {
	got, ok := canonicalCameraUp(
		Vec3{X: 3500, Y: -2300, Z: 1850},
		Vec3{X: 900, Y: 217.5, Z: 450},
		Vec3{X: 0, Y: 0, Z: 1},
	)
	if !ok {
		t.Fatal("canonicalCameraUp returned !ok")
	}
	want := Vec3{X: -0.25919179561043354, Y: 0.25096744055741016, Z: 0.9326494287074336}
	if !vec3Close(got, want, 1e-12) {
		t.Fatalf("canonicalCameraUp = %#v, want %#v", got, want)
	}
}

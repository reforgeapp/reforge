package runnerclient

import (
	"strings"
	"testing"

	"reforge/internal/sandbox"
)

func TestValidateRuntimeConfigSelectsPinnedKubernetesImages(t *testing.T) {
	image := "sha256:" + strings.Repeat("a", 64)
	config := sandbox.RuntimeConfig{
		Backend: "kubernetes",
		Kubernetes: &sandbox.KubernetesRuntimeConfig{
			Namespace: "reforge", BrokerAdvertiseAddress: "127.0.0.1:8086",
			Images:     map[string]string{image: "ghcr.io/reforge/workspace-go@" + image},
			Toolchains: map[string]string{"go": image, "javascript": image, "python": image},
		},
		MemoryBytes: 512 << 20, DiskBytes: 128 << 20, CPUs: 2,
	}
	if err := ValidateRuntimeConfig(config); err != nil {
		t.Fatalf("valid Kubernetes config rejected: %v", err)
	}
	config.Images = map[string]string{image: "/images/go"}
	if err := ValidateRuntimeConfig(config); err == nil {
		t.Fatal("local rootfs image accepted for Kubernetes backend")
	}
	config.Images = nil
	config.Kubernetes.Images[image] = "ghcr.io/reforge/workspace-go@sha256:" + strings.Repeat("b", 64)
	if err := ValidateRuntimeConfig(config); err == nil {
		t.Fatal("image digest mismatch accepted")
	}
}

func TestValidateRuntimeConfigKeepsBackendSelectionExplicit(t *testing.T) {
	if err := ValidateRuntimeConfig(sandbox.RuntimeConfig{}); err != nil {
		t.Fatalf("empty backend should retain local default: %v", err)
	}
	if err := ValidateRuntimeConfig(sandbox.RuntimeConfig{Backend: "unknown"}); err == nil {
		t.Fatal("unknown backend accepted")
	}
	if err := ValidateRuntimeConfig(sandbox.RuntimeConfig{Backend: "local", Kubernetes: &sandbox.KubernetesRuntimeConfig{}}); err == nil {
		t.Fatal("Kubernetes config accepted with local backend")
	}
}

func TestBrokerAdvertiseAddressUsesPodIPAndConfiguredPort(t *testing.T) {
	t.Setenv("POD_IP", "10.12.0.9")
	address, err := brokerAdvertiseAddress(&sandbox.KubernetesRuntimeConfig{}, "0.0.0.0:8086")
	if err != nil || address != "10.12.0.9:8086" {
		t.Fatalf("advertise address=%q err=%v", address, err)
	}
	config := &sandbox.KubernetesRuntimeConfig{BrokerAdvertiseAddress: "10.12.0.9:9090"}
	if err = validateBrokerConfig(config); err == nil {
		t.Fatal("advertise port differing from listener accepted")
	}
}

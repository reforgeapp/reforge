package runnerclient

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"reforge/internal/egress"
	"reforge/internal/sandbox"
	"reforge/internal/sandbox/kubernetes"
)

func ValidateRuntimeConfig(cfg sandbox.RuntimeConfig) error {
	switch cfg.Backend {
	case "", "local":
		if cfg.Kubernetes != nil {
			return sandbox.ErrBoundary
		}
		return nil
	case "kubernetes":
		if cfg.Kubernetes == nil || len(cfg.Images) != 0 || cfg.DependencyRoot != "" {
			return sandbox.ErrBoundary
		}
		kube := cfg.Kubernetes
		allowed := map[string]bool{"go": true, "javascript": true, "python": true}
		if len(kube.Toolchains) != len(allowed) {
			return sandbox.ErrBoundary
		}
		for name, digest := range kube.Toolchains {
			if !allowed[name] {
				return sandbox.ErrBoundary
			}
			if _, ok := kube.Images[digest]; !ok {
				return sandbox.ErrBoundary
			}
		}
		if err := validateBrokerConfig(kube); err != nil {
			return err
		}
		return kubernetes.ValidateConfig(kubernetes.Config{
			Namespace: kube.Namespace, RuntimeClassName: kube.RuntimeClassName, Images: kube.Images,
			MemoryBytes: cfg.MemoryBytes, DiskBytes: cfg.DiskBytes, CPUs: cfg.CPUs, MaxProcesses: cfg.MaxProcesses,
		})
	default:
		return sandbox.ErrBoundary
	}
}

func NewSandboxRuntime(cfg sandbox.RuntimeConfig, fetch func(context.Context, sandbox.WorkspaceRequest) (sandbox.Snapshot, error)) (sandbox.SandboxRuntime, error) {
	if err := ValidateRuntimeConfig(cfg); err != nil {
		return nil, err
	}
	cfg.Fetch = fetch
	switch cfg.Backend {
	case "", "local":
		return sandbox.NewRuntime(cfg)
	case "kubernetes":
		kubeConfig := cfg.Kubernetes
		client, err := kubernetes.NewHTTPPodClient(kubernetes.APIConfig{Namespace: kubeConfig.Namespace})
		if err != nil {
			return nil, err
		}
		listen := kubeConfig.BrokerListenAddress
		if listen == "" {
			listen = "0.0.0.0:8086"
		}
		advertise, err := brokerAdvertiseAddress(kubeConfig, listen)
		if err != nil {
			return nil, err
		}
		broker, err := egress.ListenTCP(listen, egress.Registries)
		if err != nil {
			return nil, err
		}
		bootstrapper, err := kubernetes.NewRegistryBootstrapper(broker, advertise)
		if err != nil {
			_ = broker.Close()
			return nil, err
		}
		runtime, err := kubernetes.NewRuntime(kubernetes.Config{
			Namespace: kubeConfig.Namespace, RunnerID: kubeConfig.RunnerID, RuntimeClassName: kubeConfig.RuntimeClassName, Images: kubeConfig.Images,
			MemoryBytes: cfg.MemoryBytes, DiskBytes: cfg.DiskBytes, CPUs: cfg.CPUs, MaxProcesses: cfg.MaxProcesses,
			Fetch: fetch, Client: client, Bootstrapper: bootstrapper,
		})
		if err != nil {
			_ = broker.Close()
			return nil, err
		}
		return runtime, nil
	default:
		return nil, errors.New("sandbox backend invalid")
	}
}

func brokerAdvertiseAddress(config *sandbox.KubernetesRuntimeConfig, listen string) (string, error) {
	if config.BrokerAdvertiseAddress != "" {
		if !validTCPAddress(config.BrokerAdvertiseAddress) {
			return "", sandbox.ErrBoundary
		}
		return config.BrokerAdvertiseAddress, nil
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", sandbox.ErrBoundary
	}
	podIP := strings.TrimSpace(os.Getenv("POD_IP"))
	if podIP == "" {
		podIP = strings.TrimSpace(os.Getenv("KUBERNETES_POD_IP"))
	}
	if _, err := netip.ParseAddr(podIP); err == nil {
		return net.JoinHostPort(podIP, port), nil
	}
	return "", sandbox.ErrBoundary
}

func validTCPAddress(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	if _, err = netip.ParseAddr(host); err != nil {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func validateBrokerConfig(config *sandbox.KubernetesRuntimeConfig) error {
	listen := config.BrokerListenAddress
	if listen == "" {
		listen = "0.0.0.0:8086"
	}
	if !validTCPAddress(listen) {
		return sandbox.ErrBoundary
	}
	advertise := config.BrokerAdvertiseAddress
	if advertise == "" {
		podIP := strings.TrimSpace(os.Getenv("POD_IP"))
		if podIP == "" {
			podIP = strings.TrimSpace(os.Getenv("KUBERNETES_POD_IP"))
		}
		_, port, err := net.SplitHostPort(listen)
		if err != nil {
			return sandbox.ErrBoundary
		}
		if _, err = netip.ParseAddr(podIP); err != nil {
			return sandbox.ErrBoundary
		}
		advertise = net.JoinHostPort(podIP, port)
	}
	address, err := netip.ParseAddrPort(advertise)
	_, listenPort, listenErr := net.SplitHostPort(listen)
	port, portErr := strconv.ParseUint(listenPort, 10, 16)
	if err != nil || listenErr != nil || portErr != nil || !address.Addr().IsValid() || address.Addr().IsUnspecified() || address.Addr().Zone() != "" || address.Port() == 0 || uint64(address.Port()) != port {
		return sandbox.ErrBoundary
	}
	return nil
}

package github

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"reforge/internal/forge"
	"reforge/internal/network"
)

func (p *Provider) ReadCheckLog(ctx context.Context, reference forge.RepoRef, checkID string) (string, error) {
	segments, err := repositoryPath(reference)
	if err != nil {
		return "", err
	}
	if id, err := strconv.ParseInt(checkID, 10, 64); err != nil || id < 1 {
		return "", failure("invalid", "GitHub job identity required")
	}
	status, headers, body, err := p.request(ctx, http.MethodGet, append(segments, "actions", "jobs", checkID, "logs"), nil, nil)
	if err != nil {
		return "", err
	}
	switch status {
	case http.StatusOK:
		return forge.LogTail(body), nil
	case http.StatusFound, http.StatusMovedPermanently, http.StatusTemporaryRedirect:
		return downloadLog(ctx, headers.Get("Location"))
	}
	return "", responseError(status, headers)
}

func downloadLog(ctx context.Context, location string) (string, error) {
	target, err := url.Parse(location)
	if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil {
		return "", failure("protocol", "GitHub log location is invalid")
	}
	client, err := network.NewClient("https://"+target.Host, network.Options{})
	if err != nil {
		return "", failure("protocol", "GitHub log host is not allowed")
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return "", failure("protocol", "GitHub log location is invalid")
	}
	response, err := client.Do(req)
	if err != nil {
		return "", failure("transient", "GitHub log download failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", failure("transient", "GitHub log download failed")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return "", failure("transient", "GitHub log download failed")
	}
	return forge.LogTail(body), nil
}

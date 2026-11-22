package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

const (
	labelManaged     = "wiredeck2.managed"
	labelSiteID      = "wiredeck2.site_id"
	labelFingerprint = "wiredeck2.conf_fingerprint"
)

type DockerConfig struct {
	NetworkName string
	ConfDir     string
}

type DockerBackend struct {
	cli *client.Client
	cfg DockerConfig
}

func NewDockerBackend(cfg DockerConfig) (*DockerBackend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("init docker client: %w", err)
	}
	return &DockerBackend{cli: cli, cfg: cfg}, nil
}

func (d *DockerBackend) Close() error {
	return d.cli.Close()
}

func (d *DockerBackend) confFilePath(siteID string) string {
	return filepath.Join(d.cfg.ConfDir, siteID, "wg0.conf")
}

func (d *DockerBackend) writeConf(siteID string, conf []byte) (string, error) {
	dir := filepath.Join(d.cfg.ConfDir, siteID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir runner conf dir: %w", err)
	}
	p := filepath.Join(dir, "wg0.conf")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, conf, 0o600); err != nil {
		return "", fmt.Errorf("write runner conf tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename runner conf: %w", err)
	}
	return p, nil
}

func (d *DockerBackend) Ensure(ctx context.Context, spec RunnerSpec) (RunnerState, error) {
	confPath, err := d.writeConf(spec.SiteID, spec.Conf)
	if err != nil {
		return RunnerState{}, err
	}

	name := ContainerName(spec.SiteID)
	existing, err := d.Get(ctx, spec.SiteID)
	if err != nil {
		return RunnerState{}, err
	}

	needsRecreate := existing.ObservedState == "missing" ||
		existing.Image != spec.Image ||
		existing.ConfFingerprint != spec.ConfFingerprint

	if !needsRecreate {
		if existing.ObservedState != "running" {
			if err := d.cli.ContainerStart(ctx, existing.RuntimeID, container.StartOptions{}); err != nil {
				return RunnerState{}, fmt.Errorf("start existing runner: %w", err)
			}
		}
		return d.Get(ctx, spec.SiteID)
	}

	if existing.ObservedState != "missing" {
		timeoutSec := 10
		_ = d.cli.ContainerStop(ctx, existing.RuntimeID, container.StopOptions{Timeout: &timeoutSec})
		_ = d.cli.ContainerRemove(ctx, existing.RuntimeID, container.RemoveOptions{Force: true})
	}

	env := []string{
		"WIRERELAY_GRPC_PORT=9000",
		"WIRERELAY_HTTP_PORT=8080",
	}
	if spec.DisableIPv6 {
		env = append(env, "DISABLE_IPV6=true")
	}
	if spec.AuthToken != "" {
		env = append(env, "WIRERELAY_AUTH_TOKEN="+spec.AuthToken)
	}

	pidsLimit := int64(128)
	resp, err := d.cli.ContainerCreate(
		ctx,
		&container.Config{
			Image: spec.Image,
			Env:   env,
			Labels: map[string]string{
				labelManaged:     "true",
				labelSiteID:      spec.SiteID,
				labelFingerprint: spec.ConfFingerprint,
			},
		},
		&container.HostConfig{
			CapAdd:        []string{"NET_ADMIN"},
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
			LogConfig: container.LogConfig{
				Type: "json-file",
				Config: map[string]string{
					"max-size": "10m",
					"max-file": "3",
				},
			},
			Resources: container.Resources{
				Memory:    128 * 1024 * 1024, // 128 MiB cap
				PidsLimit: &pidsLimit,
			},
			Mounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   confPath,
					Target:   "/etc/wireguard/wg0.conf",
					ReadOnly: true,
				},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				d.cfg.NetworkName: {
					Aliases: []string{name},
				},
			},
		},
		nil,
		name,
	)
	if err != nil {
		return RunnerState{}, fmt.Errorf("create runner container: %w", err)
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return RunnerState{}, fmt.Errorf("start runner container: %w", err)
	}

	return d.Get(ctx, spec.SiteID)
}

func (d *DockerBackend) Stop(ctx context.Context, siteID string) error {
	name := ContainerName(siteID)
	timeoutSec := 10
	err := d.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeoutSec})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *DockerBackend) Remove(ctx context.Context, siteID string) error {
	name := ContainerName(siteID)
	err := d.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	_ = os.RemoveAll(filepath.Join(d.cfg.ConfDir, siteID))
	return nil
}

func (d *DockerBackend) Get(ctx context.Context, siteID string) (RunnerState, error) {
	name := ContainerName(siteID)
	_, statErr := os.Stat(d.confFilePath(siteID))
	confExists := statErr == nil

	insp, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return RunnerState{
				SiteID:        siteID,
				Name:          name,
				ObservedState: "missing",
				ConfExists:    confExists,
			}, nil
		}
		return RunnerState{}, err
	}
	obs := "unknown"
	if insp.State != nil {
		obs = insp.State.Status
	}
	return RunnerState{
		SiteID:          siteID,
		Name:            name,
		RuntimeID:       insp.ID,
		Image:           insp.Config.Image,
		ConfFingerprint: insp.Config.Labels[labelFingerprint],
		ObservedState:   obs,
		ConfExists:      confExists,
	}, nil
}

func (d *DockerBackend) List(ctx context.Context) ([]RunnerState, error) {
	f := filters.NewArgs(filters.Arg("label", labelManaged+"=true"))
	items, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, err
	}
	out := make([]RunnerState, 0, len(items))
	for _, c := range items {
		siteID := c.Labels[labelSiteID]
		_, statErr := os.Stat(d.confFilePath(siteID))
		out = append(out, RunnerState{
			SiteID:          siteID,
			Name:            ContainerName(siteID),
			RuntimeID:       c.ID,
			Image:           c.Image,
			ConfFingerprint: c.Labels[labelFingerprint],
			ObservedState:   c.State,
			ConfExists:      statErr == nil,
		})
	}
	return out, nil
}

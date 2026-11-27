package backend

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

const (
	labelManaged     = "wiredeck2.managed"
	labelSiteID      = "wiredeck2.site_id"
	labelFingerprint = "wiredeck2.conf_fingerprint"
	labelConfigMode  = "wiredeck2.config_mode"
)

type DockerConfig struct {
	NetworkName string
}

type DockerBackend struct {
	cli    *client.Client
	cfg    DockerConfig
	pullMu sync.Mutex
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

// EnsureImage checks whether imageRef exists in the local Docker daemon and pulls it
// from its registry if missing. Concurrent calls are serialized to avoid duplicate pulls.
func (d *DockerBackend) EnsureImage(ctx context.Context, imageRef string) error {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return fmt.Errorf("runner image reference is empty")
	}
	if _, err := d.cli.ImageInspect(ctx, ref); err == nil {
		return nil
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspect runner image %s: %w", ref, err)
	}

	d.pullMu.Lock()
	defer d.pullMu.Unlock()

	// Re-check after acquiring lock in case another goroutine just completed the pull.
	if _, err := d.cli.ImageInspect(ctx, ref); err == nil {
		return nil
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspect runner image %s: %w", ref, err)
	}

	slog.Info("pulling_runner_image", "image", ref)
	rc, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull runner image %s: %w", ref, err)
	}
	defer rc.Close()

	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("read pull stream for runner image %s: %w", ref, err)
	}
	slog.Info("runner_image_pulled", "image", ref)
	return nil
}

func (d *DockerBackend) Ensure(ctx context.Context, spec RunnerSpec) (RunnerState, error) {
	name := ContainerName(spec.SiteID)
	existing, err := d.Get(ctx, spec.SiteID)
	if err != nil {
		return RunnerState{}, err
	}

	needsRecreate := existing.ObservedState == "missing" ||
		existing.Image != spec.Image ||
		existing.ConfFingerprint != spec.ConfFingerprint ||
		existing.ConfigMode != ConfigModeGRPC

	if !needsRecreate {
		if existing.ObservedState != "running" {
			if err := d.cli.ContainerStart(ctx, existing.RuntimeID, container.StartOptions{}); err != nil {
				return RunnerState{}, fmt.Errorf("start existing runner: %w", err)
			}
		}
		return d.Get(ctx, spec.SiteID)
	}

	// Ensure the target image is present locally (pulling if absent) BEFORE tearing down
	// any existing running runner container so a failed pull never disrupts an active site.
	if err := d.EnsureImage(ctx, spec.Image); err != nil {
		return RunnerState{}, err
	}

	if existing.ObservedState != "missing" {
		timeoutSec := 10
		_ = d.cli.ContainerStop(ctx, existing.RuntimeID, container.StopOptions{Timeout: &timeoutSec})
		_ = d.cli.ContainerRemove(ctx, existing.RuntimeID, container.RemoveOptions{Force: true})
	}

	env := []string{
		"WIRERELAY_GRPC_PORT=9000",
		"WIRERELAY_HTTP_PORT=8080",
		"WIRERELAY_WAIT_FOR_CONFIG=true",
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
				labelConfigMode:  ConfigModeGRPC,
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
			// Container-private RAM tmpfs for WireRelay's ephemeral wg-quick temp file
			// (immediately shredded by WireRelay after wg-quick up; never touches host disk).
			Tmpfs: map[string]string{
				"/run/wireguard": "rw,noexec,nosuid,size=1m,mode=0700",
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
	return nil
}

func (d *DockerBackend) Get(ctx context.Context, siteID string) (RunnerState, error) {
	name := ContainerName(siteID)
	insp, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return RunnerState{
				SiteID:        siteID,
				Name:          name,
				ObservedState: "missing",
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
		ConfigMode:      insp.Config.Labels[labelConfigMode],
		ObservedState:   obs,
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
		out = append(out, RunnerState{
			SiteID:          siteID,
			Name:            ContainerName(siteID),
			RuntimeID:       c.ID,
			Image:           c.Image,
			ConfFingerprint: c.Labels[labelFingerprint],
			ConfigMode:      c.Labels[labelConfigMode],
			ObservedState:   c.State,
		})
	}
	return out, nil
}

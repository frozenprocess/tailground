//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	tsapi "tailscale.com/client/tailscale/v2"
)

type tsClient struct {
	api *tsapi.Client
}

// newChildClient builds a tsClient scoped to one child tailnet. The child
// oauthClient creds resolve their default ("-") tailnet to that child, so no
// tailnet id is needed here.
func newChildClient(cfg Config, child ChildOAuthCreds) (*tsClient, error) {
	base, err := url.Parse(strings.TrimRight(cfg.APIBaseURL, "/"))
	if err != nil {
		return nil, err
	}
	return &tsClient{
		api: &tsapi.Client{
			BaseURL: base,
			Auth: &tsapi.OAuth{
				ClientID:     child.ID,
				ClientSecret: child.Secret,
			},
		},
	}, nil
}

func (c *tsClient) SetACLRaw(ctx context.Context, hujson string) error {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()
	if err := c.api.PolicyFile().Set(ctx, hujson, ""); err != nil {
		return fmt.Errorf("push ACL: %w", err)
	}
	return nil
}

// GetACLRaw returns the tailnet's current policy as HuJSON.
func (c *tsClient) GetACLRaw(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()
	raw, err := c.api.PolicyFile().Raw(ctx)
	if err != nil {
		return "", fmt.Errorf("get ACL: %w", err)
	}
	return raw.HuJSON, nil
}

func (c *tsClient) ValidateACL(ctx context.Context, hujson string) error {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()
	return c.api.PolicyFile().Validate(ctx, hujson)
}

func (c *tsClient) MintAuthKey(ctx context.Context, tags []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()

	var req tsapi.CreateKeyRequest
	req.Description = "tg-e2e ephemeral node key"
	req.ExpirySeconds = int64(2 * time.Hour / time.Second)
	req.Capabilities.Devices.Create.Reusable = true
	req.Capabilities.Devices.Create.Ephemeral = true
	req.Capabilities.Devices.Create.Preauthorized = true
	req.Capabilities.Devices.Create.Tags = tags

	key, err := c.api.Keys().CreateAuthKey(ctx, req)
	if err != nil {
		return "", fmt.Errorf("mint auth key for %v: %w", tags, err)
	}
	if key.Key == "" {
		return "", fmt.Errorf("mint auth key for %v: empty key in response", tags)
	}
	return key.Key, nil
}

// Devices lists the tailnet's current devices.
func (c *tsClient) Devices(ctx context.Context) ([]tsapi.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()
	return c.api.Devices().List(ctx)
}

func (c *tsClient) ApproveRoutes(ctx context.Context, byHostname map[string][]string) error {
	if len(byHostname) == 0 {
		return nil
	}
	devs, err := c.Devices(ctx)
	if err != nil {
		return err
	}
	idByHost := map[string]string{}
	for _, d := range devs {
		idByHost[d.Hostname] = d.NodeID
	}
	for host, routes := range byHostname {
		id, ok := idByHost[host]
		if !ok {
			return fmt.Errorf("approve routes: no device with hostname %q", host)
		}
		rctx, cancel := context.WithTimeout(ctx, APITimeout)
		err := c.api.Devices().SetSubnetRoutes(rctx, id, routes)
		cancel()
		if err != nil {
			return fmt.Errorf("approve routes %v on %s: %w", routes, host, err)
		}
	}
	return nil
}

func (c *tsClient) SetPostureAttrs(ctx context.Context, byHostname map[string]map[string]any) error {
	if len(byHostname) == 0 {
		return nil
	}
	devs, err := c.Devices(ctx)
	if err != nil {
		return err
	}
	idByHost := map[string]string{}
	for _, d := range devs {
		idByHost[d.Hostname] = d.NodeID
	}
	for host, attrs := range byHostname {
		id, ok := idByHost[host]
		if !ok {
			return fmt.Errorf("set posture attrs: no device with hostname %q", host)
		}
		for key, val := range attrs {
			actx, cancel := context.WithTimeout(ctx, APITimeout)
			err := c.api.Devices().SetPostureAttribute(actx, id, key, tsapi.DevicePostureAttributeRequest{
				Value:  val,
				Expiry: tsapi.Time{Time: time.Now().Add(24 * time.Hour)},
			})
			cancel()
			if err != nil {
				return fmt.Errorf("set %s=%v on %s: %w", key, val, host, err)
			}
		}
	}
	return nil
}

// PurgeDevices deletes every device in the tailnet. A freshly created tailnet
// has none; a reused one (E2E_REUSE) carries stale ephemeral devices from
// earlier runs that would otherwise clutter the netmap. Returns the count
// removed.
func (c *tsClient) PurgeDevices(ctx context.Context) (int, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range devs {
		dctx, cancel := context.WithTimeout(ctx, APITimeout)
		err := c.api.Devices().Delete(dctx, d.NodeID)
		cancel()
		if err != nil {
			return n, fmt.Errorf("delete device %s (%s): %w", d.NodeID, d.Hostname, err)
		}
		n++
	}
	return n, nil
}

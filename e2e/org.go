//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OrgClient talks to the org-level API with the org OAuth client's credentials
// (Tailnets: Read + Write). Its only capabilities are creating, listing, and
// deleting child tailnets.
type OrgClient struct {
	baseURL string
	http    *http.Client
}

type ChildTailnet struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"displayName"`
	DNSName     string          `json:"dnsName"`
	CreatedAt   time.Time       `json:"createdAt"`
	OAuthClient ChildOAuthCreds `json:"oauthClient"`
}

// ChildOAuthCreds is the per-tailnet OAuth client returned on creation.
type ChildOAuthCreds struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// tailnetSummary is one entry from the list endpoint.
type tailnetSummary struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// NewOrgClient builds an OrgClient from the org OAuth client credentials. The
// token is fetched lazily and refreshed automatically.
func NewOrgClient(cfg Config) *OrgClient {
	cc := clientcredentials.Config{
		ClientID:     cfg.OAuthClientID,
		ClientSecret: cfg.OAuthClientSecret,
		TokenURL:     strings.TrimRight(cfg.APIBaseURL, "/") + "/api/v2/oauth/token",
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	return &OrgClient{
		baseURL: strings.TrimRight(cfg.APIBaseURL, "/"),
		http: &http.Client{
			Timeout:   APITimeout,
			Transport: &oauth2.Transport{Source: cc.TokenSource(context.Background())},
		},
	}
}

func (o *OrgClient) CreateTailnet(ctx context.Context, displayName string) (*ChildTailnet, error) {
	if err := guardName(displayName); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, TailnetCreateTimeout)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"displayName": displayName})
	var out ChildTailnet
	if err := o.do(ctx, http.MethodPost, "/api/v2/organizations/-/tailnets", body, &out); err != nil {
		return nil, fmt.Errorf("create tailnet %q: %w", displayName, err)
	}
	if out.ID == "" || out.OAuthClient.ID == "" || out.OAuthClient.Secret == "" {
		return nil, fmt.Errorf("create tailnet %q: response missing id or oauthClient: %+v", displayName, out)
	}
	return &out, nil
}

// ListTailnets returns every child tailnet in the org.
func (o *OrgClient) ListTailnets(ctx context.Context) ([]tailnetSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()

	// The alpha endpoint's envelope is undocumented; accept a bare array or a
	// {"tailnets": [...]} wrapper.
	var raw json.RawMessage
	if err := o.do(ctx, http.MethodGet, "/api/v2/organizations/-/tailnets", nil, &raw); err != nil {
		return nil, fmt.Errorf("list tailnets: %w", err)
	}
	var arr []tailnetSummary
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var wrapped struct {
		Tailnets []tailnetSummary `json:"tailnets"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("list tailnets: unexpected response shape: %s", truncate(string(raw), 200))
	}
	return wrapped.Tailnets, nil
}

func (o *OrgClient) DeleteTailnet(ctx context.Context, cfg Config, id string, child ChildOAuthCreds) error {
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()

	cc := clientcredentials.Config{
		ClientID:     child.ID,
		ClientSecret: child.Secret,
		TokenURL:     o.baseURL + "/api/v2/oauth/token",
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	hc := &http.Client{
		Timeout:   APITimeout,
		Transport: &oauth2.Transport{Source: cc.TokenSource(ctx)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, o.baseURL+"/api/v2/tailnet/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("delete tailnet %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete tailnet %s: %s: %s", id, resp.Status, truncate(string(b), 200))
	}
	return nil
}

func (o *OrgClient) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.baseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, truncate(string(b), 300))
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], b...)
		return nil
	}
	return json.Unmarshal(b, out)
}

func guardName(displayName string) error {
	if !strings.HasPrefix(displayName, TailnetPrefix) {
		return fmt.Errorf("refusing to operate on tailnet %q: missing %q prefix", displayName, TailnetPrefix)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

package injection

import (
	"context"
	"fmt"
)

// ProxyContributor provides proxy-related environment variables.
type ProxyContributor struct {
	ProxyURL    string
	SandboxName string
}

// NewProxyContributor creates a new proxy contributor.
func NewProxyContributor(proxyURL, sandboxName string) *ProxyContributor {
	return &ProxyContributor{
		ProxyURL:    proxyURL,
		SandboxName: sandboxName,
	}
}

// ContributeEnvVars returns proxy environment variables.
func (p *ProxyContributor) ContributeEnvVars(ctx context.Context, req *EnvVarRequest) ([]EnvVar, error) {
	proxyURL := p.ProxyURL
	sandboxName := p.SandboxName

	// Use request values if provided
	if req != nil {
		if req.ProxyURL != "" {
			proxyURL = req.ProxyURL
		}
		if req.SandboxName != "" {
			sandboxName = req.SandboxName
		}
	}

	if proxyURL == "" {
		return nil, nil
	}

	return []EnvVar{
		{
			Name:  "ANTHROPIC_BASE_URL",
			Value: fmt.Sprintf("%q", proxyURL),
		},
		{
			Name:  "ANTHROPIC_CUSTOM_HEADERS",
			Value: fmt.Sprintf(`"X-Forage-Sandbox: %s"`, sandboxName),
		},
	}, nil
}

// ContributePromptFragments returns proxy information for prompts.
func (p *ProxyContributor) ContributePromptFragments(ctx context.Context) ([]PromptFragment, error) {
	if p.ProxyURL == "" {
		return nil, nil
	}

	return []PromptFragment{{
		Section:  PromptSectionAgent,
		Priority: 50,
		Content:  proxyPromptInstructions,
	}}, nil
}

const proxyPromptInstructions = `This sandbox uses an API proxy for authentication. API keys are not stored in this container - they are injected by the proxy on the host.

How it works:
- ANTHROPIC_BASE_URL points to the host proxy
- Requests are forwarded with API key injection
- Rate limiting and audit logging are applied

Limitations:
- Only works with API key authentication
- For Max/Pro plans, use "claude login" directly (auth stays in sandbox)`

// Ensure ProxyContributor implements interfaces
var (
	_ EnvVarContributor = (*ProxyContributor)(nil)
	_ PromptContributor = (*ProxyContributor)(nil)
)

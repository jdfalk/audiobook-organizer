// file: internal/ai/register.go
// version: 1.0.0

// Service registry registrations for the AI cluster (W4).
//
// These services are all optional and conditional on config. Each Build
// returns (nil, nil) when its preconditions aren't met so the container
// can complete Build without error, and downstream consumers TryGet
// instead of Get.
//
// For consumers to be safe, they MUST nil-check the value returned
// from TryGet. Wiring in NewServer remains inline for now; W7 cleanup
// flips construction over to the container.

package ai

import (
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/serviceregistry"
)

func init() {
	// embedclient — OpenAI embedding client with optional cache.
	// Conditional on: OpenAIAPIKey set AND EmbeddingEnabled true.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "embedclient",
		Needs: []string{"config", "embeddingstore"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			if cfg.OpenAIAPIKey == "" || !cfg.EmbeddingEnabled {
				return (*EmbeddingClient)(nil), nil
			}
			embStore, _ := serviceregistry.TryGet[*database.EmbeddingStore](c, "embeddingstore")
			client := NewEmbeddingClient(cfg.OpenAIAPIKey)
			if embStore != nil {
				client = client.WithCache(embStore)
			}
			return client, nil
		},
	})

	// llmparser — OpenAIParser used by dedup Layer 3 review + metadata
	// LLM reranker. Conditional on OpenAIAPIKey set.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "llmparser",
		Needs: []string{"config"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			if cfg.OpenAIAPIKey == "" {
				return (*OpenAIParser)(nil), nil
			}
			return NewOpenAIParser(cfg, cfg.OpenAIAPIKey, cfg.EnableAIParsing), nil
		},
	})

	// metadatascorer — embedding-based metadata candidate scorer.
	// Conditional on embedclient + embeddingstore both being available.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "metadatascorer",
		Needs: []string{"config", "embedclient", "embeddingstore"},
		Build: func(c *serviceregistry.Container) (any, error) {
			cfg := serviceregistry.Get[*config.Config](c, "config")
			if !cfg.MetadataEmbeddingScoringEnabled {
				return (*EmbeddingScorer)(nil), nil
			}
			client, _ := serviceregistry.TryGet[*EmbeddingClient](c, "embedclient")
			store, _ := serviceregistry.TryGet[*database.EmbeddingStore](c, "embeddingstore")
			if client == nil || store == nil {
				return (*EmbeddingScorer)(nil), nil
			}
			return NewEmbeddingScorer(client, store), nil
		},
	})

	// metadatallmscorer — LLM-based metadata candidate rerank scorer.
	// Conditional on llmparser being available.
	serviceregistry.Register(serviceregistry.ServiceDef{
		Name:  "metadatallmscorer",
		Needs: []string{"llmparser"},
		Build: func(c *serviceregistry.Container) (any, error) {
			parser, _ := serviceregistry.TryGet[*OpenAIParser](c, "llmparser")
			if parser == nil {
				return (*LLMScorer)(nil), nil
			}
			return NewLLMScorer(parser), nil
		},
	})
}

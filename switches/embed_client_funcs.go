package switches

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/googleai/vertex"
)

func setupVertexEmbedClient(d *models.Datasource) (embeddings.EmbedderClient, error) {
	// format for project and location is split with a colon
	split := strings.Split(d.DBConnString, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("Connection string endpoint format (must be project:location)")
	}

	project := split[0]
	location := split[1]
	ctx, _ := context.WithTimeout(context.Background(), 240*time.Second)

	llm, err := vertex.New(
		ctx,
		googleai.WithCloudProject(project),
		googleai.WithCloudLocation(location),
		googleai.WithAPIKey(d.DBConnAPIKey),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create vertex driver: %v", err)
	}

	return llm, nil
}

func setupGoogleAIEmbedClient(d *models.Datasource) (embeddings.EmbedderClient, error) {
	var opts = make([]googleai.Option, 0)
	if d.EmbedAPIKey != "" {
		opts = append(opts, googleai.WithAPIKey(d.EmbedAPIKey))
	}

	opts = append(opts, googleai.WithDefaultEmbeddingModel(d.EmbedModel))

	llm, err := googleai.New(context.Background(), opts...)

	if err != nil {
		return nil, fmt.Errorf("failed to create google_ai driver: %v", err)
	}

	return llm, nil
}

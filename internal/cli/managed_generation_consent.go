package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	metadataclient "github.com/ra1phdd/zev/internal/entities/metadata/client"
	metadataservice "github.com/ra1phdd/zev/internal/entities/metadata/service"
	"github.com/spf13/cobra"
)

var errManagedGenerationPreview = errors.New("metadata generation preview")

type managedGenerationCapture struct {
	url  string
	body []byte
}

func (c *managedGenerationCapture) RoundTrip(req *http.Request) (*http.Response, error) {
	body, readErr := io.ReadAll(req.Body)
	closeErr := req.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("capture generation request body: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close generation request body: %w", closeErr)
	}
	c.url = req.URL.String()
	c.body = body
	return nil, errManagedGenerationPreview
}

func captureManagedGenerationRequest(ctx context.Context, cfg *config.Config, input any) (string, []byte, error) {
	capture := new(managedGenerationCapture)
	generator, err := metadataclient.NewConfigured(cfg, &http.Client{Transport: capture})
	if err != nil {
		return "", nil, fmt.Errorf("configure metadata preview: %w", err)
	}
	var generationErr error
	switch value := input.(type) {
	case metadataservice.DescriptionInput:
		_, generationErr = generator.GenerateDescription(ctx, value)
	case metadataservice.ArgumentsInput:
		_, generationErr = generator.GenerateArguments(ctx, value)
	default:
		return "", nil, metadataservice.ErrInvalidInput
	}
	if len(capture.body) == 0 {
		return "", nil, fmt.Errorf("prepare metadata preview: %w", generationErr)
	}
	return capture.url, capture.body, nil
}

func confirmManagedGeneration(cmd *cobra.Command, cfg *config.Config, request managedGeneration, description metadataservice.DescriptionInput, arguments metadataservice.ArgumentsInput) (bool, error) {
	var input any
	switch {
	case request.Description:
		input = description
	case request.Arguments:
		input = arguments
	default:
		return false, metadataservice.ErrInvalidInput
	}
	operationURL, body, err := captureManagedGenerationRequest(cmd.Context(), cfg, input)
	if err != nil {
		return false, fmt.Errorf("prepare generation disclosure: %w", err)
	}
	parsed, err := url.Parse(operationURL)
	if err != nil {
		return false, errors.New("invalid metadata destination")
	}
	location := locale.Text(locale.KeyManagedRemoteEndpoint)
	if parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback() {
		location = locale.Text(locale.KeyManagedLocalEndpoint)
	}
	if err := writeQueryOutput(cmd.ErrOrStderr(), locale.Text(locale.KeyManagedGenerationDisclosure), cfg.LLMProvider, operationURL, location, len(body), string(body)); err != nil {
		return false, err
	}
	form := request.Form
	if form == nil {
		form = newManagedForm(cmd)
	}
	answer, err := form.field(locale.KeyManagedGenerationConfirm, "")
	return answer == "yes", err
}

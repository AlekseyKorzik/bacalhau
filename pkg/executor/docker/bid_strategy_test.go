//go:build integration || !unit

package docker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bacalhau-project/bacalhau/pkg/bidstrategy"
	"github.com/bacalhau-project/bacalhau/pkg/docker"
	"github.com/bacalhau-project/bacalhau/pkg/model"
)

func jobForDockerImage(imageID string) model.Job {
	return model.Job{
		Spec: model.Spec{
			EngineSpec: model.EngineSpec{
				Type: model.DockerEngineType,
				Spec: map[string]interface{}{
					model.DockerEngineImageKey: imageID,
				},
			},
		},
	}
}

func TestBidsBasedOnImagePlatform(t *testing.T) {
	docker.MustHaveDocker(t)

	client, err := docker.NewDockerClient()
	require.NoError(t, err)

	strategy := NewBidStrategy(client)

	t.Run("positive response for supported architecture", func(t *testing.T) {
		response, err := strategy.ShouldBid(context.Background(), bidstrategy.BidStrategyRequest{
			Job: jobForDockerImage("ubuntu"),
		})

		require.NoError(t, err)
		require.Equal(t, true, response.ShouldBid)
	})

	t.Run("negative response for unsupported architecture", func(t *testing.T) {
		response, err := strategy.ShouldBid(context.Background(), bidstrategy.BidStrategyRequest{
			Job: jobForDockerImage("mcr.microsoft.com/windows:ltsc2019"),
		})

		require.NoError(t, err)
		require.Equal(t, false, response.ShouldBid)
	})
}

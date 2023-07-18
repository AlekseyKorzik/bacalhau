//go:build unit || !integration

package publicapi

import (
	"context"
	"testing"

	"github.com/bacalhau-project/bacalhau/pkg/logger"
	"github.com/bacalhau-project/bacalhau/pkg/model"
	testutils "github.com/bacalhau-project/bacalhau/pkg/test/utils"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	logger.ConfigureTestLogging(t)
	n, c := setupNodeForTest(t)
	defer n.CleanupManager.Cleanup(context.Background())

	ctx := context.Background()

	// Submit a few random jobs to the node:
	var err error
	var j *model.Job
	for i := 0; i < 5; i++ {
		genericJob := testutils.MakeGenericJob()
		j, err = c.Submit(ctx, genericJob)
		require.NoError(t, err)
	}

	// Should be able to look up one of them:
	job2, ok, err := c.Get(ctx, j.Metadata.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, job2.Job.Metadata.ID, j.Metadata.ID)
}

func TestGetPublicKey(t *testing.T) {
	ctx := context.Background()

	logger.ConfigureTestLogging(t)
	n, c := setupNodeForTest(t)
	defer n.CleanupManager.Cleanup(ctx)

	key, err := c.GetPublicKey(ctx)
	require.NoError(t, err)
	require.NotNil(t, key)
}

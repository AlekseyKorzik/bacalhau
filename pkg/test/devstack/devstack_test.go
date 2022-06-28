package devstack

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/filecoin-project/bacalhau/pkg/computenode"
	"github.com/filecoin-project/bacalhau/pkg/devstack"
	"github.com/filecoin-project/bacalhau/pkg/executor"
	ipfs_http "github.com/filecoin-project/bacalhau/pkg/ipfs/http"
	_ "github.com/filecoin-project/bacalhau/pkg/logger"
	"github.com/filecoin-project/bacalhau/pkg/publicapi"
	"github.com/filecoin-project/bacalhau/pkg/storage"
	"github.com/filecoin-project/bacalhau/pkg/system"
	"github.com/filecoin-project/bacalhau/pkg/test/scenario"
	"github.com/filecoin-project/bacalhau/pkg/verifier"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func newSpan(name string) (context.Context, trace.Span) {
	return system.Span(context.Background(), "devstack_test", name)
}

// re-use the docker executor tests but full end to end with libp2p transport
// and 3 nodes
func devStackDockerStorageTest(
	t *testing.T,
	testCase scenario.TestCase,
	nodeCount int,
) {
	ctx, span := newSpan(testCase.Name)
	defer span.End()

	stack, cm := SetupTest(
		t,
		nodeCount,
		0,
		computenode.NewDefaultJobSelectionPolicy(),
	)
	defer TeardownTest(stack, cm)

	nodeIDs, err := stack.GetNodeIds()
	assert.NoError(t, err)

	inputStorageList, err := testCase.SetupStorage(stack, storage.IPFSAPICopy, nodeCount)
	assert.NoError(t, err)

	jobSpec := &executor.JobSpec{
		Engine:   executor.EngineDocker,
		Verifier: verifier.VerifierIpfs,
		VM:       testCase.GetJobSpec(),
		Inputs:   inputStorageList,
		Outputs:  testCase.Outputs,
	}

	jobDeal := &executor.JobDeal{
		Concurrency: nodeCount,
	}

	apiUri := stack.Nodes[0].APIServer.GetURI()
	apiClient := publicapi.NewAPIClient(apiUri)
	submittedJob, err := apiClient.Submit(ctx, jobSpec, jobDeal)
	assert.NoError(t, err)

	// wait for the job to complete across all nodes
	err = stack.WaitForJob(ctx, submittedJob.ID,
		devstack.WaitForJobThrowErrors([]executor.JobStateType{
			executor.JobStateBidRejected,
			executor.JobStateError,
		}),
		devstack.WaitForJobAllHaveState(nodeIDs, executor.JobStateComplete),
	)

	assert.NoError(t, err)

	loadedJob, ok, err := apiClient.Get(ctx, submittedJob.ID)
	assert.True(t, ok)
	assert.NoError(t, err)

	// now we check the actual results produced by the ipfs verifier
	for nodeID, state := range loadedJob.State {
		node, err := stack.GetNode(ctx, nodeID)
		assert.NoError(t, err)

		outputDir, err := ioutil.TempDir("", "bacalhau-ipfs-devstack-test")
		assert.NoError(t, err)

		ipfsClient, err := ipfs_http.NewIPFSHTTPClient(
			node.IpfsNode.APIAddress())
		assert.NoError(t, err)

		ipfsClient.DownloadTar(ctx, outputDir, state.ResultsID)
		testCase.ResultsChecker(outputDir + "/" + state.ResultsID)
	}
}

func TestCatFileStdout(t *testing.T) {
	devStackDockerStorageTest(
		t,
		scenario.CatFileToStdout(t),
		3,
	)
}

func TestCatFileOutputVolume(t *testing.T) {
	devStackDockerStorageTest(
		t,
		scenario.CatFileToVolume(t),
		1,
	)
}

func TestGrepFile(t *testing.T) {
	devStackDockerStorageTest(
		t,
		scenario.GrepFile(t),
		3,
	)
}

func TestSedFile(t *testing.T) {
	devStackDockerStorageTest(
		t,
		scenario.SedFile(t),
		3,
	)
}

func TestAwkFile(t *testing.T) {
	devStackDockerStorageTest(
		t,
		scenario.AwkFile(t),
		3,
	)
}

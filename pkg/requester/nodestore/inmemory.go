package nodestore

import (
	"context"
	"time"

	"github.com/filecoin-project/bacalhau/pkg/model"
	"github.com/filecoin-project/bacalhau/pkg/requester"
	"github.com/libp2p/go-libp2p/core/peer"
	sync "github.com/lukemarsden/golang-mutex-tracer"
	"github.com/rs/zerolog/log"
)

// TODO: replace the manual and lazy eviction with a more efficient caching library
type nodeInfoWrapper struct {
	model.NodeInfo
	evictAt time.Time
}

type InMemoryNodeInfoStoreParams struct {
	TTL time.Duration
}

type InMemoryNodeInfoStore struct {
	ttl             time.Duration
	nodeInfoMap     map[peer.ID]nodeInfoWrapper
	engineNodeIDMap map[model.Engine]map[peer.ID]struct{}
	mu              sync.RWMutex
}

func NewInMemoryNodeInfoStore(params InMemoryNodeInfoStoreParams) *InMemoryNodeInfoStore {
	res := &InMemoryNodeInfoStore{
		ttl:             params.TTL,
		nodeInfoMap:     make(map[peer.ID]nodeInfoWrapper),
		engineNodeIDMap: make(map[model.Engine]map[peer.ID]struct{}),
	}
	res.mu.EnableTracerWithOpts(sync.Opts{
		Threshold: 10 * time.Millisecond,
		Id:        "InMemoryNodeInfoStore.mu",
	})
	return res
}

func (r *InMemoryNodeInfoStore) Add(ctx context.Context, nodeInfo model.NodeInfo) error {
	if !nodeInfo.IsComputeNode() {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// delete node from previous engines if it already exists to replace old engines with new ones if they've changed
	existingNodeInfo, ok := r.nodeInfoMap[nodeInfo.PeerInfo.ID]
	if ok {
		for _, engine := range existingNodeInfo.ComputeNodeInfo.ExecutionEngines {
			delete(r.engineNodeIDMap[engine], nodeInfo.PeerInfo.ID)
		}
	}

	// TODO: use data structure that maintains nodes in descending order based on available capacity.
	for _, engine := range nodeInfo.ComputeNodeInfo.ExecutionEngines {
		if _, ok := r.engineNodeIDMap[engine]; !ok {
			r.engineNodeIDMap[engine] = make(map[peer.ID]struct{})
		}
		r.engineNodeIDMap[engine][nodeInfo.PeerInfo.ID] = struct{}{}
	}

	// add or update the node info
	r.nodeInfoMap[nodeInfo.PeerInfo.ID] = nodeInfoWrapper{
		NodeInfo: nodeInfo,
		evictAt:  time.Now().Add(r.ttl),
	}

	log.Ctx(ctx).Trace().Msgf("Added node info %+v", nodeInfo)
	return nil
}

func (r *InMemoryNodeInfoStore) Get(ctx context.Context, peerID peer.ID) (model.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infoWrapper, ok := r.nodeInfoMap[peerID]
	if !ok {
		return model.NodeInfo{}, requester.NewErrNodeNotFound(peerID)
	}
	if time.Now().After(infoWrapper.evictAt) {
		go r.evict(ctx, infoWrapper)
		return model.NodeInfo{}, requester.NewErrNodeNotFound(peerID)
	}
	return infoWrapper.NodeInfo, nil
}

func (r *InMemoryNodeInfoStore) List(ctx context.Context) ([]model.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var nodeInfos []model.NodeInfo
	var toEvict []nodeInfoWrapper
	for _, nodeInfo := range r.nodeInfoMap {
		if time.Now().After(nodeInfo.evictAt) {
			toEvict = append(toEvict, nodeInfo)
		} else {
			nodeInfos = append(nodeInfos, nodeInfo.NodeInfo)
		}
	}
	if len(toEvict) > 0 {
		go r.evict(ctx, toEvict...)
	}
	return nodeInfos, nil
}

func (r *InMemoryNodeInfoStore) ListForEngine(ctx context.Context, engine model.Engine) ([]model.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var nodeInfos []model.NodeInfo
	var toEvict []nodeInfoWrapper
	for nodeID := range r.engineNodeIDMap[engine] {
		nodeInfo := r.nodeInfoMap[nodeID]
		if time.Now().After(nodeInfo.evictAt) {
			toEvict = append(toEvict, nodeInfo)
		} else {
			nodeInfos = append(nodeInfos, nodeInfo.NodeInfo)
		}
	}
	if len(toEvict) > 0 {
		go r.evict(ctx, toEvict...)
	}
	return nodeInfos, nil
}

func (r *InMemoryNodeInfoStore) Delete(ctx context.Context, peerID peer.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.doDelete(ctx, peerID)
}

func (r *InMemoryNodeInfoStore) evict(ctx context.Context, infoWrappers ...nodeInfoWrapper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, infoWrapper := range infoWrappers {
		nodeInfo, ok := r.nodeInfoMap[infoWrapper.PeerInfo.ID]
		if !ok || nodeInfo.evictAt != infoWrapper.evictAt {
			return // node info already evicted or has been updated since it was scheduled for eviction
		}
		err := r.doDelete(ctx, infoWrapper.PeerInfo.ID)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("Failed to evict expired node info for peer %s", infoWrapper.PeerInfo.ID)
		}
	}
}

func (r *InMemoryNodeInfoStore) doDelete(ctx context.Context, peerID peer.ID) error {
	nodeInfo, ok := r.nodeInfoMap[peerID]
	if !ok {
		return nil
	}
	for _, engine := range nodeInfo.ComputeNodeInfo.ExecutionEngines {
		delete(r.engineNodeIDMap[engine], peerID)
	}
	delete(r.nodeInfoMap, peerID)
	return nil
}

// compile time check that we implement the interface
var _ requester.NodeInfoStore = (*InMemoryNodeInfoStore)(nil)
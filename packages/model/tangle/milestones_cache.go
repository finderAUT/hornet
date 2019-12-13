package tangle

import (
	"github.com/gohornet/hornet/packages/datastructure"
	"github.com/gohornet/hornet/packages/model/milestone_index"
	"github.com/gohornet/hornet/packages/profile"
)

var (
	milestoneCache *datastructure.LRUCache
)

func InitMilestoneCache() {
	opts := profile.GetProfile().Caches.Milestones
	milestoneCache = datastructure.NewLRUCache(opts.Size, &datastructure.LRUCacheOptions{
		EvictionCallback:  onEvictMilestones,
		EvictionBatchSize: opts.EvictionSize,
	})
}

func onEvictMilestones(_ interface{}, values interface{}) {
	valT := values.([]interface{})

	var milestones []*Bundle
	for _, obj := range valT {
		milestones = append(milestones, obj.(*Bundle))
	}

	if err := StoreMilestonesInDatabase(milestones); err != nil {
		panic(err)
	}
}

func DiscardMilestoneFromCache(milestoneIndex milestone_index.MilestoneIndex) {
	milestoneCache.DeleteWithoutEviction(milestoneIndex)
}

func FlushMilestoneCache() {
	milestoneCache.DeleteAll()
}

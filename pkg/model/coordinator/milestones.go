package coordinator

import (
	"context"
	"time"

	"github.com/gohornet/hornet/pkg/model/hornet"
	"github.com/gohornet/hornet/pkg/model/milestone"
	"github.com/gohornet/hornet/pkg/model/storage"
	"github.com/iotaledger/hive.go/serializer"
	iotago "github.com/iotaledger/iota.go/v2"
)

// createCheckpoint creates a checkpoint message.
func (coo *Coordinator) createCheckpoint(parents hornet.MessageIDs) (*storage.Message, error) {
	iotaMsg := &iotago.Message{
		NetworkID: coo.networkID,
		Parents:   parents.ToSliceOfArrays(),
		Payload:   nil,
	}

	// we pass a background context here to not create invalid checkpoints at node shutdown.
	if err := coo.powHandler.DoPoW(context.Background(), iotaMsg, coo.opts.powWorkerCount); err != nil {
		return nil, err
	}

	msg, err := storage.NewMessage(iotaMsg, serializer.DeSeriModePerformValidation)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// createMilestone creates a signed milestone message.
func (coo *Coordinator) createMilestone(index milestone.Index, parents hornet.MessageIDs, receipt *iotago.Receipt, whiteFlagMerkleRootTreeHash [iotago.MilestoneInclusionMerkleProofLength]byte) (*storage.Message, error) {
	milestoneIndexSigner := coo.signerProvider.MilestoneIndexSigner(index)
	pubKeys := milestoneIndexSigner.PublicKeys()

	parentsSliceOfArray := parents.ToSliceOfArrays()
	msPayload, err := iotago.NewMilestone(uint32(index), uint64(time.Now().Unix()), parentsSliceOfArray, whiteFlagMerkleRootTreeHash, pubKeys)
	if err != nil {
		return nil, err
	}
	if receipt != nil {
		msPayload.Receipt = receipt
	}

	iotaMsg := &iotago.Message{
		NetworkID: coo.networkID,
		Parents:   parentsSliceOfArray,
		Payload:   msPayload,
	}

	if err := msPayload.Sign(coo.createSigningFuncWithRetries(milestoneIndexSigner.SigningFunc())); err != nil {
		return nil, err
	}

	if err = msPayload.VerifySignatures(coo.signerProvider.PublicKeysCount(), milestoneIndexSigner.PublicKeysSet()); err != nil {
		return nil, err
	}

	// we pass a background context here to not create invalid milestones at node shutdown.
	// otherwise the coordinator could panic at shutdown.
	if err := coo.powHandler.DoPoW(context.Background(), iotaMsg, coo.opts.powWorkerCount); err != nil {
		return nil, err
	}

	msg, err := storage.NewMessage(iotaMsg, serializer.DeSeriModePerformValidation)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// wraps the given MilestoneSigningFunc into a with retries enhanced version.
func (coo *Coordinator) createSigningFuncWithRetries(signingFunc iotago.MilestoneSigningFunc) iotago.MilestoneSigningFunc {
	return func(pubKeys []iotago.MilestonePublicKey, msEssence []byte) (sigs []iotago.MilestoneSignature, err error) {
		if coo.opts.signingRetryAmount <= 0 {
			return signingFunc(pubKeys, msEssence)
		}
		for i := 0; i < coo.opts.signingRetryAmount; i++ {
			sigs, err = signingFunc(pubKeys, msEssence)
			if err != nil {
				if i+1 != coo.opts.signingRetryAmount {
					coo.LogWarnf("signing attempt failed: %s, retrying in %v, retries left %d", err, coo.opts.signingRetryTimeout, coo.opts.signingRetryAmount-(i+1))
					time.Sleep(coo.opts.signingRetryTimeout)
				}
				continue
			}
			return sigs, nil
		}
		coo.LogWarnf("signing failed after %d attempts: %s ", coo.opts.signingRetryAmount, err)
		return
	}
}

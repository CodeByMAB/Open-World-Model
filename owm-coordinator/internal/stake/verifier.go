// Package stake implements the Proof-of-Stake enforcement layer: it verifies
// that nodes have opened a qualifying Lightning channel to the treasury before
// they are admitted to the network, monitors channel health, and executes
// force-close (slashing) on confirmed misbehaving nodes.
package stake

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/lightning"
)

// TierMinimums maps hardware tier to the minimum Lightning channel capacity
// required to join the network (in satoshis). Values match BRS-POS-02.
var TierMinimums = map[string]int64{
	"t1": 100_000,
	"t2": 500_000,
	"t3": 2_000_000,
}

// StakeResult is returned by VerifyStake.
type StakeResult struct {
	OK              bool
	ChannelID       string
	CapacitySats    int64
	LocalBalanceSats int64
	TierMinimumSats int64
	BonusMultiplier float64
	Error           string // non-empty if OK=false
}

// SlashConfig holds tunable slashing thresholds.
type SlashConfig struct {
	T1AutoSlashSignals int           // signals before automated slash for T1
	T2T3MaintainerAcks int           // maintainer acks required for T2/T3
	CooldownDuration   time.Duration // how long a slashed node must wait
}

// Verifier enforces PoS requirements and manages the slashing lifecycle.
type Verifier struct {
	db           *pgxpool.Pool
	lnReadonly   lightning.Client // ListChannels permission only
	lnSlash      lightning.Client // ForceCloseChan permission only
	slashCfg     SlashConfig
	log          *zap.Logger
}

// New creates a Verifier.
func New(db *pgxpool.Pool, lnReadonly, lnSlash lightning.Client, cfg SlashConfig, log *zap.Logger) *Verifier {
	return &Verifier{
		db:         db,
		lnReadonly: lnReadonly,
		lnSlash:    lnSlash,
		slashCfg:   cfg,
		log:        log,
	}
}

// VerifyStake checks whether the node identified by pubKeyHex has an open
// Lightning channel to the treasury that meets the minimum capacity for tier.
// This is called during node registration (SRS-STAKE-01, SRS-LN-11).
func (v *Verifier) VerifyStake(ctx context.Context, pubKeyHex, tier string) (*StakeResult, error) {
	tierMin, ok := TierMinimums[tier]
	if !ok {
		return nil, fmt.Errorf("unknown tier: %s", tier)
	}

	channels, err := v.lnReadonly.ListChannels(ctx, pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("querying LND channels: %w", err)
	}

	// Find the best qualifying channel (highest local balance toward treasury).
	var best *lightning.Channel
	for i := range channels {
		ch := &channels[i]
		if ch.LocalBalanceSats >= tierMin {
			if best == nil || ch.LocalBalanceSats > best.LocalBalanceSats {
				best = ch
			}
		}
	}

	if best == nil {
		return &StakeResult{
			OK:              false,
			TierMinimumSats: tierMin,
			Error:           fmt.Sprintf("INSUFFICIENT_STAKE: need %d sats, found none qualifying", tierMin),
		}, nil
	}

	bonus := computeBonus(best.LocalBalanceSats, tierMin)
	return &StakeResult{
		OK:               true,
		ChannelID:        best.ChannelID,
		CapacitySats:     best.CapacitySats,
		LocalBalanceSats: best.LocalBalanceSats,
		TierMinimumSats:  tierMin,
		BonusMultiplier:  bonus,
	}, nil
}

// PersistStake writes or updates a node_stakes record after successful verification.
func (v *Verifier) PersistStake(ctx context.Context, nodeID uuid.UUID, result *StakeResult) error {
	const q = `
		INSERT INTO node_stakes
		    (node_id, channel_id, channel_capacity, local_balance,
		     tier_minimum, bonus_multiplier, stake_status, last_verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', now())
		ON CONFLICT (node_id) DO UPDATE SET
		    channel_id       = EXCLUDED.channel_id,
		    channel_capacity = EXCLUDED.channel_capacity,
		    local_balance    = EXCLUDED.local_balance,
		    bonus_multiplier = EXCLUDED.bonus_multiplier,
		    stake_status     = 'active',
		    last_verified_at = now(),
		    degraded_since   = NULL`

	_, err := v.db.Exec(ctx, q,
		nodeID, result.ChannelID, result.CapacitySats,
		result.LocalBalanceSats, result.TierMinimumSats, result.BonusMultiplier,
	)
	return err
}

// VerifyAllActive re-checks stake channels for all active nodes.
// Called on a 6-hour timer (SRS-STAKE-02).
func (v *Verifier) VerifyAllActive(ctx context.Context) error {
	rows, err := v.db.Query(ctx,
		`SELECT n.node_id, n.public_key, n.tier, ns.channel_id, ns.tier_minimum
		 FROM nodes n JOIN node_stakes ns ON ns.node_id = n.node_id
		 WHERE n.status = 'active'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nodeID    uuid.UUID
			pubKey    string
			tier      string
			channelID string
			tierMin   int64
		)
		if err := rows.Scan(&nodeID, &pubKey, &tier, &channelID, &tierMin); err != nil {
			v.log.Error("scanning node for stake re-verification", zap.Error(err))
			continue
		}

		result, err := v.VerifyStake(ctx, pubKey, tier)
		if err != nil {
			v.log.Warn("stake re-verify error", zap.String("node_id", nodeID.String()), zap.Error(err))
			continue
		}

		if !result.OK {
			v.log.Warn("node stake degraded", zap.String("node_id", nodeID.String()))
			if err := v.markDegraded(ctx, nodeID); err != nil {
				v.log.Error("marking node degraded", zap.Error(err))
			}
		} else {
			// Refresh bonus multiplier and reset degraded state.
			if err := v.PersistStake(ctx, nodeID, result); err != nil {
				v.log.Error("persisting updated stake", zap.Error(err))
			}
		}
	}
	return rows.Err()
}

// RecordMisbehaviorSignal logs a misbehavior signal. If the signal count
// reaches the threshold for the node's tier, slashing is initiated.
// Implements SRS-STAKE-04 and the slashing flow from docs/architecture.md.
func (v *Verifier) RecordMisbehaviorSignal(ctx context.Context, nodeID uuid.UUID, tier, signalType, evidenceHash string) error {
	var signalCount int
	err := v.db.QueryRow(ctx,
		`INSERT INTO misbehavior_signals (node_id, signal_type, evidence_hash)
		 VALUES ($1, $2, $3)
		 RETURNING (SELECT count(*) FROM misbehavior_signals WHERE node_id = $1 AND slashing_event IS NULL)`,
		nodeID, signalType, evidenceHash,
	).Scan(&signalCount)
	if err != nil {
		return fmt.Errorf("recording signal: %w", err)
	}

	v.log.Info("misbehavior signal recorded",
		zap.String("node_id", nodeID.String()),
		zap.String("type", signalType),
		zap.Int("count", signalCount),
	)

	// T1: automated slash at threshold; T2/T3: requires external maintainer acks.
	if tier == "t1" && signalCount >= v.slashCfg.T1AutoSlashSignals {
		return v.Slash(ctx, nodeID, tier, signalType, evidenceHash, signalCount)
	}
	return nil
}

// Slash force-closes the node's stake channel and suspends the node.
// For T2/T3 nodes this must be called only after maintainer acknowledgment.
func (v *Verifier) Slash(ctx context.Context, nodeID uuid.UUID, tier, reason, evidenceHash string, signalCount int) error {
	// Fetch channel ID from stake record.
	var channelID string
	err := v.db.QueryRow(ctx,
		`SELECT channel_id FROM node_stakes WHERE node_id = $1`, nodeID,
	).Scan(&channelID)
	if err != nil {
		return fmt.Errorf("fetching channel for slashing: %w", err)
	}

	v.log.Warn("initiating slash",
		zap.String("node_id", nodeID.String()),
		zap.String("channel_id", channelID),
		zap.String("reason", reason),
	)

	// Force-close via the restricted slashing LND credential (SRS-SEC-13).
	if err := v.lnSlash.ForceCloseChan(ctx, channelID); err != nil {
		return fmt.Errorf("force-closing channel %s: %w", channelID, err)
	}

	cooldownExpires := time.Now().UTC().Add(v.slashCfg.CooldownDuration)

	// Persist slashing event.
	var eventID uuid.UUID
	if err := v.db.QueryRow(ctx,
		`INSERT INTO slashing_events
		    (node_id, channel_id, tier, reason, evidence_hash, signal_count, cooldown_expires_at, coordinator_sig)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		 RETURNING event_id`,
		nodeID, channelID, tier, reason, evidenceHash, signalCount, cooldownExpires,
	).Scan(&eventID); err != nil {
		return fmt.Errorf("persisting slash event: %w", err)
	}

	// Suspend node and mark stake force-closed.
	if _, err := v.db.Exec(ctx,
		`UPDATE nodes SET status = 'suspended' WHERE node_id = $1`, nodeID,
	); err != nil {
		return err
	}
	if _, err := v.db.Exec(ctx,
		`UPDATE node_stakes SET stake_status = 'force_closed' WHERE node_id = $1`, nodeID,
	); err != nil {
		return err
	}

	// Link all unresolved signals to this slashing event.
	_, err = v.db.Exec(ctx,
		`UPDATE misbehavior_signals SET slashing_event = $1
		 WHERE node_id = $2 AND slashing_event IS NULL`,
		eventID, nodeID,
	)

	v.log.Info("node slashed",
		zap.String("node_id", nodeID.String()),
		zap.String("event_id", eventID.String()),
		zap.Time("cooldown_expires", cooldownExpires),
	)
	return err
}

// markDegraded sets a node to degraded status if not already.
// The grace period enforcement (24h → suspended) is handled by a separate job.
func (v *Verifier) markDegraded(ctx context.Context, nodeID uuid.UUID) error {
	_, err := v.db.Exec(ctx,
		`UPDATE nodes SET status = 'degraded' WHERE node_id = $1 AND status = 'active'`,
		nodeID,
	)
	if err != nil {
		return err
	}
	_, err = v.db.Exec(ctx,
		`UPDATE node_stakes SET stake_status = 'degraded', degraded_since = now()
		 WHERE node_id = $1 AND degraded_since IS NULL`,
		nodeID,
	)
	return err
}

// computeBonus implements the stake bonus formula from SRS-4.4.2:
// clamp(1.0 + (stake - min) / min * 0.5, 1.0, 2.0)
func computeBonus(stakeSats, minSats int64) float64 {
	if stakeSats <= minSats {
		return 1.0
	}
	bonus := 1.0 + float64(stakeSats-minSats)/float64(minSats)*0.5
	if bonus > 2.0 {
		return 2.0
	}
	return bonus
}

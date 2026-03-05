// Package rpc implements the CoordinatorService gRPC server, bridging
// the protobuf API to the internal registry, scheduler, stake, and FL packages.
package rpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coordinatorv1 "github.com/owmnetwork/owm-coordinator/proto/coordinator/v1"

	"github.com/owmnetwork/owm-coordinator/internal/fl"
	"github.com/owmnetwork/owm-coordinator/internal/registry"
	"github.com/owmnetwork/owm-coordinator/internal/scheduler"
	stakeVerifier "github.com/owmnetwork/owm-coordinator/internal/stake"
)

const (
	coordinatorVersion = "0.1.0"
	// antiReplayWindow is the maximum age of a signed request timestamp.
	antiReplayWindow = 5 * time.Minute
)

// Server implements coordinatorv1.CoordinatorServiceServer.
type Server struct {
	coordinatorv1.UnimplementedCoordinatorServiceServer

	registry  *registry.Registry
	scheduler *scheduler.Scheduler
	verifier  *stakeVerifier.Verifier
	fl        *fl.Orchestrator
	db        *pgxpool.Pool
	log       *zap.Logger
}

// New creates a Server with all required dependencies.
func New(
	reg *registry.Registry,
	sched *scheduler.Scheduler,
	verif *stakeVerifier.Verifier,
	flOrch *fl.Orchestrator,
	db *pgxpool.Pool,
	log *zap.Logger,
) *Server {
	return &Server{
		registry:  reg,
		scheduler: sched,
		verifier:  verif,
		fl:        flOrch,
		db:        db,
		log:       log,
	}
}

// ─── Node lifecycle ──────────────────────────────────────────────────────────

// RegisterNode handles node registration including Ed25519 signature verification
// and Lightning stake verification (SRS-REG-01 through SRS-REG-05).
func (s *Server) RegisterNode(ctx context.Context, req *coordinatorv1.RegisterNodeRequest) (*coordinatorv1.RegisterNodeResponse, error) {
	if err := validateTimestamp(req.Timestamp); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "timestamp: %v", err)
	}
	if req.PublicKey == "" || req.LnNodeUri == "" || req.Capabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "public_key, ln_node_uri, and capabilities are required")
	}

	caps := registry.NodeCapabilities{
		Tier:               req.Capabilities.Tier,
		VRAMGB:             float64(req.Capabilities.VramGb),
		RAMGB:              float64(req.Capabilities.RamGb),
		BandwidthMbps:      float64(req.Capabilities.BandwidthMbps),
		SupportedTaskTypes: req.Capabilities.SupportedTaskTypes,
	}

	node, err := s.registry.Register(ctx, req.PublicKey, req.LnNodeUri, caps, req.Signature, req.Timestamp)
	if err != nil {
		s.log.Warn("node registration failed", zap.String("pubkey", req.PublicKey), zap.Error(err))
		return nil, status.Errorf(codes.PermissionDenied, "registration failed: %v", err)
	}

	// Verify Lightning stake asynchronously; return pending if stake not yet confirmed.
	stakeResult, err := s.verifier.VerifyStake(ctx, req.PublicKey, caps.Tier)
	if err != nil {
		s.log.Warn("stake verification failed", zap.String("node_id", node.NodeID.String()), zap.Error(err))
		return &coordinatorv1.RegisterNodeResponse{
			NodeId:    node.NodeID.String(),
			Status:    "pending",
			Message:   fmt.Sprintf("open a Lightning channel to the treasury (min %d sats for %s)", stakeMinSats(caps.Tier), caps.Tier),
			ErrorCode: "INSUFFICIENT_STAKE",
		}, nil
	}

	if err := s.verifier.PersistStake(ctx, node.NodeID, stakeResult); err != nil {
		s.log.Error("persisting stake failed", zap.Error(err))
	}
	if err := s.registry.Activate(ctx, node.NodeID); err != nil {
		return nil, status.Errorf(codes.Internal, "activating node: %v", err)
	}

	s.log.Info("node registered and activated",
		zap.String("node_id", node.NodeID.String()),
		zap.String("tier", caps.Tier),
		zap.Float64("bonus_mult", stakeResult.BonusMultiplier),
	)
	return &coordinatorv1.RegisterNodeResponse{
		NodeId:     node.NodeID.String(),
		Status:     "active",
		Message:    "node activated",
		StakeSats:  stakeResult.CapacitySats,
		BonusMult:  float32(stakeResult.BonusMultiplier),
	}, nil
}

// Heartbeat updates the node's last-seen timestamp and metrics.
func (s *Server) Heartbeat(ctx context.Context, req *coordinatorv1.HeartbeatRequest) (*coordinatorv1.HeartbeatResponse, error) {
	if err := validateTimestamp(req.Timestamp); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "timestamp: %v", err)
	}

	nodeID, err := uuid.Parse(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node_id: %v", err)
	}

	if _, err := s.registry.RecordHeartbeat(ctx, nodeID); err != nil {
		return nil, status.Errorf(codes.Internal, "recording heartbeat: %v", err)
	}

	return &coordinatorv1.HeartbeatResponse{
		CoordinatorVersion: coordinatorVersion,
	}, nil
}

// DeregisterNode marks a node as deregistered.
func (s *Server) DeregisterNode(ctx context.Context, req *coordinatorv1.DeregisterNodeRequest) (*coordinatorv1.DeregisterNodeResponse, error) {
	if err := validateTimestamp(req.Timestamp); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "timestamp: %v", err)
	}

	nodeID, err := uuid.Parse(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node_id: %v", err)
	}

	if err := s.registry.UpdateStatus(ctx, nodeID, registry.StatusSuspended); err != nil {
		return nil, status.Errorf(codes.Internal, "deregistering node: %v", err)
	}

	s.log.Info("node deregistered", zap.String("node_id", req.NodeId), zap.String("reason", req.Reason))
	return &coordinatorv1.DeregisterNodeResponse{Success: true, Message: "node deregistered"}, nil
}

// ─── Task streaming ──────────────────────────────────────────────────────────

// StreamTasks opens a server-side stream and pushes Task messages to the node.
// The node keeps this stream open for the duration of its session.
func (s *Server) StreamTasks(req *coordinatorv1.HeartbeatRequest, stream coordinatorv1.CoordinatorService_StreamTasksServer) error {
	nodeID := req.NodeId
	s.log.Info("task stream opened", zap.String("node_id", nodeID))

	// Scaffold: block until context is cancelled (real impl polls a Redis task queue
	// and pushes tasks as they are scheduled for this node).
	<-stream.Context().Done()
	s.log.Info("task stream closed", zap.String("node_id", nodeID))
	return nil
}

// SubmitTaskResult processes a completed task result from a node.
func (s *Server) SubmitTaskResult(ctx context.Context, result *coordinatorv1.TaskResult) (*coordinatorv1.TaskResultAck, error) {
	taskID, err := uuid.Parse(result.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid task_id: %v", err)
	}
	nodeID, err := uuid.Parse(result.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node_id: %v", err)
	}

	outputHash := hex.EncodeToString(result.OutputHash)
	if err := s.scheduler.MarkComplete(ctx, taskID, outputHash); err != nil {
		s.log.Error("marking task complete", zap.String("task_id", result.TaskId), zap.Error(err))
		return &coordinatorv1.TaskResultAck{
			Accepted:        false,
			RejectionReason: "internal error recording completion",
			PaymentStatus:   "failed",
		}, nil
	}

	// Update node reliability score positively.
	if err := s.registry.UpdateReliability(ctx, nodeID, true); err != nil {
		s.log.Warn("updating reliability after success", zap.Error(err))
	}

	s.log.Info("task result accepted",
		zap.String("task_id", result.TaskId),
		zap.String("node_id", result.NodeId),
		zap.Int64("exec_ms", result.ExecMs),
	)
	return &coordinatorv1.TaskResultAck{
		Accepted:      true,
		PaymentStatus: "dispatched",
	}, nil
}

// ─── Model versioning ────────────────────────────────────────────────────────

// GetCurrentModel returns the latest model version record.
func (s *Server) GetCurrentModel(ctx context.Context, _ *coordinatorv1.GetModelRequest) (*coordinatorv1.ModelVersion, error) {
	row := s.db.QueryRow(ctx,
		`SELECT version_id, round_number, created_at, btc_block, ots_verified, coordinator_sig
		 FROM model_versions ORDER BY round_number DESC LIMIT 1`)

	var mv coordinatorv1.ModelVersion
	var createdAt time.Time
	if err := row.Scan(&mv.VersionId, &mv.RoundNumber, &createdAt, &mv.BtcBlock, &mv.OtsVerified, &mv.CoordinatorSig); err != nil {
		return nil, status.Errorf(codes.NotFound, "no model versions found")
	}
	mv.CreatedAt = createdAt.Unix()
	return &mv, nil
}

// ListModelVersions returns paginated model version records.
func (s *Server) ListModelVersions(ctx context.Context, req *coordinatorv1.ListModelsRequest) (*coordinatorv1.ListModelsResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.Query(ctx,
		`SELECT version_id, round_number, created_at, btc_block, ots_verified, coordinator_sig
		 FROM model_versions ORDER BY round_number DESC LIMIT $1 OFFSET $2`,
		limit, req.Offset,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "querying model versions: %v", err)
	}
	defer rows.Close()

	var versions []*coordinatorv1.ModelVersion
	for rows.Next() {
		var mv coordinatorv1.ModelVersion
		var createdAt time.Time
		if err := rows.Scan(&mv.VersionId, &mv.RoundNumber, &createdAt, &mv.BtcBlock, &mv.OtsVerified, &mv.CoordinatorSig); err != nil {
			return nil, status.Errorf(codes.Internal, "scanning model version: %v", err)
		}
		mv.CreatedAt = createdAt.Unix()
		versions = append(versions, &mv)
	}

	var total int32
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM model_versions`).Scan(&total)

	return &coordinatorv1.ListModelsResponse{Versions: versions, Total: total}, nil
}

// ─── Stake ───────────────────────────────────────────────────────────────────

// GetStakeStatus returns the current Lightning channel stake info for a node.
func (s *Server) GetStakeStatus(ctx context.Context, req *coordinatorv1.GetStakeRequest) (*coordinatorv1.StakeStatus, error) {
	nodeID, err := uuid.Parse(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node_id: %v", err)
	}

	var ss coordinatorv1.StakeStatus
	var lastVerified time.Time
	err = s.db.QueryRow(ctx,
		`SELECT ns.channel_id, ns.capacity_sats, ns.local_balance_sats,
		        ns.bonus_multiplier, ns.status, ns.last_verified_at,
		        n.tier
		 FROM node_stakes ns
		 JOIN nodes n ON n.node_id = ns.node_id
		 WHERE ns.node_id = $1`, nodeID,
	).Scan(&ss.ChannelId, &ss.CapacitySats, &ss.LocalBalance,
		&ss.BonusMultiplier, &ss.Status, &lastVerified, &ss.TierMinimum)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "stake record not found for node %s", req.NodeId)
	}
	ss.LastVerifiedAt = lastVerified.Unix()
	return &ss, nil
}

// ─── Federated Learning ──────────────────────────────────────────────────────

// SubmitGradient records a gradient update from a node for the current FL round.
func (s *Server) SubmitGradient(ctx context.Context, grad *coordinatorv1.GradientUpdate) (*coordinatorv1.GradientAck, error) {
	nodeID, err := uuid.Parse(grad.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid node_id: %v", err)
	}

	sub := fl.GradientSubmission{
		NodeID:       nodeID,
		RoundID:      grad.RoundNumber,
		GradientData: grad.GradientData,
		GradientHash: grad.GradientHash,
	}

	if err := s.fl.SubmitGradient(ctx, sub); err != nil {
		s.log.Warn("gradient submission failed",
			zap.String("node_id", grad.NodeId),
			zap.Int32("round", grad.RoundNumber),
			zap.Error(err),
		)
		return &coordinatorv1.GradientAck{
			Accepted:        false,
			RejectionReason: err.Error(),
		}, nil
	}

	return &coordinatorv1.GradientAck{
		Accepted:   true,
		RewardSats: 0, // paid after round completes
	}, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// validateTimestamp rejects requests with timestamps older than antiReplayWindow
// or more than 30 seconds in the future (clock skew tolerance).
func validateTimestamp(ts int64) error {
	t := time.Unix(ts, 0)
	now := time.Now().UTC()
	if now.Sub(t) > antiReplayWindow {
		return fmt.Errorf("timestamp too old (%s ago)", now.Sub(t).Round(time.Second))
	}
	if t.Sub(now) > 30*time.Second {
		return fmt.Errorf("timestamp too far in the future")
	}
	return nil
}

// stakeMinSats returns the minimum stake in satoshis for a given tier string.
func stakeMinSats(tier string) int64 {
	switch tier {
	case "t2":
		return 500_000
	case "t3":
		return 2_000_000
	default: // t1
		return 100_000
	}
}

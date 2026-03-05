// Package scheduler assigns compute tasks to eligible nodes based on
// capability, reliability, tier, and stake bonus (SRS-SCHED-01 through 07).
package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/registry"
)

// TaskType enumerates the compute task categories.
type TaskType string

const (
	TaskInference   TaskType = "inference"
	TaskFLRound     TaskType = "fl_round"
	TaskGradientAgg TaskType = "gradient_agg"
	TaskDataIngest  TaskType = "data_ingest"
	TaskAuditRepo   TaskType = "audit_repo"
	TaskEmbedData   TaskType = "embed_data"
)

// taskMinTier maps each task type to its minimum required node tier.
var taskMinTier = map[TaskType]int{
	TaskInference:   1,
	TaskFLRound:     1,
	TaskGradientAgg: 2,
	TaskDataIngest:  2,
	TaskAuditRepo:   1,
	TaskEmbedData:   1,
}

// taskWeights maps each task type to its reward weight multiplier.
var taskWeights = map[TaskType]float64{
	TaskInference:   1.0,
	TaskFLRound:     3.0,
	TaskGradientAgg: 5.0,
	TaskDataIngest:  2.0,
	TaskAuditRepo:   1.5,
	TaskEmbedData:   1.0,
}

// baseRateSats is the configurable base reward per compute unit.
// In production this is read from Config; hardcoded here for scaffold clarity.
const baseRateSats int64 = 10

// Assignment represents a task-to-node binding.
type Assignment struct {
	TaskID      uuid.UUID
	NodeID      uuid.UUID
	NodeLNURI   string
	TaskType    TaskType
	RewardSats  int64
	TimeoutSecs int
	AssignedAt  time.Time
}

// Scheduler selects nodes for tasks and persists assignments.
type Scheduler struct {
	db       *pgxpool.Pool
	registry *registry.Registry
	log      *zap.Logger
	rng      *rand.Rand
}

// New creates a Scheduler.
func New(db *pgxpool.Pool, reg *registry.Registry, log *zap.Logger) *Scheduler {
	return &Scheduler{
		db:       db,
		registry: reg,
		log:      log,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Schedule selects the best available node for a task and records the
// assignment in the database. Returns the assignment or an error if no
// eligible nodes are available.
func (s *Scheduler) Schedule(ctx context.Context, taskType TaskType, inputHash string, timeoutSecs int) (*Assignment, error) {
	nodes, err := s.registry.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing active nodes: %w", err)
	}

	minTier := taskMinTier[taskType]
	eligible := s.filterEligible(ctx, nodes, taskType, minTier)
	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible nodes for task type %s", taskType)
	}

	node := s.selectNode(eligible)
	rewardSats := s.computeReward(ctx, node, taskType)

	taskID := uuid.New()
	assignment := &Assignment{
		TaskID:      taskID,
		NodeID:      node.NodeID,
		NodeLNURI:   node.LNNodeURI,
		TaskType:    taskType,
		RewardSats:  rewardSats,
		TimeoutSecs: timeoutSecs,
		AssignedAt:  time.Now().UTC(),
	}

	if err := s.persistAssignment(ctx, assignment, inputHash); err != nil {
		return nil, fmt.Errorf("persisting assignment: %w", err)
	}

	s.log.Info("task scheduled",
		zap.String("task_id", taskID.String()),
		zap.String("node_id", node.NodeID.String()),
		zap.String("task_type", string(taskType)),
		zap.Int64("reward_sats", rewardSats),
	)
	return assignment, nil
}

// MarkComplete updates a task to completed and triggers payment dispatch.
func (s *Scheduler) MarkComplete(ctx context.Context, taskID uuid.UUID, outputHash string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tasks
		 SET status = 'completed', output_hash = $2, completed_at = now()
		 WHERE task_id = $1`,
		taskID, outputHash,
	)
	return err
}

// MarkFailed updates a task to failed and updates the node's reliability score.
func (s *Scheduler) MarkFailed(ctx context.Context, taskID uuid.UUID, nodeID uuid.UUID) error {
	if _, err := s.db.Exec(ctx,
		`UPDATE tasks SET status = 'failed', completed_at = now() WHERE task_id = $1`,
		taskID,
	); err != nil {
		return err
	}
	return s.registry.UpdateReliability(ctx, nodeID, false)
}

// RequeueTimedOut finds tasks past their deadline and requeues them.
// Called on a periodic ticker (SRS-SCHED-02).
func (s *Scheduler) RequeueTimedOut(ctx context.Context) (int, error) {
	result, err := s.db.Exec(ctx,
		`UPDATE tasks
		 SET status = 'pending', assigned_node = NULL, started_at = NULL
		 WHERE status = 'running'
		   AND started_at + (timeout_seconds || ' seconds')::INTERVAL < now()`)
	if err != nil {
		return 0, err
	}
	n := int(result.RowsAffected())
	if n > 0 {
		s.log.Info("requeued timed-out tasks", zap.Int("count", n))
	}
	return n, nil
}

// filterEligible returns nodes that meet the minimum tier for the task.
func (s *Scheduler) filterEligible(_ context.Context, nodes []*registry.Node, _ TaskType, minTierNum int) []*registry.Node {
	tierNums := map[string]int{"t1": 1, "t2": 2, "t3": 3}
	var out []*registry.Node
	for _, n := range nodes {
		if tierNums[n.Tier] >= minTierNum {
			out = append(out, n)
		}
	}
	return out
}

// selectNode picks the best node using a weighted score:
// score = reliability × tier_multiplier × stake_bonus
// Adds jitter to avoid thundering-herd when multiple nodes score identically.
func (s *Scheduler) selectNode(nodes []*registry.Node) *registry.Node {
	tierMult := map[string]float64{"t1": 1.0, "t2": 2.5, "t3": 8.0}
	best := nodes[0]
	bestScore := -1.0

	for _, n := range nodes {
		score := n.Reliability * tierMult[n.Tier]
		// Jitter ±5%
		score *= 0.95 + s.rng.Float64()*0.10
		if score > bestScore {
			bestScore = score
			best = n
		}
	}
	return best
}

// computeReward calculates the reward in satoshis for a completed task.
// Formula from SRS-4.4.2: base_rate × tier_mult × task_weight × stake_bonus
// Uptime bonus and quality factor are applied post-completion.
func (s *Scheduler) computeReward(ctx context.Context, node *registry.Node, taskType TaskType) int64 {
	tierMult := map[string]float64{"t1": 1.0, "t2": 2.5, "t3": 8.0}
	weight := taskWeights[taskType]

	// Fetch stake bonus from DB.
	var bonusMult float64 = 1.0
	_ = s.db.QueryRow(ctx,
		`SELECT bonus_multiplier FROM node_stakes WHERE node_id = $1`, node.NodeID,
	).Scan(&bonusMult)

	reward := float64(baseRateSats) * tierMult[node.Tier] * weight * bonusMult
	return int64(reward)
}

func (s *Scheduler) persistAssignment(ctx context.Context, a *Assignment, inputHash string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO tasks
		    (task_id, task_type, assigned_node, status, input_hash,
		     reward_sats, submitted_at, started_at, timeout_seconds)
		 VALUES ($1, $2, $3, 'running', $4, $5, now(), now(), $6)`,
		a.TaskID, string(a.TaskType), a.NodeID, inputHash,
		a.RewardSats, a.TimeoutSecs,
	)
	return err
}

package deleter

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"go.uber.org/zap"
)

type DeletionTask struct {
	UserID   uuid.UUID
	ShortIDs []string
}

type URLDeleter struct {
	repo          repository.URLRepository
	logger        *zap.Logger
	tasks         chan DeletionTask
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	flushInterval time.Duration
	maxBatchSize  int
}

func NewURLDeleter(
	repo repository.URLRepository,
	logger *zap.Logger,
	bufferSize int,
	flushInterval time.Duration,
	maxBatchSize int,

) *URLDeleter {
	ctx, cancel := context.WithCancel(context.Background())

	d := &URLDeleter{
		repo:          repo,
		logger:        logger,
		tasks:         make(chan DeletionTask, bufferSize),
		ctx:           ctx,
		cancel:        cancel,
		flushInterval: flushInterval,
		maxBatchSize:  maxBatchSize,
	}
	d.startWorker(ctx)
	return d
}

func (d *URLDeleter) startWorker(ctx context.Context) {
	d.wg.Add(1)
	go d.worker(ctx)
}

func (d *URLDeleter) worker(ctx context.Context) {
	defer d.wg.Done()

	d.logger.Info("deletion worker started")

	batch := make(map[uuid.UUID][]string)
	timer := time.NewTimer(time.Hour)

	flush := func(trigger string) {
		if len(batch) == 0 {
			d.logger.Info("flush: empty batch, skipping", zap.String("trigger", trigger))
			return
		}

		totalIDs := 0
		for _, ids := range batch {
			totalIDs += len(ids)
		}

		d.logger.Info("FLUSHING batch",
			zap.String("trigger", trigger),
			zap.Int("users", len(batch)),
			zap.Int("total_ids", totalIDs))

		for userID, ids := range batch {
			if err := d.repo.MarkAsDeleted(ctx, userID, ids); err != nil {
				d.logger.Error("batch delete failed", zap.Error(err), zap.String("user_id", userID.String()))
			} else {
				d.logger.Info("batch delete success", zap.String("user_id", userID.String()), zap.Int("count", len(ids)))
			}
		}
		clear(batch)
	}

	for {
		select {
		case <-d.ctx.Done():
			flush("shutdown")
			return

		case task := <-d.tasks:
			if len(task.ShortIDs) == 0 {
				continue
			}
			d.logger.Info("received deletion task", zap.String("user_id",
				task.UserID.String()), zap.Int("count", len(task.ShortIDs)))

			// Дедупликация
			seen := make(map[string]bool)
			added := 0
			for _, id := range task.ShortIDs {
				if !seen[id] {
					seen[id] = true
					batch[task.UserID] = append(batch[task.UserID], id)
					added++
				}
			}
			d.logger.Info("added to batch", zap.Int("added", added),
				zap.Int("user_batch_size", len(batch[task.UserID])))

			d.logger.Info("immediate flush after task")
			flush("immediate after task")

			// Immediate flush для большого юзера
			if d.maxBatchSize > 0 && len(batch[task.UserID]) >= d.maxBatchSize {
				d.logger.Info("immediate flush for large batch")
				_ = d.repo.MarkAsDeleted(ctx, task.UserID, batch[task.UserID])
				delete(batch, task.UserID)
			}

		case <-timer.C:
			flush("timer")
		}
	}
}

func (d *URLDeleter) Submit(task DeletionTask) {
	d.logger.Info("Submit: attempting to send task", zap.String("user_id", task.UserID.String()), zap.Int("count", len(task.ShortIDs)))
	select {
	case d.tasks <- task:
		d.logger.Info("submitted deletion task",
			zap.String("user_id", task.UserID.String()),
			zap.Int("count", len(task.ShortIDs)))
	case <-d.ctx.Done():
		return
	default:
		d.logger.Warn("queue full, dropping task",
			zap.String("user_id", task.UserID.String()),
			zap.Int("count", len(task.ShortIDs)))
	}
}

func (d *URLDeleter) Close() error {
	d.cancel()
	d.wg.Wait()
	return nil
}

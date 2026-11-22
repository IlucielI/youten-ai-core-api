package workers

import (
	"context"
	"log"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/queue"
	"code-base-golang/internal/services"
)

// WorkerServer manages and registers all background worker subscriptions.
type WorkerServer struct {
	cfg        config.Config
	svc        *services.Service
	sub        EventSubscriber
	extractSem chan struct{}
}

// New creates a new WorkerServer instance.
func New(cfg config.Config, svc *services.Service, sub EventSubscriber) *WorkerServer {
	return &WorkerServer{
		cfg:        cfg,
		svc:        svc,
		sub:        sub,
		extractSem: make(chan struct{}, 3),
	}
}

// Shutdown gracefully waits for in-flight jobs in underlying subscribers to drain within ctx deadline.
func (w *WorkerServer) Shutdown(ctx context.Context) error {
	if w == nil || w.sub == nil {
		return nil
	}
	if drainable, ok := w.sub.(DrainableEventSubscriber); ok {
		return drainable.ShutdownWorkers(ctx)
	}
	return nil
}

// DefaultStageTimeout defines the maximum deadline allowed for any single background pipeline stage.
const DefaultStageTimeout = 15 * time.Minute

// subscribe registers a subscription with options if the subscriber supports them,
// gracefully falling back to standard MustSubscribe if only basic EventSubscriber is implemented.
func (w *WorkerServer) subscribe(topic string, handler WorkerHandler, opts ...SubscribeOption) {
	if w == nil || w.sub == nil {
		return
	}
	if configurable, ok := w.sub.(ConfigurableEventSubscriber); ok {
		configurable.MustSubscribeWithWorkerOptions(topic, handler, opts...)
		return
	}
	w.sub.MustSubscribe(topic, handler)
}

// RegisterWorker registers all worker subscriptions with differentiated stage concurrency
// and elastic dynamic idle auto-reaping.
func (w *WorkerServer) RegisterWorker() {
	if w == nil || w.sub == nil {
		return
	}

	log.Println("Registering worker subscriptions to Domain Events with elastic dynamic pool...")

	const defaultIdleReap = 30 * time.Second

	// CPU-bound media extractions: cap at 3 workers to prevent host CPU starvation
	w.subscribe(
		constants.TopicRecordingUploaded,
		w.HandleRecordingExtract,
		WithConcurrency(3),
		WithPrefetch(3),
		WithIdleTimeout(defaultIdleReap),
	)
	w.subscribe(
		constants.TopicRecordingExtract,
		w.HandleRecordingExtract,
		WithConcurrency(3),
		WithPrefetch(3),
		WithIdleTimeout(defaultIdleReap),
	)

	// Remote media link import (YouTube/Google Drive): I/O bound
	w.subscribe(
		constants.TopicRecordingImport,
		w.HandleRecordingImport,
		WithConcurrency(4),
		WithPrefetch(8),
		WithIdleTimeout(defaultIdleReap),
	)

	// Speech-to-text audio transcription: I/O bound to STT endpoint, scale up to 8 workers
	w.subscribe(
		constants.TopicRecordingTranscribe,
		w.HandleRecordingTranscribe,
		WithConcurrency(8),
		WithPrefetch(16),
		WithIdleTimeout(defaultIdleReap),
	)

	// Structured LLM Summarization: I/O bound to LLM API, scale up to 6 workers
	w.subscribe(
		constants.TopicRecordingSummarize,
		w.HandleRecordingSummarize,
		WithConcurrency(6),
		WithPrefetch(12),
		WithIdleTimeout(defaultIdleReap),
	)

	// Semantic Vector Indexing: I/O bound to Embedding API + pgvector batch insert
	w.subscribe(
		constants.TopicRecordingIndex,
		w.HandleRecordingIndex,
		WithConcurrency(6),
		WithPrefetch(12),
		WithIdleTimeout(defaultIdleReap),
	)

	// Post-processing Analytics & Chapterization: best-effort downstream analysis
	w.subscribe(
		constants.TopicRecordingAnalytics,
		w.HandleRecordingAnalytics,
		WithConcurrency(4),
		WithPrefetch(8),
		WithIdleTimeout(defaultIdleReap),
	)
	w.subscribe(
		constants.TopicRecordingChapterize,
		w.HandleRecordingChapterize,
		WithConcurrency(4),
		WithPrefetch(8),
		WithIdleTimeout(defaultIdleReap),
	)

	// Administrative audit logs: lightweight
	w.subscribe(
		constants.TopicAdminAuditLog,
		w.HandleAdminAuditLog,
		WithConcurrency(2),
		WithPrefetch(4),
		WithIdleTimeout(defaultIdleReap),
	)

	log.Println("Done RegisterWorker")
}

// StartCleanupTicker runs a periodic background loop to clean up expired recordings and their files.
func (w *WorkerServer) StartCleanupTicker(ctx context.Context, interval time.Duration) {
	if w == nil || w.svc == nil || interval <= 0 || ctx == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				batchSize := w.cfg.CleanupBatchSize
				if batchSize <= 0 {
					batchSize = constants.DefaultExpiredRecordingsCleanupBatchSize
				}
				count, err := w.svc.CleanupExpiredRecordings(ctx, batchSize)
				if err != nil {
					log.Printf("[CLEANUP ERROR] failed to clean expired recordings: %v", err)
				} else if count > 0 {
					log.Printf("[CLEANUP] successfully removed %d expired recordings and storage objects", count)
				}

				staleCount, errStale := w.svc.CleanupStaleRecordings(ctx, 2*time.Hour)
				if errStale != nil {
					log.Printf("[CLEANUP ERROR] failed to sweep stale recordings: %v", errStale)
				} else if staleCount > 0 {
					log.Printf("[CLEANUP] successfully marked %d stale recordings as failed (timed out)", staleCount)
				}
			}
		}
	}()
}

// HandleRecordingImport handles asynchronous remote media link imports (e.g. YouTube).
func (w *WorkerServer) HandleRecordingImport(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse import payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[WORKER] Started Media Link Import for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessImport(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Media link import failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
		return false
	}
	log.Printf("[WORKER] Finished Media Link Import for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleRecordingExtract handles audio extraction from uploaded video/audio files.
func (w *WorkerServer) HandleRecordingExtract(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse extract payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	// Acquire extraction semaphore slot to strictly cap CPU-intensive FFmpeg tasks across all extract topics
	if w.extractSem != nil {
		select {
		case w.extractSem <- struct{}{}:
			defer func() { <-w.extractSem }()
		case <-stageCtx.Done():
			log.Printf("[WORKER ERROR] Audio extraction timed out waiting for semaphore slot for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
			return false
		}
	}

	start := time.Now()
	log.Printf("[WORKER] Started Audio Extraction for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessExtraction(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Audio extraction failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
		return false
	}
	log.Printf("[WORKER] Finished Audio Extraction for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleRecordingTranscribe handles speech-to-text audio transcription.
func (w *WorkerServer) HandleRecordingTranscribe(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse transcribe payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[WORKER] Started Transcription for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessTranscription(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Transcription failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
		return false
	}
	log.Printf("[WORKER] Finished Transcription for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleRecordingSummarize handles structured summary generation via LLM.
func (w *WorkerServer) HandleRecordingSummarize(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse summarize payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[WORKER] Started Summarization for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessSummarization(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Summarization failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
		return false
	}
	log.Printf("[WORKER] Finished Summarization for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleRecordingIndex handles semantic chunking and vector embedding generation.
func (w *WorkerServer) HandleRecordingIndex(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse index payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[WORKER] Started Indexing for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessIndexing(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Indexing failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
		return false
	}
	log.Printf("[WORKER] Finished Indexing for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleRecordingAnalytics computes speaker talk-time and participation share.
func (w *WorkerServer) HandleRecordingAnalytics(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse analytics payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[WORKER] Started Analytics for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessAnalytics(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Analytics failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
	}
	log.Printf("[WORKER] Finished Analytics for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleRecordingChapterize generates timestamped chapters and key moment highlights.
func (w *WorkerServer) HandleRecordingChapterize(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.RecordingPipelinePayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse chapterize payload: %v", err)
		return false
	}
	if p.RequestID != "" {
		ctx = ctxmeta.WithRequestID(ctx, p.RequestID)
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	log.Printf("[WORKER] Started Chapterization for recording %s (req_id=%s)", p.RecordingID.String(), p.RequestID)
	if err := w.svc.ProcessChapterization(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] Chapterization failed for recording %s in %s: %v (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), err, p.RequestID)
	}
	log.Printf("[WORKER] Finished Chapterization for recording %s in %s (req_id=%s)", p.RecordingID.String(), time.Since(start).Round(time.Millisecond), p.RequestID)
	return true
}

// HandleAdminAuditLog persists an administrative audit log received from the message broker.
func (w *WorkerServer) HandleAdminAuditLog(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p queue.AdminAuditPayload
	if err := queue.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse admin audit payload: %v", err)
		return false
	}

	stageCtx, cancel := context.WithTimeout(ctx, DefaultStageTimeout)
	defer cancel()

	start := time.Now()
	if err := w.svc.RecordAdminAuditLog(stageCtx, p); err != nil {
		log.Printf("[WORKER ERROR] failed to persist admin audit log in %s: %v", time.Since(start).Round(time.Millisecond), err)
		return false
	}
	return true
}

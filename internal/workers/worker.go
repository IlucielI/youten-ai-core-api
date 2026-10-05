package workers

import (
	"context"
	"log"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/services"
)

// WorkerServer manages and registers all background worker subscriptions.
type WorkerServer struct {
	cfg config.Config
	svc *services.Service
	sub EventSubscriber
}

// New creates a new WorkerServer instance.
func New(cfg config.Config, svc *services.Service, sub EventSubscriber) *WorkerServer {
	return &WorkerServer{
		cfg: cfg,
		svc: svc,
		sub: sub,
	}
}

// RegisterWorker registers all worker subscriptions to Domain Events.
func (w *WorkerServer) RegisterWorker() {
	if w == nil || w.sub == nil {
		return
	}

	log.Println("Registering worker subscriptions to Domain Events...")

	w.sub.MustSubscribe(constants.TopicRecordingUploaded, w.HandleRecordingExtract)
	w.sub.MustSubscribe(constants.TopicRecordingExtract, w.HandleRecordingExtract)
	w.sub.MustSubscribe(constants.TopicRecordingTranscribe, w.HandleRecordingTranscribe)
	w.sub.MustSubscribe(constants.TopicRecordingSummarize, w.HandleRecordingSummarize)
	w.sub.MustSubscribe(constants.TopicRecordingIndex, w.HandleRecordingIndex)
	w.sub.MustSubscribe(constants.TopicRecordingAnalytics, w.HandleRecordingAnalytics)
	w.sub.MustSubscribe(constants.TopicRecordingChapterize, w.HandleRecordingChapterize)

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
				count, err := w.svc.CleanupExpiredRecordings(ctx, 100)
				if err != nil {
					log.Printf("[CLEANUP ERROR] failed to clean expired recordings: %v", err)
				} else if count > 0 {
					log.Printf("[CLEANUP] successfully removed %d expired recordings and storage objects", count)
				}
			}
		}
	}()
}

// HandleRecordingExtract handles audio extraction from uploaded video/audio files.
func (w *WorkerServer) HandleRecordingExtract(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p payload.RecordingPipelinePayload
	if err := payload.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse extract payload: %v", err)
		return false
	}

	log.Printf("[WORKER] Started Audio Extraction for recording %s", p.RecordingID.String())
	if err := w.svc.ProcessExtraction(ctx, p); err != nil {
		log.Printf("[WORKER ERROR] Audio extraction failed for recording %s: %v", p.RecordingID.String(), err)
		return false
	}
	log.Printf("[WORKER] Finished Audio Extraction for recording %s", p.RecordingID.String())
	return true
}

// HandleRecordingTranscribe handles speech-to-text audio transcription.
func (w *WorkerServer) HandleRecordingTranscribe(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p payload.RecordingPipelinePayload
	if err := payload.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse transcribe payload: %v", err)
		return false
	}

	log.Printf("[WORKER] Started Transcription for recording %s", p.RecordingID.String())
	if err := w.svc.ProcessTranscription(ctx, p); err != nil {
		log.Printf("[WORKER ERROR] Transcription failed for recording %s: %v", p.RecordingID.String(), err)
		return false
	}
	log.Printf("[WORKER] Finished Transcription for recording %s", p.RecordingID.String())
	return true
}

// HandleRecordingSummarize handles structured summary generation via LLM.
func (w *WorkerServer) HandleRecordingSummarize(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p payload.RecordingPipelinePayload
	if err := payload.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse summarize payload: %v", err)
		return false
	}

	log.Printf("[WORKER] Started Summarization for recording %s", p.RecordingID.String())
	if err := w.svc.ProcessSummarization(ctx, p); err != nil {
		log.Printf("[WORKER ERROR] Summarization failed for recording %s: %v", p.RecordingID.String(), err)
		return false
	}
	log.Printf("[WORKER] Finished Summarization for recording %s", p.RecordingID.String())
	return true
}

// HandleRecordingIndex handles semantic chunking and vector embedding generation.
func (w *WorkerServer) HandleRecordingIndex(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p payload.RecordingPipelinePayload
	if err := payload.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse index payload: %v", err)
		return false
	}

	log.Printf("[WORKER] Started Indexing for recording %s", p.RecordingID.String())
	if err := w.svc.ProcessIndexing(ctx, p); err != nil {
		log.Printf("[WORKER ERROR] Indexing failed for recording %s: %v", p.RecordingID.String(), err)
		return false
	}
	log.Printf("[WORKER] Finished Indexing for recording %s", p.RecordingID.String())
	return true
}

// HandleRecordingAnalytics computes speaker talk-time and participation share.
func (w *WorkerServer) HandleRecordingAnalytics(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p payload.RecordingPipelinePayload
	if err := payload.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse analytics payload: %v", err)
		return false
	}

	log.Printf("[WORKER] Started Analytics for recording %s", p.RecordingID.String())
	_ = w.svc.ProcessAnalytics(ctx, p)
	log.Printf("[WORKER] Finished Analytics for recording %s", p.RecordingID.String())
	return true
}

// HandleRecordingChapterize generates timestamped chapters and key moment highlights.
func (w *WorkerServer) HandleRecordingChapterize(ctx context.Context, msg []byte) bool {
	if w.svc == nil {
		return false
	}
	var p payload.RecordingPipelinePayload
	if err := payload.Parse(msg, &p); err != nil {
		log.Printf("[WORKER ERROR] failed to parse chapterize payload: %v", err)
		return false
	}

	log.Printf("[WORKER] Started Chapterization for recording %s", p.RecordingID.String())
	_ = w.svc.ProcessChapterization(ctx, p)
	log.Printf("[WORKER] Finished Chapterization for recording %s", p.RecordingID.String())
	return true
}

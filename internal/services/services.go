package services

import (
	"context"
	"net/http"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/repositories"
)

// MediaFetcher defines a function signature to safely stream external media.
type MediaFetcher func(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error)

// Service is the unified application service container.
type Service struct {
	cfg            config.Config
	storage        FileStorage
	repo           *repositories.Repositories
	publisher      EventPublisher
	mailer         EmailSender
	stt            STTProvider
	llm            LLMProvider
	embedding      EmbeddingProvider
	audioExtractor AudioExtractor
	mediaFetcher   MediaFetcher
}

// New creates a new unified service container.
func New(cfg config.Config, repo *repositories.Repositories, storage FileStorage, publisher ...EventPublisher) *Service {
	var pub EventPublisher
	if len(publisher) > 0 {
		pub = publisher[0]
	}
	return &Service{
		cfg:       cfg,
		storage:   storage,
		repo:      repo,
		publisher: pub,
	}
}

// SetStorage allows injecting or overriding storage adapter (e.g. for testing or switching provider).
func (s *Service) SetStorage(storage FileStorage) {
	s.storage = storage
}

// Storage returns the underlying file storage provider.
func (s *Service) Storage() FileStorage {
	if s == nil {
		return nil
	}
	return s.storage
}

// SetRepositories allows injecting or overriding repositories (e.g. for testing).
func (s *Service) SetRepositories(repo *repositories.Repositories) {
	s.repo = repo
}

// Repositories returns the underlying repositories container.
func (s *Service) Repositories() *repositories.Repositories {
	if s == nil {
		return nil
	}
	return s.repo
}

// SetPublisher allows injecting or overriding event publisher adapter (e.g. for testing or switching broker).
func (s *Service) SetPublisher(pub EventPublisher) {
	s.publisher = pub
}

// Publisher returns the underlying event publisher adapter.
func (s *Service) Publisher() EventPublisher {
	if s == nil {
		return nil
	}
	return s.publisher
}

// SetMailer allows injecting or overriding email sender adapter (e.g. for testing or switching mail provider).
func (s *Service) SetMailer(mailer EmailSender) {
	s.mailer = mailer
}

// WithMailer fluently sets the email sender adapter.
func (s *Service) WithMailer(mailer EmailSender) *Service {
	s.mailer = mailer
	return s
}

// Mailer returns the underlying email sender adapter.
func (s *Service) Mailer() EmailSender {
	if s == nil {
		return nil
	}
	return s.mailer
}

// SetSTT allows injecting or overriding STT provider adapter.
func (s *Service) SetSTT(stt STTProvider) {
	s.stt = stt
}

// WithSTT fluently sets the STT provider adapter.
func (s *Service) WithSTT(stt STTProvider) *Service {
	s.stt = stt
	return s
}

// STT returns the underlying STT provider adapter.
func (s *Service) STT() STTProvider {
	if s == nil {
		return nil
	}
	return s.stt
}

// SetLLM allows injecting or overriding LLM provider adapter.
func (s *Service) SetLLM(llm LLMProvider) {
	s.llm = llm
}

// WithLLM fluently sets the LLM provider adapter.
func (s *Service) WithLLM(llm LLMProvider) *Service {
	s.llm = llm
	return s
}

// LLM returns the underlying LLM provider adapter.
func (s *Service) LLM() LLMProvider {
	if s == nil {
		return nil
	}
	return s.llm
}

// SetEmbedding allows injecting or overriding Embedding provider adapter.
func (s *Service) SetEmbedding(emb EmbeddingProvider) {
	s.embedding = emb
}

// WithEmbedding fluently sets the Embedding provider adapter.
func (s *Service) WithEmbedding(emb EmbeddingProvider) *Service {
	s.embedding = emb
	return s
}

// Embedding returns the underlying Embedding provider adapter.
func (s *Service) Embedding() EmbeddingProvider {
	if s == nil {
		return nil
	}
	return s.embedding
}

// SetAudioExtractor allows injecting or overriding AudioExtractor adapter.
func (s *Service) SetAudioExtractor(ext AudioExtractor) {
	s.audioExtractor = ext
}

// WithAudioExtractor fluently sets the AudioExtractor adapter.
func (s *Service) WithAudioExtractor(ext AudioExtractor) *Service {
	s.audioExtractor = ext
	return s
}

// AudioExtractor returns the underlying AudioExtractor adapter.
func (s *Service) AudioExtractor() AudioExtractor {
	if s == nil {
		return nil
	}
	return s.audioExtractor
}

// SetMediaFetcher allows injecting or overriding external media streaming fetcher (used in testing).
func (s *Service) SetMediaFetcher(fetcher MediaFetcher) {
	s.mediaFetcher = fetcher
}

// WithMediaFetcher fluently sets the external media streaming fetcher.
func (s *Service) WithMediaFetcher(fetcher MediaFetcher) *Service {
	s.mediaFetcher = fetcher
	return s
}

// wrapError wraps unknown or system errors into structured AppError.
func (s *Service) wrapError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	return constants.ErrInternalServerError.Wrap(err)
}

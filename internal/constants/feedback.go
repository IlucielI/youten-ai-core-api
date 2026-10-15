package constants

const (
	FeedbackCategoryBug         = "bug"
	FeedbackCategoryFeature     = "feature"
	FeedbackCategoryImprovement = "improvement"
	FeedbackCategoryPraise      = "praise"
	FeedbackCategoryGeneral     = "general"
)

const (
	FeedbackStatusNew      = "new"
	FeedbackStatusReviewed = "reviewed"
	FeedbackStatusResolved = "resolved"
)

const (
	MaxFeedbackAttachmentSizeBytes = 10 * 1024 * 1024 // 10MB
	MaxFeedbackAttachmentsCount    = 5
)

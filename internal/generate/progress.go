package generate

// ProgressStage identifies an observable generation lifecycle step.
type ProgressStage uint8

const (
	ProgressRunStarted ProgressStage = iota + 1
	ProgressIntermediateGenerating
	ProgressIntermediateReady
	ProgressTargetGenerating
	ProgressCandidateGenerating
	ProgressProviderProgress
	ProgressCandidateReady
	ProgressTargetReady
	ProgressTargetSkipped
	ProgressRunCompleted
)

// ProgressEvent reports work without exposing prompts, credentials, or image data.
type ProgressEvent struct {
	Stage           ProgressStage
	RunID           string
	TargetID        string
	Current         int
	Total           int
	Candidate       int
	Candidates      int
	ProviderCurrent int
}

func (opts Options) report(event ProgressEvent) {
	if opts.Progress == nil {
		return
	}
	event.RunID = opts.RunID
	opts.Progress(event)
}

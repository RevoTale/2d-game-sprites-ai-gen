// Package provider isolates image generation backends behind a small interface.
//
// The Fake provider is deterministic and used by normal tests. OpenAI is the
// first real provider. It uses the generations endpoint for prompt-only targets
// and the edits endpoint when reference images are present. Either endpoint may
// need a valid square canvas larger than the final sprite target, leaving imageio
// to aspect-fit the returned PNG. Providers must declare whether they support
// image references so generate can fail required-reference targets before making
// a request that would silently ignore important visual guidance.
package provider

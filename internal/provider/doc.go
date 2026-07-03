// Package provider isolates image generation backends behind a small interface.
//
// The Fake provider is deterministic and used by normal tests. OpenAI is the
// first real provider. Providers must declare whether they support image
// references so generate can fail required-reference targets before making a
// request that would silently ignore important visual guidance.
package provider

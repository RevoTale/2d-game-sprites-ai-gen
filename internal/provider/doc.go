// Package provider isolates image generation backends behind a small interface.
//
// Providers declare reference, mask, and progress capabilities before
// generation. Animated consistency is orchestrated by generate through one
// canonical character master and complete semantic animation boards without
// provider masks. Mask capability remains available to reusable static flows.
// Fake is available only through --fake.
package provider

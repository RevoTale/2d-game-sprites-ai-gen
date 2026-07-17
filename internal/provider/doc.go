// Package provider isolates image generation backends behind a small interface.
//
// Providers declare reference, mask, and progress capabilities before generation.
// OpenAI uses prompt generation or ordered image
// edits; animated consistency is orchestrated by generate through approved
// directional seeds, complete animation rows, and masks. Fake is available only
// through --fake.
package provider

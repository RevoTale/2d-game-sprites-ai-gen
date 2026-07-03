// Package generate owns draft run state and provider-backed target generation.
//
// A run is resumable and represented by manifest.json plus target folders.
// Generation writes prompt evidence, a raw provider candidate, a normalized PNG
// candidate, and QA metadata. Accepted or deployed targets are skipped unless
// Force is set, which prevents accidental replacement of reviewed work.
package generate

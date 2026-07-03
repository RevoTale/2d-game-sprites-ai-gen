// Package review records visual QA decisions for generated targets.
//
// Review deliberately stays separate from generation: the provider can only
// produce an image, while review records whether that candidate is accepted or
// rejected. Rejections require a reason so future regeneration has actionable
// feedback; accepted bulk reviews receive a default audit note when no reason
// is provided.
package review

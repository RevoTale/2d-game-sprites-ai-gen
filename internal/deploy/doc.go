// Package deploy previews and copies accepted target images.
//
// Deploy is intentionally conservative. BuildPlan reports exactly which files
// would be replaced and which would remain unchanged. Execute copies only
// accepted or already deployed normalized target images, so partial pack updates
// never erase existing production files for pending or rejected targets.
package deploy

// Package pack is the boundary for user-authored sprite-pack input.
//
// It owns the JSON schema for sprites.json, THEME.md loading, unknown-field
// rejection, ID validation, reference-file validation, and safe relative path
// checks. Other packages should receive an already validated Pack instead of
// re-checking schema rules locally.
package pack

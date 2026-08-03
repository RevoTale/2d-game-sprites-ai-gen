// Package pack is the boundary for user-authored sprite-pack input.
//
// It owns strict sprites.json V5 decoding, unknown-field rejection, ID and
// reference validation, and safe relative paths. Other packages receive an
// already validated Pack and must not load parallel theme or descriptor files.
package pack

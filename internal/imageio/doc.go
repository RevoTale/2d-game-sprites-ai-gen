// Package imageio handles PNG-oriented filesystem operations.
//
// It validates normalized candidate dimensions, aspect-fits larger provider
// canvases into exact target sizes, copies accepted files to deploy destinations,
// and assembles review sheets from normalized target images. It does not crop
// AI-generated sheets into frames; sheets are output artifacts only.
package imageio

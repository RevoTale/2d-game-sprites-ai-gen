// Package envfile reads simple KEY=value dotenv files for CLI configuration.
//
// The package intentionally supports only the subset this CLI needs: blank
// lines, comments, KEY=value entries, and matching single or double quotes
// around values. It does not execute shell syntax or expand variables.
package envfile

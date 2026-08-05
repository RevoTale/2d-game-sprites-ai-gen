package pack_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/testkit"
	"github.com/stretchr/testify/require"
)

func TestValidateV6RequiresExactOrderedAuthoredDirections(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		with    string
	}{
		{
			name: "missing",
			replace: `,
        {"id":"right","description":"Right side view.","reference":{"path":"direction-right.png","description":"Facing and topology evidence only."}}`,
		},
		{
			name:    "wrong order",
			replace: `"id":"down","description":"Front/down view."`,
			with:    `"id":"left","description":"Front/down view."`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testkit.WritePack(t)
			rewritePack(t, dir, tt.replace, tt.with)

			_, err := pack.Load(dir)

			require.Error(t, err)
			require.Contains(t, err.Error(), "direction")
		})
	}
}

func TestValidateV6RequiresDirectionReferenceAtNativeObjectSize(t *testing.T) {
	dir := testkit.WritePack(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "direction-right.png"),
		testkit.PNG(t, 8, 8),
		0o644,
	))

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, "is 8x8, expected 16x16")
}

func TestValidateV6RejectsDirectionReferenceOutsidePackAndDeployRoots(t *testing.T) {
	dir := testkit.WritePack(t)
	outside := filepath.Join(filepath.Dir(dir), "outside.png")
	require.NoError(t, os.WriteFile(outside, testkit.PNG(t, 16, 16), 0o644))
	rewritePack(t, dir, `"path":"direction-right.png"`, `"path":"../outside.png"`)

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, "outside the pack or deploy directory")
}

func TestValidateV6ReservesDerivedDirectionReferenceIDsPackWide(t *testing.T) {
	dir := testkit.WritePackWithReferences(t)
	rewritePack(
		t,
		dir,
		`"id":"identity"`,
		`"id":"direction-reference-blood-duelist-right"`,
	)

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, `duplicate reference id "direction-reference-blood-duelist-right"`)
}

func TestValidateV6RequiresLockedPaletteContract(t *testing.T) {
	dir := testkit.WritePack(t)
	rewritePack(t, dir, `"maxColors":32`, `"maxColors":64`)

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, "maxColors=32")
}

func TestValidateV6ConfinesGuideDeploymentAndMatchesStyleReference(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		with    string
		want    string
	}{
		{
			name:    "outside style references",
			replace: `"deploy":{"path":"references/style/compact-dark-fantasy-style-v1.png"}`,
			with:    `"deploy":{"path":"other/guide.png"}`,
			want:    "must stay under references/style",
		},
		{
			name:    "reference mismatch",
			replace: `"deploy":{"path":"references/style/compact-dark-fantasy-style-v1.png"}`,
			with:    `"deploy":{"path":"references/style/other.png"}`,
			want:    "style reference path must equal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testkit.WritePack(t)
			rewritePack(t, dir, tt.replace, tt.with)

			_, err := pack.Load(dir)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateV6RejectsDuplicateObjectAndFrameIDs(t *testing.T) {
	tests := []struct {
		name    string
		replace string
		with    string
		want    string
	}{
		{
			name:    "object",
			replace: `"id":"grass"`,
			with:    `"id":"blood-duelist"`,
			want:    `duplicate object id "blood-duelist"`,
		},
		{
			name:    "frame",
			replace: `{"id":"contact","description":"Hit."}`,
			with:    `{"id":"00","description":"Hit."}`,
			want:    `duplicate frame id "00"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testkit.WritePack(t)
			rewritePack(t, dir, tt.replace, tt.with)

			_, err := pack.Load(dir)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateV6RejectsUnknownDeployPlaceholder(t *testing.T) {
	dir := testkit.WritePack(t)
	rewritePack(t, dir, `{object}.png`, `{unknown}.png`)

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, `unknown deploy placeholder "{unknown}"`)
}

func TestValidateV6ReservesStyleGuideObjectID(t *testing.T) {
	dir := testkit.WritePack(t)
	rewritePack(t, dir, `"id":"grass"`, `"id":"style-guide"`)

	_, err := pack.Load(dir)

	require.ErrorContains(t, err, `object id "style-guide" is reserved`)
}

func TestLoadV6AcceptsFixtureAndDoesNotReturnTheme(t *testing.T) {
	dir := testkit.WritePack(t)

	p, err := pack.Load(dir)

	require.NoError(t, err)
	require.Len(t, p.Objects, 2)
	require.Equal(t, pack.KindAnimated, p.Objects[0].Kind)
	require.Equal(t, pack.KindStatic, p.Objects[1].Kind)
}

func rewritePack(t *testing.T, dir, old, replacement string) {
	t.Helper()
	path := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	updated := strings.Replace(string(data), old, replacement, 1)
	require.NotEqual(t, string(data), updated, "fixture replacement did not match")
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
}

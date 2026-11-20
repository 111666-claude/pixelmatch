package pixelmatch

import (
	"image"
	"reflect"
	"testing"
)

// regionTestImage returns a white image with the given rectangles painted
// black, simulating changed UI elements.
func regionTestImage(rect image.Rectangle, diffs ...image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(rect)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 255, 255, 255
	}
	for _, d := range diffs {
		for y := d.Min.Y; y < d.Max.Y; y++ {
			for x := d.Min.X; x < d.Max.X; x++ {
				o := img.PixOffset(x, y)
				img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 0, 0, 0, 255
			}
		}
	}
	return img
}

func TestRegionsTwoSeparatedButtons(t *testing.T) {
	rect := image.Rect(0, 0, 60, 30)
	button1 := image.Rect(5, 5, 13, 13)
	button2 := image.Rect(40, 10, 52, 20)
	a := regionTestImage(rect)
	b := regionTestImage(rect, button1, button2)

	var regions []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&regions, RegionConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	if want := 64 + 120; diff != want {
		t.Fatalf("got %d diffs, want %d", diff, want)
	}
	want := []Region{
		{Bounds: button1, Pixels: 64},
		{Bounds: button2, Pixels: 120},
	}
	if !reflect.DeepEqual(regions, want) {
		t.Fatalf("got %+v, want %+v", regions, want)
	}
	if total := regions[0].Pixels + regions[1].Pixels; total != diff {
		t.Fatalf("region pixels %d do not add up to diff count %d", total, diff)
	}

	// The Regions option must not change the pixel comparison.
	plain, err := MatchPixel(a, b, IncludeAntiAlias)
	if err != nil || plain != diff {
		t.Fatalf("without Regions: got %d, %v; want %d", plain, err, diff)
	}
}

func TestRegionsDiagonalTouch(t *testing.T) {
	rect := image.Rect(0, 0, 4, 4)
	a := regionTestImage(rect)
	b := regionTestImage(rect, image.Rect(0, 0, 1, 1), image.Rect(1, 1, 2, 2))

	var r4 []Region
	if _, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&r4, RegionConfig{Connectivity: 4})); err != nil {
		t.Fatal(err)
	}
	want4 := []Region{
		{Bounds: image.Rect(0, 0, 1, 1), Pixels: 1},
		{Bounds: image.Rect(1, 1, 2, 2), Pixels: 1},
	}
	if !reflect.DeepEqual(r4, want4) {
		t.Fatalf("4-connectivity: got %+v, want %+v", r4, want4)
	}

	var r8 []Region
	if _, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&r8, RegionConfig{Connectivity: 8})); err != nil {
		t.Fatal(err)
	}
	want8 := []Region{{Bounds: image.Rect(0, 0, 2, 2), Pixels: 2}}
	if !reflect.DeepEqual(r8, want8) {
		t.Fatalf("8-connectivity: got %+v, want %+v", r8, want8)
	}
}

func TestRegionsMaskSeparation(t *testing.T) {
	rect := image.Rect(0, 0, 3, 1)
	a := regionTestImage(rect)
	b := regionTestImage(rect, image.Rect(0, 0, 3, 1))

	var joined []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&joined, RegionConfig{}))
	if err != nil || diff != 3 {
		t.Fatalf("got %d, %v; want 3 diffs", diff, err)
	}
	if want := []Region{{Bounds: image.Rect(0, 0, 3, 1), Pixels: 3}}; !reflect.DeepEqual(joined, want) {
		t.Fatalf("unmasked: got %+v, want %+v", joined, want)
	}

	// Masking the middle pixel must split the line into two regions; the
	// masked pixel is excluded from the diff count and from every region.
	mask := []byte{0, 1, 0}
	var split []Region
	diff, err = MatchPixel(a, b, IncludeAntiAlias, IgnoreMask(mask), Regions(&split, RegionConfig{}))
	if err != nil || diff != 2 {
		t.Fatalf("masked: got %d, %v; want 2 diffs", diff, err)
	}
	want := []Region{
		{Bounds: image.Rect(0, 0, 1, 1), Pixels: 1},
		{Bounds: image.Rect(2, 0, 3, 1), Pixels: 1},
	}
	if !reflect.DeepEqual(split, want) {
		t.Fatalf("masked: got %+v, want %+v", split, want)
	}
}

func TestRegionsNoDiff(t *testing.T) {
	a := regionTestImage(image.Rect(0, 0, 8, 8))
	var regions []Region
	diff, err := MatchPixel(a, a, Regions(&regions, RegionConfig{}))
	if err != nil || diff != 0 {
		t.Fatalf("got %d, %v; want 0", diff, err)
	}
	if regions == nil || len(regions) != 0 {
		t.Fatalf("got %+v, want empty non-nil slice", regions)
	}
}

func TestRegionsNonZeroOrigin(t *testing.T) {
	rect := image.Rect(10, 20, 30, 35)
	a := regionTestImage(rect)
	b := regionTestImage(rect, image.Rect(12, 22, 15, 25))

	var regions []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&regions, RegionConfig{}))
	if err != nil || diff != 9 {
		t.Fatalf("got %d, %v; want 9", diff, err)
	}
	want := []Region{{Bounds: image.Rect(12, 22, 15, 25), Pixels: 9}}
	if !reflect.DeepEqual(regions, want) {
		t.Fatalf("got %+v, want %+v", regions, want)
	}
}

func TestRegionsFullImageDiff(t *testing.T) {
	rect := image.Rect(0, 0, 16, 9)
	a := regionTestImage(rect)
	b := regionTestImage(rect, rect)

	var regions []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&regions, RegionConfig{}))
	if err != nil || diff != 16*9 {
		t.Fatalf("got %d, %v; want %d", diff, err, 16*9)
	}
	want := []Region{{Bounds: rect, Pixels: 16 * 9}}
	if !reflect.DeepEqual(regions, want) {
		t.Fatalf("got %+v, want %+v", regions, want)
	}
}

func TestRegionsManyFragmentsDeterministic(t *testing.T) {
	rect := image.Rect(0, 0, 64, 64)
	a := regionTestImage(rect)
	b := regionTestImage(rect)
	// Checkerboard of single-pixel diffs: the most fragmented case possible.
	for y := 0; y < 64; y++ {
		for x := (y + 1) % 2; x < 64; x += 2 {
			o := b.PixOffset(x, y)
			b.Pix[o], b.Pix[o+1], b.Pix[o+2] = 0, 0, 0
		}
	}

	var first []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&first, RegionConfig{}))
	if err != nil || diff != 64*64/2 {
		t.Fatalf("got %d, %v; want %d", diff, err, 64*64/2)
	}
	if len(first) != diff {
		t.Fatalf("got %d regions, want %d single-pixel regions", len(first), diff)
	}
	for i, r := range first {
		if r.Pixels != 1 || r.Bounds.Dx() != 1 || r.Bounds.Dy() != 1 {
			t.Fatalf("region %d: got %+v, want single pixel", i, r)
		}
		if i > 0 {
			prev := first[i-1].Bounds.Min
			cur := r.Bounds.Min
			if cur.Y < prev.Y || (cur.Y == prev.Y && cur.X <= prev.X) {
				t.Fatalf("regions not sorted: %+v before %+v", prev, cur)
			}
		}
	}

	var second []Region
	if _, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&second, RegionConfig{})); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("repeated run produced different regions")
	}
}

func TestRegionsMinPixels(t *testing.T) {
	rect := image.Rect(0, 0, 20, 5)
	a := regionTestImage(rect)
	b := regionTestImage(rect,
		image.Rect(0, 0, 1, 1),   // 1 pixel
		image.Rect(4, 0, 7, 2),   // 6 pixels
		image.Rect(12, 0, 14, 2), // 4 pixels
	)

	var regions []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&regions, RegionConfig{MinPixels: 4}))
	if err != nil || diff != 11 {
		t.Fatalf("got %d, %v; want 11", diff, err)
	}
	want := []Region{
		{Bounds: image.Rect(4, 0, 7, 2), Pixels: 6},
		{Bounds: image.Rect(12, 0, 14, 2), Pixels: 4},
	}
	if !reflect.DeepEqual(regions, want) {
		t.Fatalf("got %+v, want %+v", regions, want)
	}
}

func TestRegionsMergeGap(t *testing.T) {
	rect := image.Rect(0, 0, 20, 10)
	r1 := image.Rect(2, 2, 6, 6)
	r2 := image.Rect(9, 2, 13, 6) // 3 pixels away from r1
	a := regionTestImage(rect)
	b := regionTestImage(rect, r1, r2)

	var split []Region
	if _, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&split, RegionConfig{MergeGap: 2})); err != nil {
		t.Fatal(err)
	}
	if len(split) != 2 {
		t.Fatalf("gap 2: got %+v, want 2 regions", split)
	}

	var merged []Region
	diff, err := MatchPixel(a, b, IncludeAntiAlias, Regions(&merged, RegionConfig{MergeGap: 3}))
	if err != nil {
		t.Fatal(err)
	}
	want := []Region{{Bounds: image.Rect(2, 2, 13, 6), Pixels: 32}}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("gap 3: got %+v, want %+v", merged, want)
	}
	if merged[0].Pixels != diff {
		t.Fatalf("merged pixels %d do not equal diff count %d", merged[0].Pixels, diff)
	}
}

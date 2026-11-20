package pixelmatch

import (
	"image"
	"sort"
)

// Region describes one connected group of differing pixels. Bounds uses the
// first image's coordinate space and Pixels is the number of differing pixels
// inside it.
type Region struct {
	Bounds image.Rectangle
	Pixels int
}

// RegionConfig controls how differing pixels are grouped into regions.
type RegionConfig struct {
	// Connectivity is 4 (edge-adjacent) or 8 (edge- or corner-adjacent).
	// Any value other than 8 uses 4.
	Connectivity int
	// MinPixels drops regions containing fewer pixels. Zero keeps all regions.
	MinPixels int
	// MergeGap merges regions whose bounding boxes come within this many
	// pixels of each other. Zero disables merging. Merging never changes the
	// reported pixel counts; it only joins bounds.
	MergeGap int
}

// Regions reports the connected regions of differing pixels through out,
// grouped according to config. The pixel comparison itself is unchanged: the
// returned diff count and any WriteTo output are identical to a call without
// Regions, and the sum of region pixel counts equals that count unless
// MinPixels or MergeGap is set. Regions are sorted by bounds (top-to-bottom,
// then left-to-right), so the result is deterministic. Pixels excluded by
// IgnoreMask or anti-aliasing detection are not part of any region and never
// connect two regions.
func Regions(out *[]Region, config RegionConfig) MatchOption {
	return func(o *MatchOptions) {
		o.regions = out
		o.regionConfig = config
	}
}

// diffRegions labels connected components in the width*height diff mask and
// aggregates them into regions. It allocates O(width*height) memory
// regardless of how fragmented the mask is.
func diffRegions(mask []byte, width, height int, origin image.Point, cfg RegionConfig) []Region {
	regions := make([]Region, 0)
	if width <= 0 || height <= 0 {
		return regions
	}

	// Pass 1: assign provisional labels, unioning already-labeled neighbors.
	// Labels are 1-based; zero in labels means "not a diff pixel".
	labels := make([]int32, width*height)
	parent := make([]int32, 0, 64) // parent[label-1], roots point at themselves
	var find func(l int32) int32
	find = func(l int32) int32 {
		root := l
		for parent[root-1] != root {
			root = parent[root-1]
		}
		for parent[l-1] != root {
			parent[l-1], l = root, parent[l-1]
		}
		return root
	}
	union := func(a, b int32) {
		ra, rb := find(a), find(b)
		if ra != rb {
			// The lower label stays the root so results do not depend on
			// scan order beyond this rule.
			if ra < rb {
				parent[rb-1] = ra
			} else {
				parent[ra-1] = rb
			}
		}
	}

	diagonal := cfg.Connectivity == 8
	var nextLabel int32
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			if mask[i] == 0 {
				continue
			}
			label := int32(0)
			join := func(l int32) {
				if l == 0 {
					return
				}
				if label == 0 {
					label = l
				} else {
					union(label, l)
				}
			}
			if x > 0 {
				join(labels[i-1])
			}
			if y > 0 {
				join(labels[i-width])
				if diagonal {
					if x > 0 {
						join(labels[i-width-1])
					}
					if x+1 < width {
						join(labels[i-width+1])
					}
				}
			}
			if label == 0 {
				nextLabel++
				parent = append(parent, nextLabel)
				label = nextLabel
			}
			labels[i] = label
		}
	}

	// Pass 2: aggregate bounds and pixel counts per root label.
	type aggregate struct {
		minX, minY, maxX, maxY, count int
	}
	aggs := make([]aggregate, nextLabel+1)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			l := labels[y*width+x]
			if l == 0 {
				continue
			}
			agg := &aggs[find(l)]
			if agg.count == 0 {
				agg.minX, agg.minY, agg.maxX, agg.maxY = x, y, x, y
			} else {
				agg.minX = minInt(agg.minX, x)
				agg.minY = minInt(agg.minY, y)
				agg.maxX = maxInt(agg.maxX, x)
				agg.maxY = maxInt(agg.maxY, y)
			}
			agg.count++
		}
	}
	for i := range aggs {
		agg := &aggs[i]
		if agg.count == 0 {
			continue
		}
		regions = append(regions, Region{
			Bounds: image.Rect(agg.minX, agg.minY, agg.maxX+1, agg.maxY+1),
			Pixels: agg.count,
		})
	}

	if cfg.MergeGap > 0 && len(regions) > 1 {
		regions = mergeRegions(regions, cfg.MergeGap)
	}

	if cfg.MinPixels > 1 {
		kept := regions[:0]
		for _, r := range regions {
			if r.Pixels >= cfg.MinPixels {
				kept = append(kept, r)
			}
		}
		regions = kept
	}

	sort.Slice(regions, func(i, j int) bool {
		a, b := regions[i].Bounds, regions[j].Bounds
		if a.Min.Y != b.Min.Y {
			return a.Min.Y < b.Min.Y
		}
		if a.Min.X != b.Min.X {
			return a.Min.X < b.Min.X
		}
		if a.Max.Y != b.Max.Y {
			return a.Max.Y < b.Max.Y
		}
		return a.Max.X < b.Max.X
	})
	for i := range regions {
		regions[i].Bounds = regions[i].Bounds.Add(origin)
	}
	return regions
}

// mergeRegions unions regions whose bounding boxes, each expanded by gap
// pixels on every side, intersect. Pixel counts are preserved; bounds become
// the union of the merged bounds.
func mergeRegions(regions []Region, gap int) []Region {
	order := make([]int, len(regions))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := regions[order[i]].Bounds, regions[order[j]].Bounds
		if a.Min.X != b.Min.X {
			return a.Min.X < b.Min.X
		}
		return a.Min.Y < b.Min.Y
	})

	parent := make([]int, len(regions))
	for i := range parent {
		parent[i] = i
	}
	var find func(i int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i], i = parent[parent[i]], parent[i]
		}
		return i
	}

	// Sweep left to right, keeping regions whose expanded box can still reach
	// the current one. A region leaves the active set once its expanded right
	// edge is left of the current left edge.
	active := make([]int, 0, 16)
	for _, i := range order {
		b := regions[i].Bounds
		kept := active[:0]
		for _, j := range active {
			a := regions[j].Bounds
			if a.Max.X+gap < b.Min.X {
				continue
			}
			if a.Min.Y-gap <= b.Max.Y && b.Min.Y-gap <= a.Max.Y {
				ri, rj := find(i), find(j)
				if ri != rj {
					parent[rj] = ri
				}
			}
			kept = append(kept, j)
		}
		active = append(kept, i)
	}

	// Group by root in index order so the result does not depend on map
	// iteration order.
	merged := make([]Region, len(regions))
	used := make([]bool, len(regions))
	for i := range regions {
		root := find(i)
		if !used[root] {
			merged[root] = regions[i]
			used[root] = true
		} else {
			merged[root].Bounds = merged[root].Bounds.Union(regions[i].Bounds)
			merged[root].Pixels += regions[i].Pixels
		}
	}
	out := make([]Region, 0, len(regions))
	for i := range merged {
		if used[i] {
			out = append(out, merged[i])
		}
	}
	return out
}

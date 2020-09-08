package geojsonvt

/* these algorithms house all thats needed to perform the geospatial transformations betwen coordinate systems.

 */

import (
	"github.com/paulmach/go.geojson"
	"math"
	"fmt"
)

func Round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}

func transformPoint(x, y, extent, z2, tx, ty float64) []int32 {
	return []int32{
		int32(Round(extent*(x*z2-tx), .5, 0)),
		int32(Round(extent*(y*z2-ty), .5, 0))}
}

// square distance from a point to a segment
func getSqSegDist(px, py, x, y, bx, by float64) float64 {

	dx := bx - x
	dy := by - y

	if dx != 0 || dy != 0 {

		t := ((px-x)*dx + (py-y)*dy) / (dx*dx + dy*dy)

		if t > 1 {
			x = bx
			y = by

		} else if t > 0 {
			x += dx * t
			y += dy * t
		}
	}

	dx = px - x
	dy = py - y

	return dx*dx + dy*dy
}

// shifts each features coordinates over
func shiftCoords(points []float64, offset float64) []float64 {
	for i := 0; i < len(points); i += 3 {
		points[i] = points[i] + offset
	}
	return points
}

// shifts each featuers coords over
func shiftFeatureCoords(features []Feature, offset float64) []Feature {
	for featurepos, feature := range features {
		switch feature.Type {
		case "Point":
			feature.Geometry.Point = shiftCoords(feature.Geometry.Point, offset)
		case "MultiPoint":
			feature.Geometry.MultiPoint = shiftCoords(feature.Geometry.MultiPoint, offset)
		case "LineString":
			feature.Geometry.LineString = shiftCoords(feature.Geometry.LineString, offset)
		case "MultiLineString":
			for pos := range feature.Geometry.MultiLineString {
				feature.Geometry.MultiLineString[pos] = shiftCoords(feature.Geometry.MultiLineString[pos], offset)
			}

		case "Polygon":
			for pos := range feature.Geometry.Polygon {
				feature.Geometry.Polygon[pos] = shiftCoords(feature.Geometry.Polygon[pos], offset)
			}
		case "MultiPolygon":
			for i := range feature.Geometry.MultiPolygon {
				for pos := range feature.Geometry.MultiPolygon[i] {
					feature.Geometry.MultiPolygon[i][pos] = shiftCoords(feature.Geometry.MultiPolygon[i][pos], offset)
				}
			}
		}
		features[featurepos] = feature
	}
	return features
}

// wrap for the normal features implementation
func wrap(features []Feature, options Config) []Feature {
	buffer := options.Buffer / options.Extent
	merged := features

	left := clip(features, 1, float64(-1.0-buffer), float64(buffer), 0, -1, 2, options,options.HasM)     // left world copy
	right := clip(features, 1, float64(1.0-buffer), float64(2.0+buffer), 0, -1, 2, options,options.HasM) // right world copy

	if len(left) > 0 || len(right) > 0 {
		merged = clip(features, 1, float64(-buffer), float64(1.0+buffer), 0, -1, 2, options,options.HasM) // center world copy
		if len(left) > 0 {
			merged = append(merged, shiftFeatureCoords(left, float64(1))...)
		}
		if len(right) > 0 {
			merged = append(merged, shiftFeatureCoords(right, float64(-1))...)
		}
	}
	return merged
}

func projectX(x float64) float64 {
	return x/360 + 0.5
}

func projectY(y float64) float64 {
	sin := math.Sin(y * math.Pi / 180.0)
	y2 := 0.5 - 0.25*math.Log((1+sin)/(1-sin))/math.Pi
	if y2 < 0 {
		return 0
	} else if y2 > 1 {
		return 1
	}
	return y2
}

func convertLine(ring [][]float64, tolerance float64, isPolygon bool) []float64 {
	var x0, y0 float64
	size := 0.0
	out := NewSlice(len(ring)*3, 0)
	for j := 0; j < len(ring); j++ {
		x := projectX(ring[j][0])
		y := projectY(ring[j][1])
		out.AddPoint(x, y, 0)

		if j > 0 {
			if isPolygon {
				size += (x0*y - x*y0) / 2 // area
			} else {
				size += math.Sqrt(math.Pow(x-x0, 2) + math.Pow(y-y0, 2)) // length
			}
		}
		x0 = x
		y0 = y
	}

	last := out.Pos - 3
	out.Slice[2] = 1
	var simplify func(coords []float64, first, last int, sqTolerance float64)
	var first int
	simplify = func(coords []float64, first, last int, sqTolerance float64) {
		var maxSqDist = sqTolerance
		var index int

		ax := coords[first]
		ay := coords[first+1]
		bx := coords[last]
		by := coords[last+1]

		for i := first + 3; i < last; i += 3 {
			var d = getSqSegDist(coords[i], coords[i+1], ax, ay, bx, by)
			if d > maxSqDist {
				index = i
				maxSqDist = d
			}
		}

		if maxSqDist > sqTolerance {
			if index-first > 3 {
				simplify(coords, first, index, sqTolerance)
			}
			coords[index+2] = maxSqDist
			if last-index > 3 {
				simplify(coords, index, last, sqTolerance)
			}
		}
	}
	simplify(out.Slice, first, last, tolerance)
	out.Slice[last+2] = 1
	return out.Slice
}


func convertLineM(ring [][]float64, tolerance float64, isPolygon bool) []float64 {
	var x0, y0 float64
	size := 0.0
	out := NewSlice(len(ring)*4, 0)
	for j := 0; j < len(ring); j++ {
		x := projectX(ring[j][0])
		y := projectY(ring[j][1])
		m := ring[j][2]
		out.AddPointM(x, y, m, 0)

		if j > 0 {
			if isPolygon {
				size += (x0*y - x*y0) / 2 // area
			} else {
				size += math.Sqrt(math.Pow(x-x0, 2) + math.Pow(y-y0, 2)) // length
			}
		}
		x0 = x
		y0 = y
	}

	last := out.Pos - 4
	out.Slice[3] = 1
	var simplify func(coords []float64, first, last int, sqTolerance float64)
	var first int
	simplify = func(coords []float64, first, last int, sqTolerance float64) {
		var maxSqDist = sqTolerance
		var index int

		ax := coords[first]
		ay := coords[first+1]
		bx := coords[last]
		by := coords[last+1]

		for i := first + 4; i < last; i += 4 {
			var d = getSqSegDist(coords[i], coords[i+1], ax, ay, bx, by)
			if d > maxSqDist {
				index = i
				maxSqDist = d
			}
		}

		if maxSqDist > sqTolerance {
			if index-first > 4 {
				simplify(coords, first, index, sqTolerance)
			}
			coords[index+3] = maxSqDist
			if last-index > 4 {
				simplify(coords, index, last, sqTolerance)
			}
		}
	}
	simplify(out.Slice, first, last, tolerance)
	out.Slice[last+3] = 1
	return out.Slice
}



func convertLines(rings [][][]float64, tolerance float64, isPolygon bool) [][]float64 {
	out := make([][]float64, len(rings))
	for pos, ring := range rings {
		if len(ring[0]) == 3 {
			out[pos] = convertLineM(ring,tolerance,isPolygon)
		} else {
			out[pos] = convertLine(ring, tolerance, isPolygon)
		}
	}
	return out
}

func ConvertFeature(feature *geojson.Feature, options Config) Feature {
	tolerance := math.Pow(float64(options.Tolerance)/float64((1<<uint64(options.MaxZoom))*options.Extent), 2)
	var geometry Geometry
	switch feature.Geometry.Type {
	case "Point":
		geometry = Geometry{
			Point: []float64{
				projectX(feature.Geometry.Point[0]),
				projectY(feature.Geometry.Point[1]),
				0.0,
			},
			Type: "Point",
		}
	case "MultiPoint":
		geometry = Geometry{MultiPoint: make([]float64, len(feature.Geometry.MultiPoint)), Type: "MultiPoint"}
		for pos, i := range feature.Geometry.MultiPoint {
			geometry.MultiPoint[pos*3] = projectX(i[0])
			geometry.MultiPoint[pos*3+1] = projectY(i[1])
			geometry.MultiPoint[pos*3+2] = 0.0
		}
	case "LineString":
		if options.HasM {
			fmt.Println("hasm",options.HasM)
			geometry = Geometry{LineString: convertLineM(feature.Geometry.LineString, tolerance, false), Type: "LineString"}
		} else {
			geometry = Geometry{LineString: convertLine(feature.Geometry.LineString, tolerance, false), Type: "LineString"}
		}
			
	case "MultiLineString":

		geometry = Geometry{MultiLineString: convertLines(feature.Geometry.MultiLineString, tolerance, false), Type: "MultiLineString"}
	case "Polygon":
		geometry = Geometry{Polygon: convertLines(feature.Geometry.Polygon, tolerance, true), Type: "Polygon"}

	case "MultiPolygon":
		multipolygon := make([][][]float64, len(feature.Geometry.MultiPolygon))
		for pos, polygon := range feature.Geometry.MultiPolygon {
			multipolygon[pos] = convertLines(polygon, tolerance, true)
		}

		geometry = Geometry{MultiPolygon: multipolygon, Type: "MultiPolygon"}

	}
	if options.PropertiesBool {
		return CreateFeature(feature.ID, geometry, feature.Properties,options.HasM)
	}

	return CreateFeature(feature.ID, geometry, feature.Properties,options.HasM)

}

func ConvertFeatures(features []*geojson.Feature, options Config) []Feature {
	newfeatures := make([]Feature, len(features))
	for i := range features {
		newfeatures[i] = ConvertFeature(features[i], options)
	}
	return newfeatures
}

func reprojectX(x float64) float64 {
	return (x - .5) * 360
}

func reprojectY(y float64) float64 {
	eh := math.Exp(((y - .5) / -.25 * math.Pi))

	y = math.Asin((1-eh)/(-eh-1)) / math.Pi * 180.0

	return y
}

// reprojects a point back into long lat
func Reproject(pt []float64) []float64 {
	return []float64{reprojectX(pt[0]), reprojectY(pt[1])}
}

func convertlinetovt(line []float64) [][]float64 {
	newline := make([][]float64, len(line)/3)
	increment := 0
	for i := 0; i < len(line); i += 3 {
		newline[increment] = Reproject([]float64{line[i], line[i+1]})
		increment++
	}
	return newline
}

func convertlinestovt(lines [][]float64) [][][]float64 {
	newlines := make([][][]float64, len(lines))
	for i := range lines {
		newlines[i] = convertlinetovt(lines[i])
	}
	return newlines
}

func ConvertGeometry(oldgeometry Geometry) *geojson.Geometry {
	var geometry *geojson.Geometry
	switch oldgeometry.Type {
	case "Point":
		geometry = &geojson.Geometry{
			Point: []float64{
				reprojectX(oldgeometry.Point[0]),
				reprojectY(oldgeometry.Point[1]),
			},
			Type: "Point",
		}
	case "MultiPoint":
		geometry = &geojson.Geometry{Type: "MultiPoint"}
		geometry.MultiPoint = convertlinetovt(oldgeometry.MultiPoint)
	case "LineString":
		geometry = &geojson.Geometry{LineString: convertlinetovt(oldgeometry.LineString), Type: "LineString"}
	case "MultiLineString":
		geometry = &geojson.Geometry{MultiLineString: convertlinestovt(oldgeometry.MultiLineString), Type: "MultiLineString"}

	case "Polygon":
		geometry = &geojson.Geometry{Polygon: convertlinestovt(oldgeometry.Polygon), Type: "Polygon"}

	case "MultiPolygon":
		multipolygon := make([][][][]float64, len(oldgeometry.MultiPolygon))
		for pos, polygon := range oldgeometry.MultiPolygon {
			multipolygon[pos] = convertlinestovt(polygon)
		}

		geometry = &geojson.Geometry{MultiPolygon: multipolygon, Type: "MultiPolygon"}

	}
	return geometry
}

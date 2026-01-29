package solarweather

import "math"

const (
	pi     = math.Pi
	twoPi  = 2 * math.Pi
	radDeg = 180 / math.Pi
	degRad = math.Pi / 180
)

type Vec3 struct {
	X float64
	Y float64
	Z float64
}

func (v Vec3) Dot(o Vec3) float64 {
	return v.X*o.X + v.Y*o.Y + v.Z*o.Z
}

func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		X: v.Y*o.Z - v.Z*o.Y,
		Y: v.Z*o.X - v.X*o.Z,
		Z: v.X*o.Y - v.Y*o.X,
	}
}

func (v Vec3) Add(o Vec3) Vec3 {
	return Vec3{X: v.X + o.X, Y: v.Y + o.Y, Z: v.Z + o.Z}
}

func (v Vec3) Sub(o Vec3) Vec3 {
	return Vec3{X: v.X - o.X, Y: v.Y - o.Y, Z: v.Z - o.Z}
}

func (v Vec3) Mul(k float64) Vec3 {
	return Vec3{X: v.X * k, Y: v.Y * k, Z: v.Z * k}
}

func (v Vec3) Norm() float64 {
	return math.Sqrt(v.Dot(v))
}

func (v Vec3) Normalize() Vec3 {
	n := v.Norm()
	if n == 0 {
		return Vec3{}
	}
	return v.Mul(1 / n)
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func wrapToPi(x float64) float64 {
	y := math.Mod(x+pi, twoPi)
	if y < 0 {
		y += twoPi
	}
	y -= pi
	if y == -pi {
		return pi
	}
	return y
}

func degToRad(deg float64) float64 {
	return deg * degRad
}

func radToDeg(rad float64) float64 {
	return rad * radDeg
}

func latLonToVec(latDeg, lonDeg float64) Vec3 {
	lat := degToRad(latDeg)
	lon := degToRad(lonDeg)
	clat := math.Cos(lat)
	return Vec3{
		X: clat * math.Cos(lon),
		Y: clat * math.Sin(lon),
		Z: math.Sin(lat),
	}
}

func angleBetween(a, b Vec3) float64 {
	dot := clamp(a.Dot(b), -1, 1)
	return math.Acos(dot)
}

func slerp(a, b Vec3, t float64) Vec3 {
	dot := clamp(a.Dot(b), -1, 1)
	omega := math.Acos(dot)
	if omega == 0 {
		return a
	}
	sinOmega := math.Sin(omega)
	if sinOmega == 0 {
		return a
	}
	factor1 := math.Sin((1-t)*omega) / sinOmega
	factor2 := math.Sin(t*omega) / sinOmega
	return a.Mul(factor1).Add(b.Mul(factor2))
}

func overlapLength(aStart, aEnd, bStart, bEnd float64) float64 {
	if aEnd < aStart {
		aStart, aEnd = aEnd, aStart
	}
	if bEnd < bStart {
		bStart, bEnd = bEnd, bStart
	}
	start := aStart
	if bStart > start {
		start = bStart
	}
	end := aEnd
	if bEnd < end {
		end = bEnd
	}
	if end <= start {
		return 0
	}
	return end - start
}

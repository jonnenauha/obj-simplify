package objectfile

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// http://www.martinreddy.net/gfx/3d/OBJ.spec

// ObjectType

type Type int

const (
	Unkown Type = iota

	Comment        // #
	MtlLib         // mtllib
	MtlUse         // usemtl
	ChildGroup     // g
	ChildObject    // o
	SmoothingGroup // s
	Vertex         // v
	Normal         // vn
	UV             // vt
	Param          // vp
	Face           // f
	Line           // l
	Point          // p
	Curve          // curv
	Curve2         // curv2
	Surface        // surf
)

func (ot Type) String() string {
	switch ot {
	case Comment:
		return "#"
	case MtlLib:
		return "mtllib"
	case MtlUse:
		return "usemtl"
	case ChildGroup:
		return "g"
	case ChildObject:
		return "o"
	case SmoothingGroup:
		return "s"
	case Vertex:
		return "v"
	case Normal:
		return "vn"
	case UV:
		return "vt"
	case Param:
		return "vp"
	case Face:
		return "f"
	case Line:
		return "l"
	case Point:
		return "p"
	case Curve:
		return "curv"
	case Curve2:
		return "curv2"
	case Surface:
		return "surf"
	}
	return ""
}

func (ot Type) Name() string {
	switch ot {
	case Vertex:
		return "vertices"
	case Normal:
		return "normals"
	case UV:
		return "uvs"
	case Param:
		return "params"
	case ChildGroup:
		return "group"
	case ChildObject:
		return "object"
	}
	return ""
}

func TypeFromString(str string) Type {
	switch str {
	case "#":
		return Comment
	case "mtllib":
		return MtlLib
	case "usemtl":
		return MtlUse
	case "g":
		return ChildGroup
	case "o":
		return ChildObject
	case "s":
		return SmoothingGroup
	case "v":
		return Vertex
	case "vn":
		return Normal
	case "vt":
		return UV
	case "vp":
		return Param
	case "f":
		return Face
	case "l":
		return Line
	case "p":
		return Point
	case "curv":
		return Curve
	case "curv2":
		return Curve2
	case "surf":
		return Surface
	}
	return Unkown
}

// ObjStats

type ObjStats struct {
	Objects  int
	Groups   int
	Faces    int
	Lines    int
	Points   int
	Geometry GeometryStats
}

// OBJ

type OBJ struct {
	Geometry          *Geometry
	MaterialLibraries []string

	Objects  []*Object
	Comments []string
}

func NewOBJ() *OBJ {
	return &OBJ{
		Geometry: NewGeometry(),
		Objects:  make([]*Object, 0),
	}
}

func (o *OBJ) ObjectWithType(t Type) (objects []*Object) {
	for _, o := range o.Objects {
		if o.Type == t {
			objects = append(objects, o)
		}
	}
	return objects
}

func (o *OBJ) CreateObject(t Type, name, material string) *Object {
	if t != ChildObject && t != ChildGroup {
		fmt.Printf("CreateObject: invalid object type %s", t)
		return nil
	}
	child := &Object{
		Type:     t,
		Name:     name,
		Material: material,
		parent:   o,
	}
	if child.Name == "" {
		child.Name = fmt.Sprintf("%s_%d", t.Name(), len(o.ObjectWithType(t))+1)
	}
	o.Objects = append(o.Objects, child)
	return child
}

func (o *OBJ) Stats() ObjStats {
	stats := ObjStats{
		Objects: len(o.ObjectWithType(ChildObject)),
		Groups:  len(o.ObjectWithType(ChildGroup)),
	}
	for _, child := range o.Objects {
		for _, vt := range child.VertexData {
			switch vt.Type {
			case Face:
				stats.Faces++
			case Line:
				stats.Lines++
			case Point:
				stats.Points++
			}
		}
	}
	if o.Geometry != nil {
		stats.Geometry = o.Geometry.Stats()
	}
	return stats
}

// Object

type Object struct {
	Type       Type
	Name       string
	Material   string
	VertexData []*VertexData
	Comments   []string

	parent *OBJ
}

// Reads a vertex data line eg. f and l into this object.
//
// If parent OBJ is non nil, additionally converts negative index
// references into absolute indexes and check out of bounds errors.
func (o *Object) ReadVertexData(t Type, value string, strict bool) (*VertexData, error) {
	var (
		vt  *VertexData
		err error
	)
	switch t {
	case Face:
		vt, err = ParseFaceVertexData(value, strict)
	case Line, Point:
		vt, err = ParseListVertexData(t, value, strict)
	default:
		err = fmt.Errorf("Unsupported vertex data declaration %s %s", t, value)
	}

	if err != nil {
		return nil, err
	} else if o.parent == nil {
		return vt, nil
	}

	// OBJ index references start from 1 not zero.
	// Negative values are relative from the end of currently
	// declared geometry. Convert relative values to absolute.
	geomStats := o.parent.Geometry.Stats()

	for _, decl := range vt.Declarations() {

		if decl.Vertex != 0 {
			if decl.Vertex < 0 {
				decl.Vertex = decl.Vertex + geomStats.Vertices + 1
			}
			if decl.Vertex <= 0 || decl.Vertex > geomStats.Vertices {
				return nil, fmt.Errorf("vertex index %d out of bounds, %d declared so far", decl.Vertex, geomStats.Vertices)
			}
			decl.RefVertex = o.parent.Geometry.Vertices[decl.Vertex-1]
			if decl.RefVertex.Index != decl.Vertex {
				return nil, fmt.Errorf("vertex index %d does not match with referenced geometry value %#v", decl.Vertex, decl.RefVertex)
			}
		}

		if decl.UV != 0 {
			if decl.UV < 0 {
				decl.UV = decl.UV + geomStats.UVs + 1
			}
			if decl.UV <= 0 || decl.UV > geomStats.UVs {
				return nil, fmt.Errorf("uv index %d out of bounds, %d declared so far", decl.UV, geomStats.UVs)
			}
			decl.RefUV = o.parent.Geometry.UVs[decl.UV-1]
			if decl.RefUV.Index != decl.UV {
				return nil, fmt.Errorf("uv index %d does not match with referenced geometry value %#v", decl.UV, decl.RefUV)
			}
		}

		if decl.Normal != 0 {
			if decl.Normal < 0 {
				decl.Normal = decl.Normal + geomStats.Normals + 1
			}
			if decl.Normal <= 0 || decl.Normal > geomStats.Normals {
				return nil, fmt.Errorf("normal index %d out of bounds, %d declared so far", decl.Normal, geomStats.Normals)
			}
			decl.RefNormal = o.parent.Geometry.Normals[decl.Normal-1]
			if decl.RefNormal.Index != decl.Normal {
				return nil, fmt.Errorf("normal index %d does not match with referenced geometry value %#v", decl.Normal, decl.RefNormal)
			}
		}
	}
	o.VertexData = append(o.VertexData, vt)
	return vt, nil
}

// Declaration

// zero value means it was not declared, should not be written
// @note exception: if sibling declares it, must be written eg. 1//2
type Declaration struct {
	Vertex int
	UV     int
	Normal int

	// Pointers to actual geometry values.
	// When serialized to string, the index is read from ref
	// if available. This enables easy geometry rewrites.
	RefVertex, RefUV, RefNormal *GeometryValue
}

func (d *Declaration) Equals(other *Declaration) bool {
	if d.Index(Vertex) != other.Index(Vertex) ||
		d.Index(UV) != other.Index(UV) ||
		d.Index(Normal) != other.Index(Normal) {
		return false
	}
	return true
}

// Use this getter when possible index rewrites has occurred.
// Will first return index from geometry value pointers, if available.
func (d *Declaration) Index(t Type) int {
	switch t {
	case Vertex:
		if d.RefVertex != nil {
			return d.RefVertex.Index
		}
		return d.Vertex
	case UV:
		if d.RefUV != nil {
			return d.RefUV.Index
		}
		return d.UV
	case Normal:
		if d.RefNormal != nil {
			return d.RefNormal.Index
		}
		return d.Normal
	default:
		fmt.Printf("Declaration.Index: Unsupported type %s\n", t)
	}
	return 0
}

// vertex data parsers

func ParseFaceVertexData(str string, strict bool) (vt *VertexData, err error) {
	vt = &VertexData{
		Type: Face,
	}
	for iMain, part := range strings.Split(str, " ") {
		dest := vt.Index(iMain)
		if dest == nil {
			if strict {
				return nil, fmt.Errorf("Invalid face index %d in %s", iMain, str)
			}
			break
		}
		for iPart, datapart := range strings.Split(part, "/") {
			value := 0
			// can be empty eg. "f 1//1 2//2 3//3 4//4"
			if len(datapart) > 0 {
				value, err = strconv.Atoi(datapart)
				if err != nil {
					return nil, err
				}
			}
			switch iPart {
			case 0:
				dest.Vertex = value
			case 1:
				dest.UV = value
			case 2:
				dest.Normal = value
			default:
				if strict {
					return nil, fmt.Errorf("Invalid face vertex data index %d.%d in %s", iMain, iPart, str)
				}
				break
			}
		}
	}
	return vt, nil
}

func ParseListVertexData(t Type, str string, strict bool) (*VertexData, error) {
	if t != Line && t != Point {
		return nil, fmt.Errorf("ParseListVertexData supports face and point type, given: %s", t)
	}
	vt := &VertexData{
		Type: t,
	}
	for iMain, part := range strings.Split(str, " ") {
		decl := &Declaration{}
		for iPart, datapart := range strings.Split(part, "/") {
			if len(datapart) == 0 {
				continue
			}
			value, vErr := strconv.Atoi(datapart)
			if vErr != nil {
				return nil, vErr
			}
			switch iPart {
			case 0:
				decl.Vertex = value
			case 1:
				decl.UV = value
			default:
				if strict {
					return nil, fmt.Errorf("Invalid face vertex data index %d.%d in %s", iMain, iPart, str)
				}
				break
			}
		}
		vt.Points = append(vt.Points, decl)
	}
	return vt, nil
}

// VertexData
// @todo Make face, line etc. separate objects with VertexData being an interface
type VertexData struct {
	Type Type

	// Face
	A *Declaration
	B *Declaration
	C *Declaration
	D *Declaration

	// Line/Point
	Points []*Declaration

	meta map[Type]string
}

func (f *VertexData) SetMeta(t Type, value string) {
	if f.meta == nil {
		f.meta = make(map[Type]string)
	}
	f.meta[t] = value
}

func (f *VertexData) Meta(t Type) string {
	if f.meta != nil {
		return f.meta[t]
	}
	return ""
}

func (f *VertexData) Index(index int) *Declaration {
	switch index {
	case 0:
		if f.A == nil {
			f.A = &Declaration{}
		}
		return f.A
	case 1:
		if f.B == nil {
			f.B = &Declaration{}
		}
		return f.B
	case 2:
		if f.C == nil {
			f.C = &Declaration{}
		}
		return f.C
	case 3:
		if f.D == nil {
			f.D = &Declaration{}
		}
		return f.D
	default:
		return nil
	}
}

func (vt *VertexData) Declarations() []*Declaration {
	switch vt.Type {
	case Face:
		return vt.Face()
	case Line, Point:
		return vt.Points
	}
	return nil
}

func (vt *VertexData) Face() (out []*Declaration) {
	for _, fd := range []*Declaration{vt.A, vt.B, vt.C, vt.D} {
		if fd != nil {
			out = append(out, fd)
		}
	}
	return out
}

func (vt *VertexData) String() (out string) {

	switch vt.Type {

	case Line, Point:
		hasUVs := false
		if vt.Type == Line {
			for _, decl := range vt.Points {
				if decl.Index(UV) != 0 {
					hasUVs = true
					break
				}
			}
		}
		var prev *Declaration
		for di, decl := range vt.Points {
			// remove consecutive duplicate points eg. "l 1 1 2 2 3 4 4"
			if prev != nil && prev.Equals(decl) {
				continue
			}
			if di > 0 {
				out += " "
			}
			out += strconv.Itoa(decl.Index(Vertex))
			if hasUVs {
				out += "/"
				if index := decl.Index(UV); index != 0 {
					out += strconv.Itoa(index)
				}
			}
			prev = decl
		}

	case Face:
		hasUVs, hasNormals := false, false

		// always use ptr refs if available.
		// this enables simple index rewrites.
		decls := vt.Face()
		for _, decl := range decls {
			if !hasUVs {
				hasUVs = decl.Index(UV) != 0
			}
			if !hasNormals {
				hasNormals = decl.Index(Normal) != 0
			}
			if hasUVs && hasNormals {
				break
			}
		}
		for di, decl := range decls {
			if di > 0 {
				out += " "
			}
			out += strconv.Itoa(decl.Index(Vertex))
			if hasUVs || hasNormals {
				out += "/"
				if index := decl.Index(UV); index != 0 {
					out += strconv.Itoa(index)
				}
			}
			if hasNormals {
				out += "/"
				if index := decl.Index(Normal); index != 0 {
					out += strconv.Itoa(index)
				}
			}
		}
	}
	return out
}

// Geometry

type Geometry struct {
	Vertices []*GeometryValue // v    x y z [w]
	Normals  []*GeometryValue // vn   i j k
	UVs      []*GeometryValue // vt   u [v [w]]
	Params   []*GeometryValue // vp   u v [w]
}

func (g *Geometry) ReadValue(t Type, value string, strict bool) (*GeometryValue, error) {
	gv := &GeometryValue{}
	// default values by the spec, not serialized in String() if not touched.
	if t == Vertex || t == Point {
		gv.W = 1
	}
	for i, part := range strings.Split(value, " ") {
		if len(part) == 0 {
			continue
		}
		if part == "-0" {
			part = "0"
		} else if strings.Index(part, "-0.") == 0 {
			// "-0.000000" etc.
			if trimmed := strings.TrimRight(part, "0"); trimmed == "-0." {
				part = "0"
			}
		}
		num, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("Found invalid number from %q: %s", value, err)
		}

		switch i {
		case 0:
			gv.X = num
		case 1:
			gv.Y = num
		case 2:
			gv.Z = num
		case 3:
			if strict && t != Vertex {
				return nil, fmt.Errorf("Found invalid fourth component: %s %s", t.String(), value)
			}
			gv.W = num
		default:
			if strict {
				return nil, fmt.Errorf("Found invalid fifth component: %s %s", t.String(), value)
			}
			break
		}
	}
	// OBJ refs start from 1 not zero
	gv.Index = len(g.Get(t)) + 1
	switch t {
	case Vertex:
		g.Vertices = append(g.Vertices, gv)
	case UV:
		g.UVs = append(g.UVs, gv)
	case Normal:
		g.Normals = append(g.Normals, gv)
	case Param:
		g.Params = append(g.Params, gv)
	default:
		return nil, fmt.Errorf("Unkown geometry value type %d %s", t, t)
	}
	return gv, nil
}

func (g *Geometry) Set(t Type, values []*GeometryValue) {
	switch t {
	case Vertex:
		g.Vertices = values
	case Normal:
		g.Normals = values
	case UV:
		g.UVs = values
	case Param:
		g.Params = values
	}
}

func (g *Geometry) Get(t Type) []*GeometryValue {
	switch t {
	case Vertex:
		return g.Vertices
	case Normal:
		return g.Normals
	case UV:
		return g.UVs
	case Param:
		return g.Params
	}
	return nil
}

func (g *Geometry) Stats() GeometryStats {
	return GeometryStats{
		Vertices: len(g.Vertices),
		Normals:  len(g.Normals),
		UVs:      len(g.UVs),
		Params:   len(g.Params),
	}
}

// GeometryStats

type GeometryStats struct {
	Vertices, Normals, UVs, Params int
}

func (gs GeometryStats) IsEmpty() bool {
	return gs.Vertices == 0 && gs.UVs == 0 && gs.Normals == 0 && gs.Params == 0
}

func (gs GeometryStats) Num(t Type) int {
	switch t {
	case Vertex:
		return gs.Vertices
	case UV:
		return gs.UVs
	case Normal:
		return gs.Normals
	case Param:
		return gs.Params
	default:
		return 0
	}
}

// GeometryValue

type GeometryValue struct {
	Index      int
	Discard    bool
	X, Y, Z, W float64
}

func equals(a, b, epsilon float64) bool {
	return (math.Abs(a-b) <= epsilon)
}

func (gv *GeometryValue) String(t Type) (out string) {
	switch t {
	case UV:
		out = strconv.FormatFloat(gv.X, 'g', -1, 64) + " " + strconv.FormatFloat(gv.Y, 'g', -1, 64)
	default:
		out = strconv.FormatFloat(gv.X, 'g', -1, 64) + " " + strconv.FormatFloat(gv.Y, 'g', -1, 64) + " " + strconv.FormatFloat(gv.Z, 'g', -1, 64)
	}
	// omit default values
	switch t {
	case Vertex, Point:
		if !equals(gv.W, 1, 1e-10) {
			out += " " + strconv.FormatFloat(gv.W, 'g', -1, 64)
		}
	}
	return out
}

func (gv *GeometryValue) Distance(to *GeometryValue) float64 {
	dx := gv.X - to.X
	dy := gv.Y - to.Y
	dz := gv.Z - to.Z
	return dx*dx + dy*dy + dz*dz
}

func (gv *GeometryValue) Equals(other *GeometryValue, epsilon float64) bool {
	if math.Abs(gv.X-other.X) <= epsilon &&
		math.Abs(gv.Y-other.Y) <= epsilon &&
		math.Abs(gv.Z-other.Z) <= epsilon &&
		math.Abs(gv.W-other.W) <= epsilon {
		return true
	}
	return false
}

func NewGeometry() *Geometry {
	return &Geometry{
		Vertices: make([]*GeometryValue, 0),
		Normals:  make([]*GeometryValue, 0),
		UVs:      make([]*GeometryValue, 0),
		Params:   make([]*GeometryValue, 0),
	}
}

// Material

type Material struct {
	Mtllib string
	Name   string
}

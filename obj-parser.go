package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/jonnenauha/obj-simplify/objectfile"
)

var (
	ObjectsParsed int
	GroupsParsed  int
)

type Parser struct {
	Filepath string
}

func (p *Parser) ParseFile(path string) (*objectfile.OBJ, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return p.Parse(b)
}

func (p *Parser) Parse(b []byte) (*objectfile.OBJ, error) {
	dest := objectfile.NewOBJ()
	geom := dest.Geometry

	scanner := bufio.NewScanner(bytes.NewBuffer(b))
	linenum := 0

	var (
		currentObject           *objectfile.Object
		currentObjectName       string
		currentObjectChildIndex int
		currentMaterial         string
		currentSmoothGroup      string
	)

	fakeObject := func(material string) *objectfile.Object {
		ot := objectfile.ChildObject
		if currentObject != nil {
			ot = currentObject.Type
		}
		currentObjectChildIndex++
		name := fmt.Sprintf("%s_%d", currentObjectName, currentObjectChildIndex)
		return dest.CreateObject(ot, name, material)
	}

	for scanner.Scan() {
		linenum++

		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		t, value := parseLineType(line)

		switch t {

		// comments
		case objectfile.Comment:
			if currentObject == nil && len(dest.MaterialLibraries) == 0 {
				dest.Comments = append(dest.Comments, value)
			} else if currentObject != nil {
				// skip comments that might refecence vertex, normal, uv, polygon etc.
				// counts as they wont be most likely true after this tool is done.
				if len(value) > 0 && !strContainsAny(value, []string{"vertices", "normals", "uvs", "texture coords", "polygons", "triangles"}, caseInsensitive) {
					currentObject.Comments = append(currentObject.Comments, value)
				}
			}

		// mtl file ref
		case objectfile.MtlLib:
			dest.MaterialLibraries = append(dest.MaterialLibraries, value)

		// geometry
		case objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param:
			if _, err := geom.ReadValue(t, value); err != nil {
				return nil, wrapErrorLine(err, linenum)
			}

		// object, group
		case objectfile.ChildObject, objectfile.ChildGroup:
			currentObjectName = value
			currentObjectChildIndex = 0
			// inherit currently declared material
			currentObject = dest.CreateObject(t, currentObjectName, currentMaterial)
			if t == objectfile.ChildObject {
				ObjectsParsed++
			} else if t == objectfile.ChildGroup {
				GroupsParsed++
			}

		// object: material
		case objectfile.MtlUse:

			// obj files can define multiple materials inside a single object/group.
			// usually these are small face groups that kill performance on 3D engines
			// as they have to render hundreds or thousands of meshes with the same material,
			// each mesh containing a few faces.
			//
			// this app will convert all these "multi material" objects into
			// separate object, later merging all meshes with the same material into
			// a single draw call geometry.
			//
			// this might be undesirable for certain users, renderers and authoring software,
			// in this case don't use this simplified on your obj files. simple as that.

			// only fake if an object has been declared
			if currentObject != nil {
				// only fake if the current object has declared vertex data (faces etc.)
				// and the material name actually changed (ecountering the same usemtl
				// multiple times in a row would be rare, but check for completeness)
				if len(currentObject.VertexData) > 0 && currentObject.Material != value {
					currentObject = fakeObject(value)
				}
			}

			// store material value for inheriting
			currentMaterial = value

			// set material to current object
			if currentObject != nil {
				currentObject.Material = currentMaterial
			}

		// object: faces
		case objectfile.Face, objectfile.Line:
			// most tools support the file not defining a o/g prior to face declarations.
			// I'm not sure if the spec allows not declaring any o/g.
			// Our data structures and parsing however requires objects to put the faces into,
			// create a default object that is named after the input file (without suffix).
			if currentObject == nil {
				currentObject = dest.CreateObject(objectfile.ChildObject, fileBasename(StartParams.Input), currentMaterial)
			}
			vd, vdErr := currentObject.ReadVertexData(t, value)
			if vdErr != nil {
				return nil, wrapErrorLine(vdErr, linenum)
			}
			// attach current smooth group and reset it
			if len(currentSmoothGroup) > 0 {
				vd.SetMeta(objectfile.SmoothingGroup, currentSmoothGroup)
				currentSmoothGroup = ""
			}

		case objectfile.SmoothingGroup:
			// smooth group can change mid vertex data declaration
			// so it is attched to the vertex data instead of current object directly
			currentSmoothGroup = value

		// unknown
		case objectfile.Unkown:
			return nil, wrapErrorLine(fmt.Errorf("Unsupported line %q\n\nPlease submit a bug report. If you can, provide this file as an attachement.\n> %s\n", line, ApplicationURL+"/issues"), linenum)
		default:
			return nil, wrapErrorLine(fmt.Errorf("Unsupported line %q\n\nPlease submit a bug report. If you can, provide this file as an attachement.\n> %s\n", line, ApplicationURL+"/issues"), linenum)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return dest, nil
}

func wrapErrorLine(err error, linenum int) error {
	return fmt.Errorf("line:%d %s", linenum, err.Error())
}

func parseLineType(str string) (objectfile.Type, string) {
	value := ""
	if i := strings.Index(str, " "); i != -1 {
		value = strings.TrimSpace(str[i+1:])
		str = str[0:i]
	}
	return objectfile.TypeFromString(str), value
}

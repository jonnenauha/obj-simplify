package main

import (
	"strings"

	"github.com/jonnenauha/obj-simplify/objectfile"
)

type Merge struct{}

type merger struct {
	Material string
	Objects  []*objectfile.Object
}

func (processor Merge) Name() string {
	return "Merge"
}

func (processor Merge) Desc() string {
	return "Merges objects and groups with the same material into a single mesh."
}

func (processor Merge) Execute(obj *objectfile.OBJ) error {
	// use an array to preserve original order and
	// to produce always the same output with same input.
	// Map will 'randomize' keys in golang on each run.
	materials := make([]*merger, 0)

	for _, child := range obj.Objects {
		// skip children that do not declare faces etc.
		if len(child.VertexData) == 0 {
			continue
		}
		found := false
		for _, m := range materials {
			if m.Material == child.Material {
				m.Objects = append(m.Objects, child)
				found = true
				break
			}
		}
		if !found {
			materials = append(materials, &merger{
				Material: child.Material,
				Objects:  []*objectfile.Object{child},
			})
		}
	}
	logInfo("  - Found %d unique materials", len(materials))

	mergeName := func(objects []*objectfile.Object) string {
		parts := []string{}
		for _, child := range objects {
			if len(child.Name) > 0 {
				parts = append(parts, child.Name)
			}
		}
		if len(parts) == 0 {
			parts = append(parts, "Unnamed")
		}
		return strings.Join(parts, " ")
	}

	// reset objects, we are about to rewrite them
	obj.Objects = make([]*objectfile.Object, 0)

	for _, merger := range materials {
		src := merger.Objects[0]
		child := obj.CreateObject(src.Type, mergeName(merger.Objects), merger.Material)
		for _, original := range merger.Objects {
			child.VertexData = append(child.VertexData, original.VertexData...)
		}
	}

	return nil
}

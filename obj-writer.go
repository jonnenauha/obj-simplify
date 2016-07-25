package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jonnenauha/obj-simplify/objectfile"
)

type Writer struct {
	obj *objectfile.OBJ
}

func (wr *Writer) WriteFile(path string) error {
	if fileExists(path) {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	err = wr.WriteTo(f)
	if cErr := f.Close(); cErr != nil && err == nil {
		err = cErr
	}
	return err
}

func (wr *Writer) WriteTo(w io.Writer) error {
	ln := func() {
		fmt.Fprint(w, "\n")
	}
	writeLine := func(t objectfile.Type, value string, newline bool) {
		fmt.Fprintf(w, "%s %s\n", t, value)
		if newline {
			ln()
		}
	}
	writeLines := func(t objectfile.Type, values []string, newline bool) {
		for _, v := range values {
			writeLine(t, v, false)
		}
		if newline {
			ln()
		}
	}

	obj := wr.obj

	// leave a comment that signifies this tool was ran on the file
	writeLine(objectfile.Comment, fmt.Sprintf("Processed by %s v%s | %s | %s", ApplicationName, ApplicationVersion, time.Now().UTC().Format(time.RFC3339), ApplicationURL), true)

	// comments
	writeLines(objectfile.Comment, obj.Comments, true)

	// Materials (I think there is always just one, if this can change mid file, this needs to be adjusted and pos tracked during parsing)
	writeLines(objectfile.MtlLib, obj.MaterialLibraries, true)

	// geometry
	for ti, t := range []objectfile.Type{objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param} {
		if slice := obj.Geometry.Get(t); len(slice) > 0 {
			if ti > 0 {
				ln()
			}
			writeLine(objectfile.Comment, fmt.Sprintf("%s [%d]", t.Name(), len(slice)), true)
			for _, value := range slice {
				writeLine(t, value.String(t), false)
			}
		}
	}
	ln()

	// objects: preserves the parsing order of g/o
	writeLine(objectfile.Comment, fmt.Sprintf("objects [%d]", len(obj.Objects)), true)
	for _, child := range obj.Objects {
		writeLine(child.Type, child.Name, false)
		// we dont skip writing material if it has already been declared as the
		// last material, the file is easier to read for humans with write on each
		// child, and this wont take many bytes in the total file size.
		if len(child.Material) > 0 {
			writeLine(objectfile.MtlUse, child.Material, false)
		}
		ln()
		for _, vd := range child.VertexData {
			if sgroup := vd.Meta(objectfile.SmoothingGroup); len(sgroup) > 0 {
				writeLine(objectfile.SmoothingGroup, sgroup, false)
			}
			writeLine(vd.Type, vd.String(), false)
		}
		ln()
	}

	return nil
}

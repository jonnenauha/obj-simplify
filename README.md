# obj-simplify

There are a lot of authoring tools that produce OBJ files. The [spec](http://www.martinreddy.net/gfx/3d/OBJ.spec) is quite simple, but it still leaves a lot of room to export geometry and meshe/material combos that are not always optimal for 3D rendering engines. Artists and the exporters can also omit doing simple cleanup operations that would reduce file size, making loading and rendering the model faster.

The biggest problem in an average OBJ export is the amount of draw calls that can be reduced trivially, but is rarely done in the authoring tool.

This tool automates the following optimization and simplification steps.

* Merge duplicate vertex `v`, normal `vn` and UV `vt` declarations.
* Create objects from "multi-material" face groups.
* Merge object `o` and group `g` face declarations that use the same material into a single mesh, reducing draw call overhead.
* Rewrite geometry declarations.
* Rewrite `o/g` to use absolute indexing and the deduplicated geometry.

This tool can be destructive and contain bugs, it will not let you overwrite the source file. Keep your original files intact. The implementation does not support all the OBJ features out there. It is meant to be used on 3D-models
 that declare faces with `f`. All variants of face declarations in the spec are supported. Lines `l` and points `p` are also preserved and the same deduplication logic is applied to them.
 
 If a particular line in the input file is not supported by the parser, the tool will exit and print a link to submit an issue. If you are submitting an issue please attach a file that can reproduce the bug.

## Quickstart

After [installing go](https://golang.org/doc/install) (verify with `go version`, and don't forget the PATH steps), install and run this tool:

```bash
# Install
go get github.com/jonnenauha/obj-simplify

# Run
obj-simplify -in <input-file> -out <output-file>

# Help
obj-simplify -h
```

## Merging duplicate geometry

Use `-eplison` to tune vector equality checks, the default is `1e-6`. This can have a positive impact especially on large OBJ files. Basic cleanup like trimming trailing zeros and converting -0 into 0 to reduce file size is also executed.

## Object merging and multi-materials

If your 3D-application needs to interact with multiple submeshes (`o/g`) in the model with the same material, you should not use this tool. For example an avatar model that has the same material in both gloves and your app wants to know e.g. which glove the user clicked on. This tool will merge both of the gloves face declarations to a single submesh to reduce draw calls. The visuals are the same, but the structure of the model from the code point of view can change.

Multi-materials inside a single `o/g` declaration is another problem this tool tackles. These are OBJ files that set `material_1`, declare a few faces, set `material_2`, declare a few faces, rinse and repeat. This can produce huge files that have hundreds, thousands or tens of thousands meshes with small triangle counts, that all reference the same few materials. Most rendering engines will happily do those 10k draw calls if you don't do optimizations/merging in your application code after loading the model. This tool will merge all these triangles to a single draw call per material.

## Rewrites

All found geometry from the source file is written at the top of the file, skipping any detected duplicates. Objects/groups are rewritten next so that they reference the deduplicated geometry indexes and are ordered per material.

## three.js

I have contributed to the OBJ parser/loader in three.js and know it very well. I know what kind of files it has performance problems with and how to try to avoid them. I have also implemented some of the optimization done in this tool in JS on the client side, after the model has been loaded. But even if doable, its a waste of time to do them on each load for each user. Also certain optimizations can not be done on the client side.  That being said there is nothing spesific in the tool for three.js, it can help as much in other rendering engines. This tool can help you get:

* Faster load over the network
 * Reduce filesize, possibly better compression e.g. with gzip (see `-gzip`).
* Faster loading by the parser 
 * Drop duplicates, reduce files size in general to parse less lines.
 * Arraging file output in a way that *might* benefit V8 etc. to optimize the execution better.
* Faster rendering 
 * Remove patterns that result in using `THREE.MultiMaterial`.
 * Reduce draw calls.

## Command line options

There are command line flags for configuration and disabling processing steps, see `-h` for help.

```
obj-simplify {
  "Input": "test.obj",
  "Output": "test.simplified.obj",
  "Workers": 32,
  "Gzip": -1,
  "Eplison": 1e-06,
  "Strict": false,
  "Stdout": false,
  "Quiet": false,
  "NoProgress": false,
  "CpuProfile": false
}

processor #1: Duplicates
  - Using epsilon of 1e-06
  - vn deduplicate     1957 / 1957 [==================================] 100.00%
  - vt deduplicate     11 / 11 [======================================] 100.00%
  - v  deduplicate     353 / 353 [====================================] 100.00%
  - v      386 duplicates found for 353 unique indexes (0.91%) in 4.87s
  - vn   11235 duplicates found for 1551 unique indexes (46%) in 6.48s
  - vt      11 duplicates found for 11 unique indexes (0.01%) in 5.71s
  - v     4920 refs replaced in 0.11s
  - vn  296829 refs replaced in 0.06s
  - vt      60 refs replaced in 0.06s

processor #2: Merge
  - Found 88 unique materials

Parse                     0.41s    4%
Duplicates                6.92s    82%
Merge                     0.01s    0.16%
Write                     1.03s    12%
Total                     8.37s

Vertices                 42 099    -386       -0.91%
Normals                  13 041    -11235     -47%
UVs                      76 891    -11        -0.01%

Faces                   162 982

Groups                       88    -532       -86%

Lines input             519 767
Lines output            295 384    -224 383   -43%

File input             12.52 MB
File output            10.01 MB    -2.51 MB   -20%
```

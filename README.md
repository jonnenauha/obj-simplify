# obj-simplify

There are a lot of authoring tools that produce OBJ files. The [spec](http://www.martinreddy.net/gfx/3d/OBJ.spec) is quite simple, but it still leaves a lot of room to export geometry and meshes/material combos that are not always optimal for 3D rendering engines. Artists and the exporters can also omit doing simple cleanup operations that would reduce file sizes, making loading and rendering the model faster.

This tool automates the following processing and simplification steps.

* Merge duplicate vertex `v`, normal `vn` and UV `vt` declarations.
* Create objects from "multi-material" face groups.
* Merge object `o` and group `g` face declarations that use the same material into a single mesh.
* Rewrite geometry declarations.
* Rewrite `o/g` to use absolute indexing and the deduplicated geometry.

This tool can be destructive, the implementation does not support all the OBJ features out there. It is meant to be used on 3D-models, although lines `l` and points `p` are preserved. If a particular line is not supported by the parser, the tool will exit and print a link to submit an issue.

## Merging duplicate geometry

Use `-eplison` to tune vector equality checks, the default is `1e-6`. This can have a positive impact especially on large OBJ files. Basic cleanup like trimming trailing zeros and converting -0 into 0.

Use 

## Object merging and multi-materials

If your 3D-application needs to interact with all of the submeshes in the model, you should not use this tool. For example an avatar model that has the same material in both gloves and your app wants to know e.g. which glove the user clicked on. This tool will merge both of the gloves face declarations to a single submesh to reduce draw calls. The visuals are the same, but the structure of the model can change.

Multi-materials is another problem I wanted to tackle. These are OBJ files that set `material_1`, declare a few faces, set `material_2`, declare a few faces, rinse and repeat. This can produce huge files that have hundreds, thousands or tens of thousands meshes with small triangle counts, that all reference the same few materials. Most rendering engines will happily do 10k draw calls if you don't do optimizations/batching in your application code. This tool will merge all these triangles to a single draw call per material.

## Rewrites

All found geometry from the source file is written at the top of the file, skipping any detected duplicates. Objects/groups are rewritten next so that they reference the deduplicated geometry indexes and are ordered per material.

## Configuration

There are command line flags for configuration and disabling processing steps, see `-h` for more.

```
obj-simplify v0.1 {
  "Input": "test.obj",
  "Output": "test.simplified.obj",
  "Stdout": false,
  "Quiet": false,
  "NoProgress": false,
  "Eplison": 1e-06
}

processor #1: Duplicates
  - Using epsilon of 1e-06
  - v      386 duplicates found for 353 unique indexes (0.91%) in 4.21s
  - vt      11 duplicates found for 11 unique indexes (0.01%) in 5.74s
  - vn   11235 duplicates found for 1551 unique indexes (46%) in 6.59s
  - v     4920 refs replaced in 0.05s
  - vt      60 refs replaced in 0.06s
  - vn  296829 refs replaced in 0.06s
 
processor #2: Merge
  - Found 88 unique materials
 
Parse                     0.40s    4%
Duplicates                6.77s    82%
Merge                     0.01s    0.11%
Write                     1.03s    12%
Total                     8.21s
 
Vertices                 42 099    -386       -0.91%
Normals                  13 041    -11235     -47%
UVs                      76 891    -11        -0.01%
 
Faces                   162 982    
 
Groups                       88    -532       -86%
 
Input file             12.52 MB
Output file            10.00 MB
Diff                   -2.53 MB    -20%
```

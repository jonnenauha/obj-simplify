package main

import (
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"gopkg.in/cheggaaa/pb.v1"

	"github.com/jonnenauha/obj-simplify/objectfile"
)

// replacerList

type replacerList []*replacer

// flat map of index to ptr that replaces that index
func (rl replacerList) FlattenGeometry() map[int]*objectfile.GeometryValue {
	out := make(map[int]*objectfile.GeometryValue)
	for _, r := range rl {
		for index, _ := range r.replaces {
			if out[index] != nil {
				fmt.Printf("duplicate warning\n   %#v\n   %#v\n   %t\n\n", out[index], r.ref, out[index].Equals(r.ref, 1e-6))
			}
			out[index] = r.ref
		}
	}
	return out
}

// replacer

type replacer struct {
	ref           *objectfile.GeometryValue
	replaces      map[int]*objectfile.GeometryValue
	replacesSlice []*objectfile.GeometryValue
	dirty         bool
	hasItems      bool
}

func (r *replacer) Index() int {
	return r.ref.Index
}

func (r *replacer) IsEmpty() bool {
	return !r.hasItems
}

func (r *replacer) NumReplaces() int {
	if r.replaces != nil {
		return len(r.replaces)
	}
	return 0
}

func (r *replacer) Replaces() []*objectfile.GeometryValue {
	// optimization to avoid huge map iters
	if r.dirty {
		r.dirty = false

		if r.replaces != nil {
			r.replacesSlice = make([]*objectfile.GeometryValue, len(r.replaces), len(r.replaces))
			i := 0
			for _, ref := range r.replaces {
				r.replacesSlice[i] = ref
				i++
			}
		} else {
			r.replacesSlice = nil
		}
	}
	return r.replacesSlice
}

func (r *replacer) Remove(index int) {
	if r.replaces == nil {
		return
	}
	if _, found := r.replaces[index]; found {
		r.dirty = true
		delete(r.replaces, index)
		r.hasItems = len(r.replaces) > 0
	}
}

func (r *replacer) Hit(ref *objectfile.GeometryValue) {
	// cannot hit self
	if ref.Index == r.ref.Index {
		return
	}
	if r.replaces == nil {
		r.replaces = make(map[int]*objectfile.GeometryValue)
	}
	r.dirty = true
	r.hasItems = true
	r.replaces[ref.Index] = ref
}

func (r *replacer) Hits(index int) bool {
	return r.hasItems && r.replaces[index] != nil
}

// call merge only if r.Hits(other.Index())
// returns if other was completely merged to r.
func (r *replacer) Merge(other *replacer) {
	for _, value := range other.Replaces() {
		if value.Index == r.ref.Index {
			other.Remove(r.ref.Index)
			continue
		}
		if r.hasItems && r.replaces[value.Index] != nil {
			// straight up duplicate
			other.Remove(value.Index)
		} else if r.ref.Equals(value, StartParams.Epsilon) {
			// move equals hit to r from other
			r.Hit(value)
			other.Remove(value.Index)
		}
	}
	// if not completely merged at this point, we must
	// reject other.Index() from our hit list.
	if other.hasItems {
		r.Remove(other.Index())
	}
}

// replacerByIndex

type replacerByIndex []*replacer

func (a replacerByIndex) Len() int           { return len(a) }
func (a replacerByIndex) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a replacerByIndex) Less(i, j int) bool { return a[i].Index() < a[j].Index() }

// replacerResults

type replacerResults struct {
	Type  objectfile.Type
	Items []*replacer
	Spent time.Duration
}

func (rr *replacerResults) Duplicates() (duplicates int) {
	for _, r := range rr.Items {
		duplicates += r.NumReplaces()
	}
	return duplicates
}

// Duplicates

type Duplicates struct{}

func (processor Duplicates) Name() string {
	return "Duplicates"
}

func (processor Duplicates) Desc() string {
	return "Removes duplicate v/vn/vt declarations. Rewrites vertex data references."
}

func (processor Duplicates) Execute(obj *objectfile.OBJ) error {
	var (
		results         = make(map[objectfile.Type]*replacerResults)
		mResults        = sync.RWMutex{}
		wg              = &sync.WaitGroup{}
		preStats        = obj.Geometry.Stats()
		epsilon         = StartParams.Epsilon
		progressEnabled = !StartParams.NoProgress
	)

	logInfo("  - Using epsilon of %s", strconv.FormatFloat(epsilon, 'g', -1, 64))

	// Doing this with channels felt a bit overkill, copying a lot of replacers etc.
	setResults := func(result *replacerResults) {
		mResults.Lock()
		// If there is no progress bars, report results as they come in so user knows something is happening...
		if !progressEnabled {
			logInfo("  - %-2s %7d duplicates found for %d unique indexes (%s%%) in %s",
				result.Type, result.Duplicates(), len(result.Items), computeFloatPerc(float64(result.Duplicates()), float64(preStats.Num(result.Type))), formatDuration(result.Spent))
		}
		results[result.Type] = result
		mResults.Unlock()
	}

	// find duplicated
	{
		mResults.Lock()

		var (
			bars         = make(map[objectfile.Type]*pb.ProgressBar)
			progressPool *pb.Pool
			progressErr  error
		)
		// progress bars
		if !StartParams.NoProgress {
			for _, t := range []objectfile.Type{objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param} {
				if slice := obj.Geometry.Get(t); len(slice) > 0 {
					bar := pb.New(len(slice)).Prefix(fmt.Sprintf("  - %-2s scan    ", t.String())).SetMaxWidth(130)
					bar.ShowTimeLeft = false
					bars[t] = bar
				}
			}
			barsSlice := make([]*pb.ProgressBar, 0)
			for _, bar := range bars {
				barsSlice = append(barsSlice, bar)
			}
			if progressPool, progressErr = pb.StartPool(barsSlice...); progressErr != nil {
				// progress pools do not work in all shells on windows (eg. liteide)
				bars = make(map[objectfile.Type]*pb.ProgressBar)
				progressPool = nil
				progressEnabled = false
			}
		}
		// start goroutines
		for _, t := range []objectfile.Type{objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param} {
			if slice := obj.Geometry.Get(t); len(slice) > 0 {
				wg.Add(1)
				go findDuplicates(t, slice, epsilon, wg, bars[t], setResults)
			}
		}

		mResults.Unlock()

		wg.Wait()

		if progressPool != nil {
			progressPool.Stop()
		}
		// report totals now in the same order as it would print without progress bars
		if progressEnabled {
			mResults.Lock()
			for _, t := range []objectfile.Type{objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param} {
				if result := results[t]; result != nil {
					logInfo("  - %-2s %7d duplicates found for %d unique indexes (%s%%) in %s",
						result.Type, result.Duplicates(), len(result.Items), computeFloatPerc(float64(result.Duplicates()), float64(preStats.Num(result.Type))), formatDuration(result.Spent))
				}
			}
			mResults.Unlock()
		}
	}

	// Rewrite ptr refs to vertex data that is using an about to be removed duplicate.
	// Exec in main thread, accessing the vertex data arrays in objects would be
	// too much contention with a mutex. This operation is fairly fast, no need for parallel exec.
	// Sweeps and marks .Discard to replaced values
	for _, t := range []objectfile.Type{objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param} {
		if result := results[t]; result != nil {
			replaceDuplicates(result.Type, obj, result.Items)
		}
	}

	// Rewrite geometry
	for _, t := range []objectfile.Type{objectfile.Vertex, objectfile.Normal, objectfile.UV, objectfile.Param} {
		src := obj.Geometry.Get(t)
		if len(src) == 0 {
			continue
		}
		dest := make([]*objectfile.GeometryValue, 0)
		for _, gv := range src {
			if !gv.Discard {
				gv.Index = len(dest) + 1
				dest = append(dest, gv)
			}
		}
		if len(dest) != len(src) {
			obj.Geometry.Set(t, dest)
		}
	}
	return nil
}

func findDuplicates(t objectfile.Type, slice []*objectfile.GeometryValue, epsilon float64, wgMain *sync.WaitGroup, progress *pb.ProgressBar, callback func(*replacerResults)) {
	defer wgMain.Done()

	var (
		started  = time.Now()
		results  = make(replacerList, 0)
		mResults sync.RWMutex
	)

	appendResults := func(rs []*replacer) {
		mResults.Lock()
		for _, result := range rs {
			if !result.IsEmpty() {
				results = append(results, result)
			}
		}
		mResults.Unlock()
	}

	processSlice := func(substart, subend int, fullslice []*objectfile.GeometryValue, subwg *sync.WaitGroup) {
		innerResults := make(replacerList, 0)
		var value *objectfile.GeometryValue
		for first := substart; first < subend; first++ {
			if progress != nil {
				progress.Increment()
			}
			result := &replacer{
				ref: fullslice[first],
			}
			for second, lenFull := first+1, len(fullslice); second < lenFull; second++ {
				value = fullslice[second]
				if value.Equals(result.ref, epsilon) {
					result.Hit(value)
				}
			}
			if !result.IsEmpty() {
				innerResults = append(innerResults, result)
			}
		}
		appendResults(innerResults)

		subwg.Done()
	}

	wgInternal := &sync.WaitGroup{}
	numPerRoutine := len(slice) / StartParams.Workers
	for iter := 0; iter < StartParams.Workers; iter++ {
		start := iter * numPerRoutine
		end := start + numPerRoutine
		if end >= len(slice) || iter == StartParams.Workers-1 {
			end = len(slice)
			iter = StartParams.Workers
		}
		wgInternal.Add(1)
		go processSlice(start, end, slice, wgInternal)
	}
	wgInternal.Wait()

	mResults.Lock()
	defer mResults.Unlock()

	if len(results) == 0 {
		return
	}

	sort.Sort(replacerByIndex(results))

	if progress != nil {
		progress.Prefix(fmt.Sprintf("  - %-2s merge    ", t))
		progress.Total = int64(len(results))
		progress.Set(0)
	}

	var r1, r2 *replacer

	// 1st run: merge
	for i1, lenResults := 0, len(results); i1 < lenResults; i1++ {
		if progress != nil {
			progress.Increment()
		}
		r1 = results[i1]
		if !r1.hasItems {
			continue
		}
		for i2 := i1 + 1; i2 < lenResults; i2++ {
			r2 = results[i2]
			/*if !r2.hasItems {
				continue
			}*/
			/*if r1.ref.Index == r2.ref.Index {
				// same primary index, this is a bug
				logFatal("r1.Index() and r2.Index() are the same, something wrong with sub slice processing code\n%#v\n%#v\n\n", r1, r2)
			}*/
			if r2.hasItems && r1.hasItems && r1.replaces[r2.ref.Index] != nil {
				// r1 geom value equals r2.
				// only merge r2 hits where value equals r1, otherwise
				// we would do transitive merges which is not what we want:
				// eg. r1 closer than epsilon to r2, but r1 further than epsilon to r2.hitN
				r1.Merge(r2)
				// r1 might now be empty if r2 was its only hit,
				// and it was not completely merged.
				if !r1.hasItems {
					break
				}
			}
		}
	}

	nonemptyMerged := make(replacerList, 0)
	for _, r := range results {
		if r.hasItems {
			nonemptyMerged = append(nonemptyMerged, r)
		}
	}
	if progress != nil {
		progress.Prefix(fmt.Sprintf("  - %-2s deduplicate    ", t))
		progress.Total = int64(len(nonemptyMerged))
		progress.Set(0)
	}

	// 2nd run: deduplicate, must be done after full merge to work correctly.
	//
	// Deduplicate hits that are in both r1 and r2. This can happen if a value
	// is between r1 and r2. Both equal with the in between value but
	// not with each other (see above merge).
	// In this case the hit is kept in the result that is closest to it.
	// if r and other both have a hit index, which is not shared by being
	// closer than epsilon tp both, keep it in the parent that it is closest to.
	for i1, lenResults := 0, len(nonemptyMerged); i1 < lenResults; i1++ {
		if progress != nil {
			progress.Increment()
		}
		r1 = nonemptyMerged[i1]
		if !r1.hasItems {
			continue
		}
		for i2 := i1 + 1; i2 < lenResults; i2++ {
			r2 = nonemptyMerged[i2]
			if r2.hasItems {
				deduplicate(r1, r2)
				if !r1.hasItems {
					break
				}
			}
		}
	}

	// Gather non empty results
	nonemptyFinal := make([]*replacer, 0)
	for _, r := range nonemptyMerged {
		if r.hasItems {
			nonemptyFinal = append(nonemptyFinal, r)
		}
	}

	// send results back
	callback(&replacerResults{
		Type:  t,
		Items: nonemptyFinal,
		Spent: time.Since(started),
	})
}

func deduplicate(r1, r2 *replacer) {
	for _, value := range r1.Replaces() {
		if !r2.hasItems {
			// merged to empty during this iteration
			return
		} else if r2.replaces[value.Index] == nil {
			// no hit for index, avoid function call
			continue
		}

		// keep whichever is closest to value
		dist1, dist2 := r1.ref.Distance(value), r2.ref.Distance(value)
		if dist1 < dist2 {
			r2.Remove(value.Index)
		} else {
			r1.Remove(value.Index)
		}
	}
}

func replaceDuplicates(t objectfile.Type, obj *objectfile.OBJ, replacements replacerList) {
	rStart := time.Now()

	indexToRef := replacements.FlattenGeometry()

	replaced := 0
	for _, child := range obj.Objects {
		for _, vt := range child.VertexData {
			// catch newly added types that are not implemented yet here
			if vt.Type != objectfile.Face && vt.Type != objectfile.Line && vt.Type != objectfile.Point {
				logFatal("Unsupported vertex data type %q for replacing duplicates\n\nPlease submit a bug report. If you can, provide this file as an attachement.\n> %s\n", vt.Type, ApplicationURL+"/issues")
			}
			for _, decl := range vt.Declarations {
				switch t {
				case objectfile.Vertex:
					if ref := indexToRef[decl.Vertex]; ref != nil {
						replaced++
						decl.RefVertex.Discard = true
						decl.RefVertex = ref
					}
				case objectfile.UV:
					if ref := indexToRef[decl.UV]; ref != nil {
						replaced++
						decl.RefUV.Discard = true
						decl.RefUV = ref
					}
				case objectfile.Normal:
					if ref := indexToRef[decl.Normal]; ref != nil {
						replaced++
						decl.RefNormal.Discard = true
						decl.RefNormal = ref
					}
				}
			}
		}
	}
	logInfo("  - %-2s %7d refs replaced in %s", t, replaced, formatDurationSince(rStart))
}
